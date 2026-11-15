# SS-022 — Backtest Orchestration, Universe Fan-Out & Progress WebSocket

| | |
| --- | --- |
| Milestone | 3 — Orchestration |
| Depends on | SS-020, SS-021 |
| Blocks | SS-023, SS-032 |
| Size | ~2.5 days |
| Stack | Go, PostgreSQL, Redis |

## Goal

One API call turns "run strategy v3 across the NSE universe on 5m for 2 years" into hundreds of queued,
monitored, cancellable jobs with live progress the UI can render.

## Scope

**In scope**
- Tables `backtest_runs` (spec §5.3) with `strategy_version_id, timeframe, from, to, universe_filter, status,
  total_stocks, completed_stocks, failed_stocks, params_json, rules_version, cost_config_version, created_at,
  started_at, finished_at, error, cancel_requested`.
- `POST /api/v1/backtests/run` `{strategy_version_id, timeframe, from, to, sector_ids?, symbols?, params?}`:
  - Resolves the universe (`stocks` where active, optional sector filter).
  - Pre-flight: checks data coverage per symbol via `candles.LatestTime`/SS-009 `Plan`; reports
    `warmup_required` and refuses (422) when coverage is below a threshold unless `allow_partial=true`.
  - Creates the run, enqueues one job per (stock, param-set) with `P2PaperWarmup`-equivalent priority for
    data backfill and normal job priority for compute.
  - Returns `202 {run_id, total_jobs}` immediately.
- Lifecycle state machine: `QUEUED → RUNNING → COMPLETED | FAILED | CANCELLED`, with partial success allowed
  (`COMPLETED_WITH_ERRORS` when some stocks fail).
- `GET /api/v1/backtests/{run_id}` → status + counters + aggregated progress.
- `POST /api/v1/backtests/{run_id}/cancel` → cooperative cancel via SS-020.
- `GET /api/v1/backtests?strategy_version_id=&limit=` → history list.
- Progress WS on the existing hub: frames `BACKTEST_PROGRESS {run_id, done, total, percent, current_symbol,
  eta_seconds, status}` at most 1/s per run; terminal frame always sent.
- Crash recovery: on boot, runs stuck in `RUNNING` with no worker heartbeat are resumed or marked failed
  (documented rule).

**Out of scope**
- Storing per-stock metrics (SS-023). Sector scoring (SS-024).

## Small architecture

```text
POST /backtests/run
  ├─ resolve universe (N stocks)
  ├─ coverage pre-flight (candles.LatestTime + backfill.Plan)
  ├─ INSERT backtest_runs(status=QUEUED, total_stocks=N)
  ├─ for each stock: queue.Enqueue(BacktestJob{run_id, stock, tf, from, to, params})
  └─ 202 {run_id}

queue.Progress(run_id) ──► aggregator (atomic counters) ──► throttled WS broadcast + DB counter update
job result  ──► persist metrics (SS-023) ──► when done==total: status=COMPLETED
```

* Counters live in Redis (`HINCRBY run:<id>`) as the hot path and are flushed to Postgres every 5 s; the API
  reads Redis first, DB as fallback — never a table scan per progress tick.
* The run is idempotent per `(strategy_version_id, timeframe, from, to)` when `reuse_completed=true`: return the
  existing completed run id instead of re-running.
* Failure policy: a stock failing validation/limits is recorded and skipped; > 20% failures aborts the run with
  `FAILED` and a summary of error classes.
* Job priority: compute jobs normal, data backfill jobs `P2` so a user's chart still wins.

## Tasks

- [ ] Migrations `backtest_runs` + status enum + counters.
- [ ] Universe resolution + coverage pre-flight (with `allow_partial`).
- [ ] Enqueue path with per-stock ids and idempotency key.
- [ ] Progress aggregator (Redis + throttled WS + DB flush).
- [ ] State machine + cancellation + boot-time recovery.
- [ ] Handlers + OpenAPI + contract tests.
- [ ] Load test: 500-stock run with a stubbed engine.

