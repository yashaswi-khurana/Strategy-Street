# SS-025 — Paper Deployment Lifecycle, Virtual Wallet & Position Sizing

| | |
| --- | --- |
| Milestone | 4 — Paper trading |
| Depends on | SS-021 |
| Blocks | SS-026, SS-033 |
| Size | ~2 days |
| Stack | Go, PostgreSQL |

## Goal

A deployment is the unit of live simulated trading: a strategy version + symbol + timeframe + virtual capital +
sizing rules, with a lifecycle the rest of the system can trust.

## Scope

**In scope**
- Tables: `paper_deployments` (spec §5.3) extended with `sizing_mode, sizing_value, max_positions,
  allow_short, cost_config_version, slippage_config_json, risk_config_json, created_at, activated_at,
  paused_at, killed_at, kill_reason`; `paper_wallets` (deployment_id, initial_capital, cash, equity,
  realized_pnl, unrealized_pnl, updated_at); `paper_positions` (open position snapshot) — trades live in SS-026.
- State machine: `DRAFT → ACTIVE ⇄ PAUSED → KILLED` (terminal). Illegal transitions return `409` with the
  current/attempted states. `KILLED` is irreversible; a new deployment is required to restart.
- Endpoints:
  - `POST /api/v1/paper-trading/deploy` `{strategy_version_id, stock_id, timeframe, initial_capital,
    sizing_mode, sizing_value, risk_config}` → validates (version belongs to user, symbol active, timeframe
    supported, capital ≥ ₹10 000) → `201 {deployment_id, status: DRAFT}`.
  - `POST /api/v1/paper-trading/deployments/{id}/activate|pause|resume|kill` with idempotent semantics.
  - `GET /api/v1/paper-trading/deployments` (list, filtered), `GET .../{id}` (state + wallet + open positions).
- Sizing rules (`internal/paper/sizing`): `fixed_capital` (₹ amount per trade), `pct_equity` (% of current
  equity), `fixed_qty`; integer share rounding against `tick_size`/`lot_size`; never exceed available cash.
- Validation guardrails: one ACTIVE deployment per (strategy_version, symbol, timeframe) to avoid double
  counting; max deployments per user; capital reserved from a per-user notional budget (config).
- Activation emits `DeploymentActivated{id, stock, tf, version}` on the event bus so SS-012 acquires the
  subscription and SS-026 warms up history.

**Out of scope**
- Signal evaluation, fills, PnL accounting (SS-026). Risk kill-switch logic beyond storing config (SS-027).

## Small architecture

```text
POST /deploy → validate → INSERT paper_deployments(DRAFT) + wallet(initial_capital)
activate     → state machine guard → DRAFT|PAUSED→ACTIVE → bus.publish(DeploymentActivated)
pause        → ACTIVE→PAUSED → bus.publish(DeploymentPaused)   (positions kept, no new entries)
kill         → *→KILLED → bus.publish(DeploymentKilled)         (force-flatten in SS-026)
```

```go
type Sizing interface { Qty(ctx SizingContext) (int, error) }   // deterministic, pure
type SizingContext struct { Equity, Cash, Price float64; LotSize int; Mode Mode; Value float64 }
```

* `sizing.Qty` is pure and table-tested: `pct_equity` recomputes from equity each entry; rounding is
  `floor` to `lot_size` multiples and must be deterministic (no float drift — use `math.Floor(x+1e-9)`).
* Wallet writes go through one repository method (`ApplyFill`) so SS-026 has a single place to update cash,
  realised/unrealised PnL; this issue ships the method plus tests with a fake fill.
* State transitions are enforced in SQL with a `CHECK`-driven trigger or a transaction with `SELECT … FOR UPDATE`,
  so two concurrent `activate` calls can't both succeed.
* Deployments snapshot their config (`cost_config_version`, `slippage_config_json`, `risk_config_json`) so
  changing global config never mutates a live deployment's behaviour.

## Tasks

- [ ] Migrations: deployments (extended), wallets, positions + transition constraints.
- [ ] Repository + state machine with `FOR UPDATE` transitions and `409` on illegal moves.
- [ ] Sizing package with three modes + rounding rules + table tests.
- [ ] `ApplyFill` wallet mutation (idempotent by `fill_id`).
- [ ] CRUD/action handlers + OpenAPI + validation (ownership, capital floor, duplicate-active guard).
- [ ] Event-bus publish/subscribe contract (`DeploymentActivated` etc.) + ADR `docs/adr/0025-deployment-states.md`.

