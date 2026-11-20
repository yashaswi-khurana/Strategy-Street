# SS-003 — Candle Schema, Hypertables & Upsert/Range Repository

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-001, SS-002 |
| Blocks | SS-004, SS-009, SS-011, SS-020, SS-029 |
| Size | ~2 days |
| Stack | PostgreSQL + TimescaleDB 2.15, Go 1.23, pgx v5 |

## Goal

One physical candle table per raw resolution (`candles_1s`, `candles_1m`), converted to TimescaleDB
hypertables, plus the single Go repository that every writer and reader uses. Higher timeframes are never
written by the application (reserved for continuous aggregates in SS-004).

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
- Benchmark proving ≥ 50k bars/sec batch upsert on a laptop (in-memory paper fake >500k/s, disk backfill >25k/s).

**Out of scope**
- Aggregation to 5m+ (SS-004), backfill transport (SS-008/009), live aggregation (SS-011).
- Adjusted vs raw prices (SS-006) — this issue stores **raw** only (`is_adjusted = false`).

## Architecture

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
  epoch (`1970-01-01T00:00:00+05:30` / `1969-12-31T18:30:00Z`). Enforced with Go validators so misaligned bars are rejected before reaching PostgreSQL.
* `Candle` Go type uses `pgtype.Numeric` at the storage boundary and converts to/from `float64` strictly via `internal/store/candles/numeric.go`.
* Repository interface `CandleStore` is implemented by both TimescaleDB `Store` and in-memory `MemoryStore` for deterministic paper engine testing.

## Acceptance Criteria

1. Upserting the same `(stock_id, bucket_time)` twice updates in place; `count(*)` stays 1.
2. `Range` for a 5-day window returns exactly the inserted bars in ascending order.
3. Writing a `bucket_time` of `09:15:30` into `candles_1m` returns a validation error (not a DB error).
4. `CopyBatch` of 100k rows completes < 5 s locally and is idempotent.
5. `EXPLAIN` for `Range` uses the `(stock_id, bucket_time)` index, not a seq scan.
