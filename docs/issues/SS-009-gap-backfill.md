# SS-009 — Gap Detection & Smart Backfill Worker

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-005, SS-008 |
| Blocks | SS-022 (backtests need complete history), SS-030 (chart) |
| Size | ~2 days |
| Stack | Go, PostgreSQL |

## Goal

When a user opens a chart or a backtest needs history, fill only the missing sessions — never re-fetch what is
already stored, never hammer the 2 RPS budget blindly.

## Scope

**In scope**
- `internal/backfill`:
  - `Plan(ctx, stockID, tf, from, to) ([]Gap, error)` — session-aware gap detection using
    `candles.LatestTime` plus `marketcal.TradingDays` (skips weekends/holidays).
  - `Run(ctx, plan, priority) error` — fetches gaps via SS-008 and upserts via SS-003.
  - `EnsureFresh(ctx, stockID, tf, priority)` — "make this symbol/timeframe current", used on chart open.
  - Durable job table `backfill_jobs` (stock_id, tf, from, to, priority, status, attempts, last_error) so work
    survives restarts and is resumable.
- Deduplication: one in-flight job per `(stock_id, tf, range)`; callers join the existing job instead of
  creating a second.
- Circuit breaker: 5 consecutive Upstox failures → pause queue for 60 s, log, emit metric.
- CLI `make backfill --symbol=TCS --tf=5m --from=2024-01-01`.

**Out of scope**
- Streaming (SS-010/011). Aggregate recomputation (handled by SS-004 policies automatically).

## Small architecture

```text
              ┌─────────────── backfill.Scheduler (single goroutine) ──────────────┐
EnsureFresh ─►│ dedupe(in-flight) → enqueue(job, priority) → worker pool(2)       │
Plan         ─►│     worker: Fetch(gaps, P) → UpsertBatch(tf) → update job row    │
              └──────────────┬───────────────────────────────────────────────────┘
                             ▼
                      SS-008 client (2 RPS, priority deques)  →  SS-003 CandleStore
```

* A `Gap` is `{From, To time.Time, Reason string}`; generation walks expected trading sessions and compares
  with contiguous ranges present in the DB. Use a SQL-side gap query
  (`generate_series` over expected buckets `EXCEPT` stored buckets) for correctness, not app-side diffing.
* Backfill writes to `candles_1m` only; higher timeframes refresh via SS-004 policies. Exception: `1s` cannot
  be backfilled from Upstox — document that 1s history is live-only and `Plan` returns
  `ErrNotBackfillable` for `tf=1s`.
* `1d` and above are fetched directly from the vendor daily endpoint (per spec), then stored in `candles_1d`;
  monthly aggregates derive from them.
* Progressive disclosure to the UI: job progress (`bars_fetched / expected`) is published on the event bus so
  SS-012 can push it to the chart ("loading 3 400 bars…").

## Tasks

- [ ] `backfill_jobs` migration + repository (claim with `FOR UPDATE SKIP LOCKED`).
- [ ] SQL gap query per timeframe + Go `Plan`.
- [ ] Scheduler with dedupe, 2 workers, resume-on-restart, attempts/backoff.
- [ ] Circuit breaker + metrics (`backfill_jobs_total{status}`, gaps filled, bars written).
- [ ] CLI + `EnsureFresh` wired to the HTTP handler `GET /api/v1/candles/:symbol?tf=…` (returns cached data
      immediately and 202-style `backfill_pending` metadata when a gap is being filled).
- [ ] ADR `docs/adr/0009-backfill-scope.md` (why 1s is live-only, how far back each tf can go).

## Acceptance criteria

1. DB has 1m bars for 2024-01-01…2024-01-31 except 2024-01-15; `Plan` returns exactly that one-day gap.
2. A gap spanning a weekend/holiday requires no requests for those non-trading days (asserted from the fake
   server's request list).
3. Two simultaneous `EnsureFresh` calls for the same symbol/tf issue **one** vendor request set (dedupe).
4. Kill the worker mid-job, restart, and the job resumes from its recorded range and completes.
5. `tf=1s` returns `ErrNotBackfillable` and creates no job.
6. A pathologically gappy DB (50% missing) still converges with ≤ 2 requests per missing month of 1m data.

## Test plan

- Fake Upstox server + real Timescale container; seed a deliberately holed dataset.
- Restart/resume test with a job interrupted by context cancel.
- Concurrency test asserting dedupe (request counter == 1).

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §7 (smart gap backfill),
docs/issues/SS-009-gap-backfill.md. Repo: Go 1.23 + pgx + TimescaleDB; SS-003 gives CandleStore
(UpsertBatch/Range/LatestTime), SS-004 gives aggregates, SS-005 gives marketcal.TradingDays/IsOpen,
SS-008 gives the 2 RPS Upstox historical client with priority levels P1UI/P2PaperWarmup/P3Background.

Implement SS-009 only: internal/backfill.
1. Plan(ctx, stockID, tf, from, to) returns []Gap computed with a SQL gap query that generate_series the
   expected buckets (session/holiday aware) EXCEPT stored buckets — do not diff in Go. Gaps must never fall on
   weekends or seeded holidays. Return ErrNotBackfillable for tf=1s (1s history is live-only).
2. Run/EnsureFresh fetch each gap via the SS-008 client at the requested priority and persist with
   CandleStore.UpsertBatch into candles_1m (or candles_1d for daily+). Higher timeframes are derived by the
   SS-004 continuous aggregates; never write them here.
3. A backfill_jobs table (stock_id, tf, from, to, priority, status, attempts, last_error) with claim via
   SELECT ... FOR UPDATE SKIP LOCKED, resume after restart, and exponential backoff on failure.
4. Deduplicate: concurrent callers for the same (stock_id, tf, range) join the in-flight job rather than
   issuing new vendor calls.
5. Circuit breaker: 5 consecutive vendor failures pause the queue for 60s with a metric and log.
6. GET /api/v1/candles/:symbol?tf=... returns stored candles immediately and reports backfill_pending progress
   metadata; a CLI `make backfill --symbol --tf --from` drives one-off jobs. Publish progress events for the
   websocket layer.

Constraints: no direct net/http to Upstox; never invent bars for non-trading days; idempotent re-runs;
all decisions documented in ADR 0009.

Done when: a single mid-month hole is detected exactly, weekends/holidays produce zero vendor requests, two
concurrent EnsureFresh calls produce one vendor request set, an interrupted job resumes and completes, tf=1s
returns ErrNotBackfillable, and 50%-gappy data converges within 2 requests per missing month. Show the fake
server request log as evidence.
```

## Notes / risks

- Vendor history depth is limited (1m often only ~1 year); record the earliest available date per
  (stock, tf) in `backfill_jobs` metadata so `Plan` stops asking for impossible ranges.
