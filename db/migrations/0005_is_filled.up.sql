-- SS-005: Add is_filled column to candles_1s, candles_1m and update continuous aggregates

-- 1. Add is_filled to raw hypertable candles_1s
ALTER TABLE candles_1s ADD COLUMN IF NOT EXISTS is_filled BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Add is_filled to raw hypertable candles_1m
ALTER TABLE candles_1m ADD COLUMN IF NOT EXISTS is_filled BOOLEAN NOT NULL DEFAULT FALSE;

-- 3. Recreate continuous aggregates with bool_and(is_filled) AS is_filled
-- Drop existing continuous aggregates in reverse dependency order
DROP VIEW IF EXISTS v_candle_gaps CASCADE;

DROP MATERIALIZED VIEW IF EXISTS candles_1y CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_6mo CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_1mo CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_15d CASCADE;

DROP MATERIALIZED VIEW IF EXISTS candles_1d CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_3h CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_1h CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_30m CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_15m CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_10m CASCADE;
DROP MATERIALIZED VIEW IF EXISTS candles_5m CASCADE;

-- Intraday Continuous Aggregates (source: candles_1m)
-- Origin: TIMESTAMPTZ '2015-01-01 09:15:00+05:30'

-- 1. candles_5m
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_5m
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '5 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '5 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_5m_stock_bucket_desc_idx ON candles_5m (stock_id, bucket_time DESC);

-- 2. candles_10m
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_10m
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '10 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '10 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_10m_stock_bucket_desc_idx ON candles_10m (stock_id, bucket_time DESC);

-- 3. candles_15m
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_15m
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '15 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '15 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_15m_stock_bucket_desc_idx ON candles_15m (stock_id, bucket_time DESC);

-- 4. candles_30m
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_30m
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '30 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '30 minutes', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_30m_stock_bucket_desc_idx ON candles_30m (stock_id, bucket_time DESC);

-- 5. candles_1h
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_1h
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '1 hour', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '1 hour', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_1h_stock_bucket_desc_idx ON candles_1h (stock_id, bucket_time DESC);

-- 6. candles_3h
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_3h
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '3 hours', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '3 hours', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_3h_stock_bucket_desc_idx ON candles_3h (stock_id, bucket_time DESC);

-- 7. candles_1d (session-aligned daily bar from 1m)
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_1d
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '1 day', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1m
GROUP BY stock_id, time_bucket(INTERVAL '1 day', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_1d_stock_bucket_desc_idx ON candles_1d (stock_id, bucket_time DESC);

-- Macro Continuous Aggregates (source: candles_1d)
-- Shares origin '2015-01-01 09:15:00+05:30' for hierarchical rollup compatibility.

-- 8. candles_15d
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_15d
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '15 days', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1d
GROUP BY stock_id, time_bucket(INTERVAL '15 days', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_15d_stock_bucket_desc_idx ON candles_15d (stock_id, bucket_time DESC);

-- 9. candles_1mo
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_1mo
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '1 month', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1d
GROUP BY stock_id, time_bucket(INTERVAL '1 month', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_1mo_stock_bucket_desc_idx ON candles_1mo (stock_id, bucket_time DESC);

-- 10. candles_6mo
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_6mo
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '6 months', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1d
GROUP BY stock_id, time_bucket(INTERVAL '6 months', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_6mo_stock_bucket_desc_idx ON candles_6mo (stock_id, bucket_time DESC);

-- 11. candles_1y
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_1y
WITH (timescaledb.continuous) AS
SELECT stock_id,
       time_bucket(INTERVAL '1 year', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
       first(open, bucket_time)  AS open,
       max(high)                 AS high,
       min(low)                  AS low,
       last(close, bucket_time)  AS close,
       sum(volume)::BIGINT       AS volume,
       last(oi, bucket_time)     AS oi,
       bool_and(is_adjusted)     AS is_adjusted,
       bool_and(is_filled)       AS is_filled
FROM candles_1d
GROUP BY stock_id, time_bucket(INTERVAL '1 year', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
WITH NO DATA;

CREATE INDEX IF NOT EXISTS candles_1y_stock_bucket_desc_idx ON candles_1y (stock_id, bucket_time DESC);

-- Recreate Gap Detection View
CREATE OR REPLACE VIEW v_candle_gaps AS
SELECT stock_id,
       '1m'::TEXT AS tf,
       bucket_time + INTERVAL '1 minute' AS missing_bucket
FROM (
  SELECT stock_id,
         bucket_time,
         LEAD(bucket_time) OVER (PARTITION BY stock_id ORDER BY bucket_time) AS next_bucket
  FROM candles_1m
) t
WHERE next_bucket IS NOT NULL
  AND next_bucket > bucket_time + INTERVAL '1 minute'
  AND (bucket_time AT TIME ZONE 'Asia/Kolkata')::DATE = (next_bucket AT TIME ZONE 'Asia/Kolkata')::DATE
  AND (next_bucket AT TIME ZONE 'Asia/Kolkata')::TIME <= '15:30:00'::TIME;

-- Re-apply continuous aggregate refresh policies
SELECT add_continuous_aggregate_policy('candles_5m',
  start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_10m',
  start_offset => INTERVAL '4 hours', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_15m',
  start_offset => INTERVAL '6 hours', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_30m',
  start_offset => INTERVAL '12 hours', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_1h',
  start_offset => INTERVAL '24 hours', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_3h',
  start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_1d',
  start_offset => INTERVAL '7 days', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_15d',
  start_offset => INTERVAL '30 days', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '15 minutes', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_1mo',
  start_offset => INTERVAL '60 days', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 hour', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_6mo',
  start_offset => INTERVAL '365 days', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '6 hours', if_not_exists => true);

SELECT add_continuous_aggregate_policy('candles_1y',
  start_offset => INTERVAL '730 days', end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '12 hours', if_not_exists => true);

-- Enable TimescaleDB compression on intraday continuous aggregates
ALTER MATERIALIZED VIEW candles_5m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);

ALTER MATERIALIZED VIEW candles_10m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);

ALTER MATERIALIZED VIEW candles_15m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);

ALTER MATERIALIZED VIEW candles_30m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);

SELECT add_compression_policy('candles_5m', INTERVAL '90 days', if_not_exists => true);
SELECT add_compression_policy('candles_10m', INTERVAL '90 days', if_not_exists => true);
SELECT add_compression_policy('candles_15m', INTERVAL '90 days', if_not_exists => true);
SELECT add_compression_policy('candles_30m', INTERVAL '180 days', if_not_exists => true);

