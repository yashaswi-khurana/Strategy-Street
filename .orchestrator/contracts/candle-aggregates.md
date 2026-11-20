# CONTRACT: candle-aggregates v1   status: FROZEN
- producer: db-engineer, backend-engineer   consumers: backtest-engine, chart-service, live-streamer
- shared paths: db/migrations/0003_*, db/migrations/0004_*, api/internal/store/candles/**

## Interface & Schema Specification

### 1. Continuous Aggregates (Materialized Views)
- Sourced from `candles_1m`:
  - `candles_5m`, `candles_10m`, `candles_15m`, `candles_30m`, `candles_1h`, `candles_3h`, `candles_1d`
- Sourced from `candles_1d` (hierarchical):
  - `candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y`
- Creation semantics: `WITH (timescaledb.continuous) ... WITH NO DATA`
- Unified bucket origin: `TIMESTAMPTZ '2015-01-01 09:15:00+05:30'` (UTC: `2015-01-01 03:45:00Z`)
- Composite index on all continuous aggregates: `(stock_id, bucket_time DESC)`

### 2. Timeframe Relation Mapping
- All 13 `Timeframe` enum values resolve to a queryable relation:
  `candles_1s`, `candles_1m`, `candles_5m`, `candles_10m`, `candles_15m`, `candles_30m`, `candles_1h`, `candles_3h`, `candles_1d`, `candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y`.
- `Timeframe.TableName()` and `Timeframe.RelationName()` return the valid relation without errors.
- `Timeframe.IsContinuousAggregate()` returns `true` for 5m through 1y.
- `Timeframe.IsWritable()` returns `true` ONLY for 1s and 1m.

### 3. Query Uniformity
- `CandleStore.Range(ctx, stockID, tf, from, to)` works identically across hypertables and continuous aggregates.
- Column types returned across all relations:
  `stock_id INT`, `bucket_time TIMESTAMPTZ`, `open NUMERIC(12,4)`, `high NUMERIC(12,4)`, `low NUMERIC(12,4)`, `close NUMERIC(12,4)`, `volume BIGINT`, `oi BIGINT`, `is_adjusted BOOLEAN`.

### 4. TimescaleDB Policy Constraints
- Refresh policies: configured on all continuous aggregates with sliding windows starting at 1m end-offset.
- Compression: enabled on raw tables (1s: 7d, 1m: 30d) and intraday caggs (5m: 90d, 10m: 90d, 15m: 90d, 30m: 180d) with `segmentby = 'stock_id'` and `orderby = 'bucket_time DESC'`.
- Retention: enabled on raw tables (1s: 7d, 1m: 30d). Daily and higher timeframes have NO retention (permanent).

