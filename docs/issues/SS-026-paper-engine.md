# SS-026 — Deterministic Paper-Trading Engine (Bar-Close Triggers, Orders, Fills, Ledger)

| | |
| --- | --- |
| Milestone | 4 — Paper trading |
| Depends on | SS-016, SS-025, SS-011 |
| Blocks | SS-027, SS-028, SS-033 |
| Size | ~3 days |
| Stack | Go, PostgreSQL, Redis (idempotency) |

## Goal

Turn live closed bars into simulated orders with the same math as the backtest, and do it *deterministically* —
identical input bars must always produce an identical ledger.

## Scope

**In scope**
- `internal/paper/engine`:
  - Subscribes to `BarClosed` events (SS-011). For each ACTIVE deployment whose `(stock, tf)` matches, builds the
    strategy input (warmup history from DB + live bars, via a bounded cache), calls the Python evaluator, and
    processes the resulting signal.
  - Trigger contract: evaluation happens **once per closed bar**, keyed by `(deployment_id, bucket_time)`. The
    same `(deployment_id, bucket_time)` replayed produces the same decision and is deduplicated by a Redis key
    `paper:done:<deployment>:<bucket_time>` **and** a DB unique constraint.
  - Order types: `MARKET`, `LIMIT`, `SL` (stop-loss), `SL-M`; TIF `DAY`/`IOC`. Limit/SL orders rest in an
    in-memory + DB order book and are evaluated against subsequent bars using a documented rule
    (limit fills when `low <= limit` for buys and `high >= limit` for sells; SL triggers on the same bar it is
    crossed, then fills at the worse of trigger/next-open).
  - Fill model: reuses SS-016 for participation cap, slippage and charges; no partial fills (qty is all-or-nothing
    after cap) and this is documented as a stated limitation.
  - Ledger: `paper_orders`, `paper_trades` (spec §5.3 schema: side, entry/exit time+price, qty, gross_pnl,
    charges_and_taxes, net_pnl, is_win), `paper_ledger_events` (append-only audit with sequence numbers).
  - Warmup: on activation, load the last N bars needed by `warmup_bars` + strategy window via SS-003/SS-005
    (filled gaps) **before** the first live evaluation, and refuse to trade until warmup completes.
  - Windows/market-hours guard: no evaluation outside `marketcal.IsOpen`; forced flat-optional at close
    (`intraday_square_off` config).
- Evaluator contract with the Python engine: `EvaluateBar{deployment_id, source_ref, symbol, tf, bars[]}` →
  `Decision{signal, indicators[], diagnostics}` through the SS-020 transport with a tight timeout (2 s) and a
  degraded mode (skip bar + emit event) rather than blocking the stream.
- Determinism requirements: no `time.Now()` inside decision logic (bars carry timestamps), no map iteration
  order affecting output, no goroutine races on wallet state (single writer per deployment, serialised queue).

**Out of scope**
- Risk caps and the daily kill switch (SS-027) — this issue exposes hooks (`pre_trade_check`) it fills in.
- UI (SS-033), replay test harness (SS-028, though it verifies this engine).

## Small architecture

```text
BarClosed{stock, tf, candle}
      │  (single serialised worker per deployment)
      ▼
[dedupe: Redis SETNX paper:done:<dep>:<ts> + DB unique]
      │
      ├─ warmup check ──► insufficient history? emit BarSkipped, return
      ├─ evaluate via Python (2s timeout) ──► Decision{signal}
      ├─ pre_trade_check (risk hooks, SS-027)
      ├─ build Order → fill via SS-016 (cap+slippage+charges) → paper_trades + ApplyFill(wallet)
      └─ emit TradeExecuted / OrderRejected on bus → SS-012 pushes TRADE to UI
```

```go
type Engine interface {
    OnBarClosed(ctx context.Context, ev BarClosed) error   // idempotent per (deployment, bucket_time)
    OnDeploy(ev DeploymentActivated) error                 // warmup then subscribe
    OnStop(ev DeploymentPaused|DeploymentKilled) error
    Replay(ctx context.Context, deploymentID int64, bars []BarClosed) (LedgerDiff, error) // used by SS-028
}
```

* One goroutine (or a per-deployment serialised queue) owns a deployment's state; parallelism is across
  deployments, never within one — this is what makes the ledger deterministic.
* Every state change writes an append-only `paper_ledger_events` row with a monotonic per-deployment `seq`, so
  the UI and SS-028 can reconstruct state by replay.
* The engine is restart-safe: on boot it reloads ACTIVE deployments, their open positions and pending orders,
  and resumes from the last processed bar; a missed bar range triggers catch-up replay through `Replay`.

## Tasks