## Acceptance criteria

1. `POST /backtests/run` returns `202` in < 300 ms with `total_jobs` equal to the active stock count and the run
   row in `QUEUED`.
2. `GET /backtests/{id}` reports monotonically increasing `completed_stocks` and never exceeds `total_stocks`.
3. WS progress deltas arrive at ≤ 1/s per run and always include a terminal frame with `percent=100` and final
   status.
4. Cancelling a running backtest stops new jobs within 2 s, marks the run `CANCELLED`, and in-flight jobs report
   `cancelled` (no orphan writes).
5. Submitting the same `(version, tf, from, to)` twice with `reuse_completed=true` returns the same `run_id`.
6. Insufficient coverage without `allow_partial` returns `422` listing the symbols and the earliest available
   date.
7. Killing the API mid-run then restarting leaves no run stuck in `RUNNING` (recovery rule applied).

## Test plan

- Integration: real Redis + fake queue producer asserting exactly N enqueues.
- Progress aggregation test with out-of-order job completions.
- Recovery test using a pre-seeded `RUNNING` row with stale heartbeat.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.4, §3.8, §5.3, §9, docs/issues/SS-022-backtest-orchestration.md.
Repo: Go api; SS-020 provides internal/queue (Enqueue, Progress(ctx,runID) channel, Cancel) over Redis Streams
+ gRPC; SS-021 provides strategies/strategy_versions; SS-003 candles store with LatestTime; SS-009 backfill.Plan;
SS-012 hub with Broadcast for WS frames.

Implement SS-022 only: backtest run orchestration.
1. backtest_runs table (strategy_version_id, timeframe, from, to, universe_filter jsonb, status enum
   QUEUED/RUNNING/COMPLETED/COMPLETED_WITH_ERRORS/FAILED/CANCELLED, total_stocks, completed_stocks,
   failed_stocks, params_json, rules_version, cost_config_version, created_at/started_at/finished_at, error,
   cancel_requested).
2. POST /api/v1/backtests/run {strategy_version_id, timeframe, from, to, sector_ids?, symbols?, params?,
   allow_partial?, reuse_completed?}: resolve the universe from stocks, run a coverage pre-flight using
   candles LatestTime and backfill.Plan, return 422 listing symbols and earliest available dates when coverage
   is insufficient (unless allow_partial), create the run, enqueue one job per stock via SS-020, return
   202 {run_id, total_jobs} in under 300ms.
3. Progress: Redis hash HINCRBY counters as the hot path, flushed to Postgres every 5s; GET /backtests/{run_id}
   returns status + counters; WS frames BACKTEST_PROGRESS {run_id, done, total, percent, current_symbol,
   eta_seconds, status} through the existing hub, throttled to 1/s per run, always ending with a terminal frame.
4. State machine incl. COMPLETED_WITH_ERRORS (>0 but <=20% failures), FAILED (>20%), cooperative cancel via
   SS-020 that marks CANCELLED within 2s, and boot-time recovery for runs stuck in RUNNING with stale heartbeat.
5. Idempotency: reuse_completed returns the existing finished run for the same (version, tf, from, to).
   GET /backtests list endpoint with filters and pagination. Update contracts/openapi.yaml.

Constraints: no per-stock metric persistence (SS-023); no strategy execution in Go; counters must never be
read by scanning rows per tick.

Done when: 202 in <300ms with correct total_jobs; progress monotonic and capped; terminal WS frame always sent;
cancel stops new jobs in <2s with no orphan writes; reuse_completed returns the same run id; 422 lists
under-covered symbols; API restart leaves no stuck RUNNING run. Include a 500-stock stub-engine load test result.
```

## Notes / risks

- Fan-out of 500 jobs each opening its own DB connection will exhaust Postgres; jobs must load data via a
  bounded pool (engine side) — specify the limit in the job payload.
- If the universe changes mid-run (new listing), counters must come from the enqueued set, not a live query.
