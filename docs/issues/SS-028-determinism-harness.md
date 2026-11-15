# SS-028 — Determinism Replay Harness + Paper PnL/Win-Rate API & Stream

| | |
| --- | --- |
| Milestone | 4 — Paper trading |
| Depends on | SS-026, SS-027 |
| Blocks | SS-033, SS-034 |
| Size | ~2.5 days |
| Stack | Go, PostgreSQL, Redis |

## Goal

Prove the spec's headline guarantee — "given the same bars, re-running the paper trader produces the exact same
entries, quantities, slippage and win rates" — and expose live PnL/win-rate numbers the dashboard can trust.

## Scope

**In scope**
- `internal/paper/replay`:
  - `Record(deploymentID, day) → fixture` (bars + decisions + fills + ledger) stored as a versioned JSON artifact.
  - `Replay(fixture, {different machine/seed/goroutine count})` → ledger, then a **structural diff**
    (`added/removed/changed` trades, wallet deltas, charges) with a CI gate requiring zero differences.
  - Golden fixtures committed for 3 scenarios (winner, loser, kill-switch day) + a nightly job replaying them.
  - Diagnostics on divergence: first differing `seq`, the bar, and both ledgers — so a failure is actionable,
    not mysterious.
- PnL aggregation service:
  - `GET /api/v1/paper-trading/summary` → per-deployment and portfolio rows: `initial_capital, equity, cash,
    realized_pnl, unrealized_pnl, total_return_pct, win_rate_pct, total_trades, wins, losses, profit_factor,
    max_drawdown_pct, avg_holding_period, status`.
  - `GET /api/v1/paper-trading/deployments/{id}/trades?cursor=` (keyset pagination) and `/equity-curve?tf=`.
  - Materialised/rolling aggregates updated on each fill (`paper_deployment_stats`) so summary is O(1) per
    deployment, recomputed nightly to correct drift.
  - WS frames on the existing hub: `PAPER_STATS {deployment_id, equity, win_rate_pct, trades, day_pnl}` at most
    once per second per deployment; `TRADE` frames already exist from SS-026.
- Reconciliation guard: nightly job comparing aggregate stats against a full recomputation from `paper_trades`,
  alerting (and auto-correcting) on mismatch > 0.

**Out of scope**
- Charts (SS-033 uses `/equity-curve`). Backtest vs paper parity (nice-to-have; note it as future work).

## Small architecture

```text
record: bars → engine.Replay(capture=true) → fixture.json {bars[], decisions[], fills[], ledger[], hashes}
replay: fixture → engine.Replay(same bars, fresh state) → ledger' → diff(ledger, ledger') → PASS/FAIL

fills ──► paper_trades (source of truth)
      └─► increment paper_deployment_stats (equity, wins, trades, PF, dd)
                └─► WS PAPER_STATS (throttled 1/s)

nightly: recompute stats from paper_trades → compare → correct + alert
```

* Determinism levers recorded in the fixture: bar list hash, strategy `source_ref` + `ast_hash`,
  `cost_config_version`, `slippage_config_json`, `risk_config_json`, engine `rules_version`. A replay refuses to
  run if any lever differs — otherwise a "failure" is just a config change.
* The diff compares money as strings (exact decimal), not floats.
* Stats updates happen in the same transaction as the fill (or via an outbox) so stats can never lag a trade by
  more than one flush.
* WS throttle is per deployment with a trailing-edge flush so the last update is never dropped.

## Tasks

- [ ] `Record`/`Replay`/`Diff` + fixture schema + `make replay-fixtures` target.
- [ ] Three committed golden fixtures covering winner/loser/kill-switch; CI job + nightly schedule.
- [ ] `paper_deployment_stats` table + transactional updates + nightly reconciliation.
- [ ] Summary/trades/equity-curve endpoints with keyset pagination + OpenAPI.
- [ ] WS `PAPER_STATS` frame with throttling and trailing flush.
- [ ] ADR `docs/adr/0028-determinism-contract.md` (levers, freeze rules, what a diff failure means).

