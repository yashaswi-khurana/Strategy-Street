# SS-004 — Continuous Aggregates for 5m → 1Y + Retention/Compression

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-003 |
| Blocks | SS-022 (backtests read high TFs), SS-030 (chart) |
| Size | ~2.5 days |
| Stack | TimescaleDB 2.15, SQL, Go (policy runner) |

## Goal

Derive every timeframe above 1m from `candles_1m`/`candles_1d` inside the database, so a live tick writes
once and thirteen timeframes stay consistent with zero application write amplification.

## Scope

**In scope**
- Continuous aggregates: `candles_5m`, `candles_10m`, `candles_15m`, `candles_30m`, `candles_1h`, `candles_3h`
  (source `candles_1m`), and `candles_1d` (source `candles_1m`, IST session-aligned).
- Monthly-anchored rollups `candles_1mo`, `candles_6mo`, `candles_1y` (source `candles_1d`) and
  `candles_15d` (source `candles_1d`, fixed 15-day buckets from a documented origin).
- Refresh policies per aggregate + `add_compression_policy` / `add_retention_policy` per the spec table.
- A Go `internal/store/candles` read path that resolves `tf → relation name`, so callers never hardcode.
- `make tf-verify` SQL script that asserts every aggregate equals a from-scratch `time_bucket` recomputation.

**Out of scope**
- Backfilling from Upstox REST (SS-009) — aggregates are fed by whatever is in `candles_1m`.
- Chart rendering (SS-030).

## Small architecture

```sql
CREATE MATERIALIZED VIEW candles_5m
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '5 minutes', bucket_time) AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)               AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted
FROM candles_1m
GROUP BY stock_id, 1
WITH NO DATA;

SELECT add_continuous_aggregate_policy('candles_5m',
  start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute');
```

Key decisions to encode:
* **Bucket origin is IST-aligned.** `time_bucket` on a UTC column starts at the UTC epoch, which puts
  1h/3h/1d boundaries at the wrong wall-clock time for NSE. Use
  `time_bucket(INTERVAL '1 hour', bucket_time, TIMESTAMPTZ '1970-01-01 00:00:00+05:30')` (origin argument)
  for every timeframe, and for `1d` anchor to `09:15 IST`.
* **`end_offset`** must exceed the refresh latency but stay under the widest source lag; document the choice
  in an ADR and keep all policies in one SQL file so they can be tuned together.
* **15d / 6mo / 1y** are not ISO-aligned; define explicit origins (`2015-01-01 IST` for 15d, fiscal-quarter
  start for 6mo, January for 1y) and expose `Timeframe.Align()` in Go using the same constants.
* Read path: `SELECT … FROM candles_5m WHERE stock_id=$1 AND bucket_time >= $2 AND bucket_time < $3` — same
  repository API as raw tables, so backtests and charts are timeframe-agnostic.
* Data-quality guard: a view `v_candle_gaps` listing (stock_id, tf, missing_bucket) for monitoring.

## Tasks

- [ ] Migration `0003_continuous_aggregates.{up,down}.sql` with all aggregates, origins, `WITH NO DATA`.
- [ ] Policies migration `0004_policies.{up,down}.sql`: refresh, compression (segmentby `stock_id`,
      orderby `bucket_time DESC`), retention (1s: 7d, 1m: 30d, → 1d permanent).
- [ ] Go: timeframe→relation resolver + `Range` working for aggregates; unit test asserting every enum value
      maps to an existing relation.
- [ ] `make tf-verify` script comparing aggregate output to direct `time_bucket` SQL for a sample stock.
- [ ] ADR `docs/adr/0004-bucket-origins.md` documenting every origin constant.
- [ ] Seed ~3 months of synthetic 1m data generator for tests (`make seed-candles`).

## Acceptance criteria

1. After `make tf-verify`, all 13 timeframes are byte-identical to the from-scratch computation for the
   sample stock and date range.
2. `candles_1d` bars start at 09:15 IST and end at 15:30 IST for intraday-only stocks; a 1d bar's
   `volume` equals the sum of its 375 1m bars.
3. Inserting a new 1m bar becomes visible in `candles_5m` after at most one policy run (test with
   `CALL refresh_continuous_aggregate(...)` in the test).
4. `1h` buckets land on 09:15, 10:15, 11:15, … IST (asserted explicitly).
5. Retention/compression policies appear in `timescaledb_information.jobs`; `1d`+ have no retention job.

## Test plan

- Integration test inserting a known 1m fixture and asserting every timeframe's OHLCV.
- Boundary test around 15:30 IST close (no daily bar spanning overnight).
- Empty-input test: no rows → aggregates return zero rows, not errors.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.5 and §5.2 (write amplification, timeframe table),
docs/issues/SS-004-continuous-aggregates.md. Repo already has: candles_1s, candles_1m hypertables with
PK (stock_id, bucket_time), IST-aligned timeframe enum with Align() in Go, golang-migrate migrations,
TimescaleDB 2.15.3.

Implement SS-004: all higher timeframes as TimescaleDB continuous aggregates sourced from candles_1m
(5m,10m,15m,30m,1h,3h,1d) and from candles_1d (15d,1mo,6mo,1y). Every aggregate must use an explicit IST
bucket origin (1970-01-01T00:00:00+05:30 for intraday; 09:15 IST anchor for 1d; documented fixed origins for
15d/6mo/1y) so buckets align with NSE wall-clock boundaries. Use first(open,bucket_time), max(high),
min(low), last(close,bucket_time), sum(volume), last(oi,bucket_time), bool_and(is_adjusted). Create refresh
policies with documented start_offset/end_offset, compression policies (segmentby stock_id, orderby
bucket_time DESC) and retention policies exactly per the spec table (1s 7d, 1m 30d, 1d+ permanent, no
retention on daily+).

Extend the Go candle repository with a tf->relation resolver so Range/Latest work identically for any
timeframe; add a unit test asserting all 13 enum values resolve to an existing relation. Add a
`make tf-verify` script that compares each aggregate against a from-scratch time_bucket computation, and a
synthetic 1m data seeder for tests.

Constraints: application code must never INSERT into 5m+ relations; no Upstox calls in this issue; all bucket
origins must live in one ADR and be mirrored by Go constants.

Done when: tf-verify passes for all 13 timeframes, 1h buckets land on :15 IST, a daily bar's volume equals the
sum of its 375 minute bars, and refresh/compression/retention jobs are visible in
timescaledb_information.jobs. Provide the SQL output you verified against.
```

## Notes / risks

- Continuous aggregates cannot use `time_bucket` with a variable origin per stock — origins are global
  constants, so document them once and reuse.
- If `1mo/6mo/1y` prove fragile with caggs on `candles_1d`, fall back to a scheduled job writing into plain
  tables; the repository interface must not care.