- [ ] Order/trade/ledger tables + repositories with idempotency constraints.
- [ ] Bar-subscription wiring, per-deployment serialised worker, dedupe key.
- [ ] Warmup loader (uses SS-005 fill policy) + readiness gating.
- [ ] Evaluator client with timeout, retry-once, degraded skip + metric.
- [ ] Market/LIMIT/SL/SL-M order lifecycle with resting orders and documented fill rules.
- [ ] Fill pipeline integration with SS-016 + `ApplyFill` (SS-025) + bus events.
- [ ] Restart/catch-up logic.
- [ ] ADR `docs/adr/0026-fill-model.md` (all stated limitations: no partial fills, no queue position, no
      market impact).

## Acceptance criteria

1. Feeding the same 100 bars twice (fresh deployment each time) produces byte-identical `paper_trades` and
   final equity.
2. A duplicate `BarClosed` for the same `(deployment, bucket_time)` produces exactly one trade and one ledger
   event (DB unique constraint proves it).
3. Warmup is enforced: a deployment activated with no history emits `BarSkipped{warmup_incomplete}` and takes no
   trades until N bars are available.
4. A BUY signal on a bar with volume 200 and a 10% cap fills ≤ 20 shares, with charges matching SS-016 exactly.
5. A resting LIMIT buy at ₹100 fills only on a bar whose low ≤ 100, at the documented price (with slippage),
   never on a bar that never traded through it.
6. An SL at ₹95 on a bar with low ₹92 fills at the documented worse-of price (not at exactly ₹95) — asserted.
7. Evaluator timeout (simulated 3 s) skips the bar, emits `EvaluationTimeout`, and does not block subsequent
   bars (next bar processed within 100 ms of arriving).
8. Restart mid-session resumes without duplicating or losing trades (count and hashes match a continuous run).

## Test plan

- Determinism test with a fixed bar fixture, run twice, compare ledger hashes.
- Idempotency test double-delivering events.
- Order-type matrix tests (each order type × filled/not-filled/triggered-on-gap).
- Restart test (kill process between bars, resume, diff ledgers).
- Warmup test with insufficient history.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.5, §3.7, §5.3, §6 (deterministic execution guarantee),
docs/issues/SS-026-paper-engine.md. Repo: Go; SS-011 emits BarClosed events on a bus; SS-016 cost/slippage
logic is mirrored in Go or called via a thin gRPC wrapper (pick one, document it); SS-025 provides deployment
state machine, wallets, ApplyFill, and DeploymentActivated/Paused/Killed events; SS-020 provides the Python
transport; marketcal from SS-005; candles store from SS-003.

Implement SS-026 only: the deterministic paper-trading engine in internal/paper/engine.
1. Trigger: one evaluation per closed bar keyed by (deployment_id, bucket_time). Deduplicate with a Redis SETNX
   key paper:done:<dep>:<bucket_time> AND a DB unique constraint so a duplicate event can never create a second
   trade. All decision logic must be a pure function of the bar sequence — no time.Now(), no map-order
   dependence.
2. Concurrency: one serialised worker per deployment (parallel across deployments only), so wallet mutations are
   single-writer and deterministic.
3. Warmup: on activation load warmup_bars + window of history via the candles store with SS-005 gap filling, and
   refuse to trade until warmup completes (emit BarSkipped{warmup_incomplete}).
4. Orders: MARKET, LIMIT, SL, SL-M with DAY/IOC; resting orders persisted and evaluated on subsequent bars using
   documented rules (limit fills when low<=limit for buys / high>=limit for sells; SL triggers and fills at the
   worse of trigger and next open). Fill pipeline reuses SS-016 for participation cap, slippage and charges; no
   partial fills — document this limitation.
5. Ledger: paper_orders, paper_trades (side, entry/exit time+price, qty, gross_pnl, charges_and_taxes, net_pnl,
   is_win) and append-only paper_ledger_events with a monotonic per-deployment seq. Emit TradeExecuted /
   OrderRejected on the bus for the WS hub.
6. Evaluator: call the Python engine over SS-020 with a 2s timeout; on timeout skip the bar, emit
   EvaluationTimeout, and keep processing later bars.
7. Restart safety: reload ACTIVE deployments, positions and resting orders; catch up missed bars through
   Replay(); provide Replay(ctx, deploymentID, bars) returning a ledger diff for SS-028.

Constraints: no risk caps or kill switch (SS-027 fills the pre_trade_check hook); no UI; every assumption goes
in ADR 0026 (no partial fills, no queue position, no market impact, worse-of SL pricing).

Done when: 100-bar fixture replay twice is byte-identical; duplicate BarClosed yields one trade; warmup gating
works; participation cap + charges match SS-016 exactly; LIMIT fills only when traded through; SL fills at the
worse-of price; a 3s evaluator timeout doesn't stall subsequent bars; restart mid-session leaves identical
ledgers. Include the ledger hash comparison.
```

## Notes / risks

- The Python round-trip per bar is the latency bottleneck at 1s timeframes; batch evaluations per deployment and
  cache indicator state, or accept a documented 1s-TF limitation.
- Determinism dies the moment costs are recomputed with floating-point accumulation order changes — keep money
  arithmetic in `Decimal`/fixed-point in the ledger.
