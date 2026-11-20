-- SS-004: Remove retention, compression, and refresh policies

-- 1. Remove retention policies
SELECT remove_retention_policy('candles_1s', if_exists => true);
SELECT remove_retention_policy('candles_1m', if_exists => true);

-- 2. Remove compression policies
SELECT remove_compression_policy('candles_1s', if_exists => true);
SELECT remove_compression_policy('candles_1m', if_exists => true);
SELECT remove_compression_policy('candles_5m', if_exists => true);
SELECT remove_compression_policy('candles_10m', if_exists => true);
SELECT remove_compression_policy('candles_15m', if_exists => true);
SELECT remove_compression_policy('candles_30m', if_exists => true);

-- 3. Remove continuous aggregate refresh policies
SELECT remove_continuous_aggregate_policy('candles_5m', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_10m', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_15m', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_30m', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_1h', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_3h', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_1d', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_15d', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_1mo', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_6mo', if_exists => true);
SELECT remove_continuous_aggregate_policy('candles_1y', if_exists => true);