## Acceptance criteria

1. `POST /deploy` with ₹5 000 capital returns `422`; with ₹1 00 000, returns `201 DRAFT` and a wallet row with
   `cash = 100000, equity = 100000`.
2. Legal transitions succeed; `KILLED → ACTIVE` returns `409` naming both states; `PAUSED → ACTIVE` succeeds.
3. Two concurrent `activate` calls yield exactly one success and one `409` (no double activation).
4. `pct_equity` with equity ₹1 00 000, price ₹123.45, lot 1 → qty 810; with lot 100 → qty 800.
   `fixed_capital` ₹50 000 at ₹250 → qty 200. Cash-insufficient cases clamp to 0 and report a reason.
5. A second ACTIVE deployment for the same (version, symbol, tf) is rejected with `409`.
6. `ApplyFill` called twice with the same `fill_id` changes the wallet once.
7. Activating publishes exactly one `DeploymentActivated` event (asserted on a test bus).

## Test plan

- Table-driven state machine tests (all 16 transition pairs).
- Sizing property tests: `qty*price <= cash`, `qty % lot == 0`, determinism.
- Concurrency test with `pgx` transactions (`-race`).

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.7 (virtual wallet, position sizing) and §5.3 (paper_deployments),
docs/issues/SS-025-paper-deployments.md. Repo: Go api + pgx + chi; SS-021 strategies/versions; SS-002 stocks/
sectors; an event-bus interface and the SS-012 hub exist.

Implement SS-025 only: paper deployment lifecycle, wallets and sizing.
1. Tables: paper_deployments with strategy_version_id, stock_id, timeframe, initial_capital, sizing_mode
   (fixed_capital|pct_equity|fixed_qty), sizing_value, max_positions, allow_short, cost_config_version,
   slippage_config_json, risk_config_json, status (DRAFT/ACTIVE/PAUSED/KILLED) and timestamps + kill_reason;
   paper_wallets (initial_capital, cash, equity, realized_pnl, unrealized_pnl); paper_positions snapshot.
2. State machine DRAFT->ACTIVE<->PAUSED->KILLED(KILLED terminal), enforced in a transaction with SELECT ... FOR
   UPDATE; illegal transitions return 409 naming both states; concurrent activations allow exactly one winner.
3. Endpoints: POST /paper-trading/deploy (validates ownership, active symbol, supported timeframe, capital >=
   Rs 10,000, and rejects a second ACTIVE deployment for the same version/symbol/timeframe), POST
   /paper-trading/deployments/{id}/activate|pause|resume|kill (idempotent), GET list and GET detail returning
   status + wallet + open positions. Update contracts/openapi.yaml.
4. Sizing package with pure, table-tested implementations: fixed_capital, pct_equity, fixed_qty; floor to
   lot_size multiples with float-drift guard; clamp to available cash with an explicit reason; deterministic.
5. Wallet repository ApplyFill(fill_id, side, qty, price, charges) idempotent by fill_id (SS-026 will call it),
   updating cash/realized/unrealized correctly.
6. Publish DeploymentActivated/Paused/Killed/Resumed events on the bus; document the state machine and the
   config snapshot rule in ADR 0025.

Constraints: no signal evaluation or fill simulation here (SS-026); no risk logic beyond storing config
(SS-027); deployments must snapshot their cost/slippage/risk config so global changes never mutate them.

Done when: capital floor and duplicate-active guards return the right codes; all 16 transition pairs behave
correctly with 409s where illegal; concurrent activate yields one success; sizing examples produce 810/800/200
with lot and cash rules; ApplyFill is idempotent by fill_id; exactly one DeploymentActivated event is published;
-race passes.
```

## Notes / risks

- Paper trading without a reserved notional budget lets a user "deploy" 200 strategies and call the aggregate
  PnL a portfolio — enforce per-user limits now, not later.
- `KILLED` being terminal is deliberate: silent restarts destroy the audit trail users rely on for win rates.
