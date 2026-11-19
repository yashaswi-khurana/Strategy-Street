-- SS-004: Refresh, Compression, and Retention Policies for Candles

-- ============================================================================
-- 1. Continuous Aggregate Refresh Policies
-- ============================================================================

-- 1. candles_5m: refresh every 1m covering last 2 hours up to 1m ago
SELECT add_continuous_aggregate_policy('candles_5m',
  start_offset => INTERVAL '2 hours',
  end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute'
);

-- 2. candles_10m: refresh every 2m covering last 4 hours up to 1m ago
SELECT add_continuous_aggregate_policy('candles_10m',
  start_offset => INTERVAL '4 hours',
  end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '2 minutes'
);

-- 3. candles_15m: refresh every 2m covering last 6 hours up to 1m ago
SELECT add_continuous_aggregate_policy('candles_15m',
  start_offset => INTERVAL '6 hours',
  end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '2 minutes'
);

-- 4. candles_30m: refresh every 5m covering last 12 hours up to 1m ago
SELECT add_continuous_aggregate_policy('candles_30m',
  start_offset => INTERVAL '12 hours',
  end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '5 minutes'
);

-- 5. candles_1h: refresh every 10m covering last 24 hours up to 1m ago
SELECT add_continuous_aggregate_policy('candles_1h',
  start_offset => INTERVAL '24 hours',
  end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '10 minutes'
);

-- 6. candles_3h: refresh every 15m covering last 3 days up to 1m ago
SELECT add_continuous_aggregate_policy('candles_3h',
  start_offset => INTERVAL '3 days',
  end_offset => INTERVAL '1 minute',
  schedule_interval => INTERVAL '15 minutes'
);

-- 7. candles_1d: refresh every 1 hour covering last 7 days up to 1 hour ago
SELECT add_continuous_aggregate_policy('candles_1d',
  start_offset => INTERVAL '7 days',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour'
);

-- 8. candles_15d: refresh daily covering last 60 days up to 1 day ago
SELECT add_continuous_aggregate_policy('candles_15d',
  start_offset => INTERVAL '60 days',
  end_offset => INTERVAL '1 day',
  schedule_interval => INTERVAL '1 day'
);

-- 9. candles_1mo: refresh daily covering last 90 days up to 1 day ago
SELECT add_continuous_aggregate_policy('candles_1mo',
  start_offset => INTERVAL '90 days',
  end_offset => INTERVAL '1 day',
  schedule_interval => INTERVAL '1 day'
);

-- 10. candles_6mo: refresh daily covering last 500 days up to 1 day ago
SELECT add_continuous_aggregate_policy('candles_6mo',
  start_offset => INTERVAL '500 days',
  end_offset => INTERVAL '1 day',
  schedule_interval => INTERVAL '1 day'
);

-- 11. candles_1y: refresh daily covering last 1000 days up to 1 day ago
SELECT add_continuous_aggregate_policy('candles_1y',
  start_offset => INTERVAL '1000 days',
  end_offset => INTERVAL '1 day',
  schedule_interval => INTERVAL '1 day'
);

-- ============================================================================
-- 2. Compression Policies
-- ============================================================================

-- Raw hypertables compression (compression settings enabled in 0002_candles)
SELECT add_compression_policy('candles_1s', INTERVAL '7 days');
SELECT add_compression_policy('candles_1m', INTERVAL '30 days');

-- Intraday continuous aggregates compression
ALTER MATERIALIZED VIEW candles_5m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);
SELECT add_compression_policy('candles_5m', INTERVAL '90 days');

ALTER MATERIALIZED VIEW candles_10m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);
SELECT add_compression_policy('candles_10m', INTERVAL '90 days');

ALTER MATERIALIZED VIEW candles_15m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);
SELECT add_compression_policy('candles_15m', INTERVAL '90 days');

ALTER MATERIALIZED VIEW candles_30m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);
SELECT add_compression_policy('candles_30m', INTERVAL '180 days');

-- Note: candles_1h and candles_3h remain standard indexed (uncompressed) per spec §5.2.
-- candles_1d, candles_15d, candles_1mo, candles_6mo, candles_1y remain permanent uncompressed.

-- ============================================================================
-- 3. Retention Policies
-- ============================================================================

-- High-velocity tick and raw minute data retention
SELECT add_retention_policy('candles_1s', INTERVAL '7 days');
SELECT add_retention_policy('candles_1m', INTERVAL '30 days');

-- Daily and higher timeframes have NO retention policy (permanent historical archive).