## Acceptance criteria

1. Replaying each committed fixture produces a **zero-diff** ledger, verified in CI, and the harness demonstrably
   fails when a single price is perturbed (negative test).
2. A replay with a changed `cost_config_version` is refused with a clear error rather than reporting a diff.
3. `GET /paper-trading/summary` returns win rate and PnL matching a recomputation from `paper_trades` to the
   paisa for 5 deployments.
4. Stats are O(1): summary for 200 deployments returns in < 200 ms with no full-ledger scan (assert via EXPLAIN).
5. WS `PAPER_STATS` arrives at most once per second per deployment and the final update after the last trade is
   always delivered.
6. Nightly reconciliation introduces zero corrections on a clean dataset and repairs a deliberately corrupted
   stats row, logging the delta.
7. Equity curve endpoint returns one point per requested interval with IST timestamps and correct cumulative
   values.

## Test plan

- Determinism: 3 fixtures × CI + nightly; negative test by perturbing a bar.
- Reconciliation: seed a wrong stats row, run the job, assert repair + log.
- Load: 200 deployments summary latency and WS fan-out sanity.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §6 ("Deterministic Paper-Trading Execution Guarantee") and §2.5
(win rate / ledger tracking) and §9 (GET /api/v1/paper-trading/summary),
docs/issues/SS-028-determinism-harness.md. Repo: Go; SS-026 exposes Replay(ctx, deploymentID, bars) returning a
ledger diff and writes paper_trades/paper_ledger_events; SS-027 adds risk events; SS-012 hub provides Broadcast;
SS-016 costs define charge math.

Implement SS-028 only.
1. internal/paper/replay: Record(deploymentID, day) capturing bars, decisions, fills and ledger into a versioned
   fixture JSON that also stores every determinism lever (bar-list hash, strategy source_ref + ast_hash,
   cost_config_version, slippage_config_json, risk_config_json, rules_version). Replay(fixture) must refuse when
   any lever differs from the current config (clear error, not a diff). Diff compares ledgers structurally
   (added/removed/changed trades, wallet deltas, charges) using exact decimal string comparison and reports the
   first divergent seq with both ledgers.
2. Commit three golden fixtures (winner day, loser day, daily-kill-switch day) plus a `make replay-fixtures`
   target wired into CI; add a negative test proving the harness fails when one price is perturbed.
3. PnL aggregation: paper_deployment_stats updated transactionally with each fill (or via outbox), plus a nightly
   reconciliation job recomputing from paper_trades, correcting drift and logging deltas.
4. Endpoints: GET /api/v1/paper-trading/summary (per-deployment and portfolio: equity, cash, realized/unrealized
   pnl, total_return_pct, win_rate_pct, total_trades, wins, losses, profit_factor, max_drawdown_pct,
   avg_holding_period, status), GET /paper-trading/deployments/{id}/trades with keyset pagination,
   GET /paper-trading/deployments/{id}/equity-curve?tf=. Update contracts/openapi.yaml.
5. WS frame PAPER_STATS {deployment_id, equity, win_rate_pct, trades, day_pnl} throttled to 1/s per deployment
   with a trailing flush so the last update is never dropped.

Constraints: summary must be O(1) per deployment (no ledger scans); no chart rendering; no backtest parity work
(note as future); exact decimal money everywhere.

Done when: all three fixtures replay with zero diff in CI and the perturbed-price test fails as expected; a lever
mismatch is refused; summary matches recomputation to the paisa across 5 deployments and returns in <200ms for
200 deployments; PAPER_STATS throttling plus trailing flush verified; reconciliation repairs a corrupted row with
a logged delta; equity curve is IST-ordered and cumulative. Include the CI log of the replay gate.
```

## Notes / risks

- The harness is only meaningful if production runs the same code path as `Replay` — no "test-only" shortcuts in
  the engine, enforced by code review and by `Replay` calling the same public engine methods.
- Fixtures contain bar data; keep them small (one session) to stay inside repo size limits, or store as artifacts
  with hashes committed.
