-- SS-003: Candle Schema, Hypertables & Indexing

-- 1. candles_1s table (raw 1-second interval)
CREATE TABLE IF NOT EXISTS candles_1s (
  stock_id    INT           NOT NULL REFERENCES stocks(id),
  bucket_time TIMESTAMPTZ   NOT NULL,
  open        NUMERIC(12,4) NOT NULL,
  high        NUMERIC(12,4) NOT NULL,
  low         NUMERIC(12,4) NOT NULL,
  close       NUMERIC(12,4) NOT NULL,
  volume      BIGINT        NOT NULL DEFAULT 0,
  oi          BIGINT        NOT NULL DEFAULT 0,
  is_adjusted BOOLEAN       NOT NULL DEFAULT FALSE,
  PRIMARY KEY (stock_id, bucket_time)
);

SELECT create_hypertable('candles_1s', 'bucket_time', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS candles_1s_stock_bucket_desc_idx ON candles_1s (stock_id, bucket_time DESC);

-- Enable TimescaleDB compression settings on candles_1s (compression-ready)
ALTER TABLE candles_1s SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);

-- 2. candles_1m table (raw 1-minute interval)
CREATE TABLE IF NOT EXISTS candles_1m (
  stock_id    INT           NOT NULL REFERENCES stocks(id),
  bucket_time TIMESTAMPTZ   NOT NULL,
  open        NUMERIC(12,4) NOT NULL,
  high        NUMERIC(12,4) NOT NULL,
  low         NUMERIC(12,4) NOT NULL,
  close       NUMERIC(12,4) NOT NULL,
  volume      BIGINT        NOT NULL DEFAULT 0,
  oi          BIGINT        NOT NULL DEFAULT 0,
  is_adjusted BOOLEAN       NOT NULL DEFAULT FALSE,
  PRIMARY KEY (stock_id, bucket_time)
);

SELECT create_hypertable('candles_1m', 'bucket_time', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS candles_1m_stock_bucket_desc_idx ON candles_1m (stock_id, bucket_time DESC);

-- Enable TimescaleDB compression settings on candles_1m (compression-ready)
ALTER TABLE candles_1m SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'stock_id',
  timescaledb.compress_orderby = 'bucket_time DESC'
);

-- Compression policy placeholder (to be scheduled in retention lifecycle):
-- SELECT add_compression_policy('candles_1s', INTERVAL '7 days');
-- SELECT add_compression_policy('candles_1m', INTERVAL '30 days');
