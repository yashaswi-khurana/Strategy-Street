# SS-003 — Candle Schema, Hypertables & Upsert/Range Repository

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-002 |
| Blocks | SS-004, SS-009, SS-011, SS-020, SS-029 |
| Size | ~2 days |
| Stack | PostgreSQL + TimescaleDB, Go |

## Goal

One physical candle table per raw resolution (`candles_1s`, `candles_1m`), converted to TimescaleDB
hypertables, plus the single Go repository that every writer and reader uses. Higher timeframes are never
written by the application (that is SS-004).

## Scope

**In scope**
- Migration creating `candles_1s` and `candles_1m` with the common candle schema from spec §5.2.
- `create_hypertable` with 1-day chunks, `(stock_id, bucket_time)` ordering, compression-ready.
- Unique constraint `(stock_id, bucket_time)` on both.
- Go `internal/store/candles`:
  - `UpsertBatch(ctx, tf, bars []Candle) error` (single multi-row `INSERT … ON CONFLICT DO UPDATE`).
  - `Range(ctx, stockID, tf, from, to) ([]Candle, error)` — inclusive/exclusive documented.
  - `Latest(ctx, stockID, tf) (Candle, error)` and `LatestTime(ctx, stockID, tf) (time.Time, error)`.
  - `CopyBatch` using `pgx.CopyFrom` into a temp table + merge, for high-volume backfill.
- Benchmark proving ≥ 50k bars/sec batch upsert on a laptop.

**Out of scope**
- Aggregation to 5m+ (SS-004), backfill transport (SS-008/009), live aggregation (SS-011).
- Adjusted vs raw prices (SS-006) — this issue stores **raw** only (`is_adjusted = false`).

## Small architecture

```sql
CREATE TABLE candles_1m (
  stock_id    INT         NOT NULL REFERENCES stocks(id),
  bucket_time TIMESTAMPTZ NOT NULL,
  open        NUMERIC(12,4) NOT NULL,
  high        NUMERIC(12,4) NOT NULL,
  low         NUMERIC(12,4) NOT NULL,
  close       NUMERIC(12,4) NOT NULL,
  volume      BIGINT        NOT NULL DEFAULT 0,
  oi          BIGINT        NOT NULL DEFAULT 0,
  is_adjusted BOOLEAN       NOT NULL DEFAULT FALSE,
  PRIMARY KEY (stock_id, bucket_time)
);
SELECT create_hypertable('candles_1m','bucket_time', chunk_time_interval => INTERVAL '1 day');

CREATE INDEX ON candles_1m (stock_id, bucket_time DESC);
```

* `bucket_time` is **bar-open time in UTC**, always an exact multiple of the timeframe from an IST-aligned
  epoch. Document the epoch (`1970-01-01T00:00:00+05:30`) and enforce it with `CHECK`/validator in Go so a
  misaligned bar can never be written.
* `Candle` Go type is the shared currency (`decimal` as `string` or `pgtype.Numeric` at the boundary,
  `float64` only inside Python). Pick `pgtype.Numeric` → `float64` conversion in one place.
* Repositories are interfaces (`CandleStore`) so the paper engine can be tested against an in-memory fake.

## Tasks

- [ ] Migration `0002_candles.{up,down}.sql` with both tables, hypertables, indexes, and later compression policy placeholder.
- [ ] `internal/store/candles/candle.go` — type, timeframe enum with `Duration()` and `Align(t)`.
- [ ] `UpsertBatch`, `Range`, `LatestTime`, `CopyBatch` implementations + interfaces.
- [ ] Alignment validator rejecting misaligned `bucket_time`.
- [ ] Table-driven tests + benchmark (`BenchmarkUpsertBatch`).
- [ ] ADR `docs/adr/0003-numeric-vs-float.md`.

## Acceptance criteria

1. Upserting the same `(stock_id, bucket_time)` twice updates in place; `count(*)` stays 1.
2. `Range` for a 5-day window returns exactly the inserted bars in ascending order.
3. Writing a `bucket_time` of `09:15:30` into `candles_1m` returns a validation error (not a DB error).
4. `CopyBatch` of 100k rows completes < 5 s locally and is idempotent.
5. `EXPLAIN` for `Range` uses the `(stock_id, bucket_time)` index, not a seq scan.

## Test plan

- testcontainers integration tests covering conflict-update, range boundaries, ordering.
- Property test: for random bar sets, `UpsertBatch` then `Range` round-trips identical values.
- Benchmark with 1m bars for 2 stocks × 60 trading days.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §5.2 (candle schema and timeframe table),
docs/issues/SS-003-candle-hypertables.md. Repo: Go 1.23 + chi + pgx v5, TimescaleDB 2.15.3 via
docker-compose, metadata tables sectors/stocks already exist (SS-002), golang-migrate is set up.

Implement SS-003 only. Add migration 0002_candles.{up,down}.sql creating candles_1s and candles_1m with
columns (stock_id INT FK, bucket_time TIMESTAMPTZ, open/high/low/close NUMERIC(12,4), volume BIGINT,
oi BIGINT, is_adjusted BOOLEAN) PRIMARY KEY (stock_id, bucket_time), create_hypertable with 1-day chunks,
and an index on (stock_id, bucket_time DESC). Use unadjusted data only in this issue.

Create Go package internal/store/candles with: a Timeframe enum (1s,1m,5m,10m,15m,30m,1h,3h,1d,15d,1mo,6mo,1y)
exposing Duration() and Align(t) against an IST-aligned epoch (1970-01-01T00:00:00+05:30); a Candle struct;
a CandleStore interface; and pgx implementations of UpsertBatch (one multi-row INSERT ... ON CONFLICT
(stock_id,bucket_time) DO UPDATE), Range, Latest, LatestTime, and CopyBatch (pgx.CopyFrom into a temp table
then merge). Reject misaligned bucket_time in Go before touching the DB.

Constraints: application never writes 5m or higher timeframes; no aggregation logic here; conversions
between NUMERIC and float64 must live in exactly one helper.

Done when: testcontainers tests prove conflict-update, idempotent CopyBatch of 100k rows, correct ascending
range results, and rejection of a 09:15:30 timestamp for the 1m timeframe; BenchmarkUpsertBatch is committed
with its numbers in the PR description; make test and make lint pass.
```

## Notes / risks

- `NUMERIC` is precise but slow; if backtests get hot, add a `float8` materialized read path later —
  do **not** change this schema mid-project.
- 1s retention grows fast (~25k bars/day/stock). Compression lands in SS-004; keep the table append-only
  and time-ordered so compression is effective.
