-- SS-006: Corporate Actions, Adjustment Factors, and Query-Time Adjusted Candles

-- 1. Corporate Actions Table
CREATE TABLE IF NOT EXISTS corporate_actions (
  id           BIGSERIAL PRIMARY KEY,
  stock_id     INT NOT NULL REFERENCES stocks(id),
  action_type  TEXT NOT NULL CHECK (action_type IN ('split','bonus','dividend','rights')),
  ex_date      DATE NOT NULL,
  ratio_from   NUMERIC(12,4),   -- split 1:5 => from=1, to=5
  ratio_to     NUMERIC(12,4),
  dividend_amt NUMERIC(12,4),   -- cash dividend in INR
  source       TEXT NOT NULL,   -- e.g. 'NSE_OFFICIAL', 'MANUAL_SEED'
  verified     BOOLEAN NOT NULL DEFAULT FALSE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stock_id, action_type, ex_date)
);

CREATE INDEX IF NOT EXISTS corporate_actions_stock_ex_date_idx ON corporate_actions (stock_id, ex_date);

-- 2. Adjustment Factors Table (Cumulative Backward Factors)
CREATE TABLE IF NOT EXISTS adjustment_factors (
  stock_id INT NOT NULL REFERENCES stocks(id),
  ex_date  DATE NOT NULL,
  factor   NUMERIC(18,10) NOT NULL,   -- multiply prices strictly before ex_date
  PRIMARY KEY (stock_id, ex_date)
);

CREATE INDEX IF NOT EXISTS adjustment_factors_stock_ex_date_idx ON adjustment_factors (stock_id, ex_date DESC);

-- 3. Data Quality Issues Table
CREATE TABLE IF NOT EXISTS data_quality_issues (
  id          BIGSERIAL PRIMARY KEY,
  stock_id    INT NOT NULL REFERENCES stocks(id),
  issue_type  TEXT NOT NULL,           -- 'unexplained_jump', 'unverified_action'
  check_date  DATE NOT NULL,
  details     JSONB NOT NULL DEFAULT '{}'::jsonb,
  resolved    BOOLEAN NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stock_id, issue_type, check_date)
);

CREATE INDEX IF NOT EXISTS data_quality_issues_stock_idx ON data_quality_issues (stock_id, resolved);

-- 4. adjusted_candles SQL Function
-- Dynamically routes to the corresponding timeframe hypertable or continuous aggregate,
-- multiplying OHLC by the cumulative backward factor while preserving volume.
CREATE OR REPLACE FUNCTION adjusted_candles(
  p_stock INT,
  p_tf TEXT,
  p_from TIMESTAMPTZ,
  p_to TIMESTAMPTZ
)
RETURNS TABLE (
  bucket_time TIMESTAMPTZ,
  open NUMERIC,
  high NUMERIC,
  low NUMERIC,
  close NUMERIC,
  volume BIGINT
)
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
  v_rel TEXT;
BEGIN
  -- Strict validation of timeframe relations
  CASE p_tf
    WHEN '1s'  THEN v_rel := 'candles_1s';
    WHEN '1m'  THEN v_rel := 'candles_1m';
    WHEN '5m'  THEN v_rel := 'candles_5m';
    WHEN '10m' THEN v_rel := 'candles_10m';
    WHEN '15m' THEN v_rel := 'candles_15m';
    WHEN '30m' THEN v_rel := 'candles_30m';
    WHEN '1h'  THEN v_rel := 'candles_1h';
    WHEN '3h'  THEN v_rel := 'candles_3h';
    WHEN '1d'  THEN v_rel := 'candles_1d';
    WHEN '15d' THEN v_rel := 'candles_15d';
    WHEN '1mo' THEN v_rel := 'candles_1mo';
    WHEN '6mo' THEN v_rel := 'candles_6mo';
    WHEN '1y'  THEN v_rel := 'candles_1y';
    ELSE
      RAISE EXCEPTION 'unsupported timeframe %', p_tf;
  END CASE;

  RETURN QUERY EXECUTE format(
    'SELECT
       c.bucket_time,
       ROUND((c.open * COALESCE(adj.factor, 1.0))::NUMERIC, 4) AS open,
       ROUND((c.high * COALESCE(adj.factor, 1.0))::NUMERIC, 4) AS high,
       ROUND((c.low * COALESCE(adj.factor, 1.0))::NUMERIC, 4) AS low,
       ROUND((c.close * COALESCE(adj.factor, 1.0))::NUMERIC, 4) AS close,
       c.volume
     FROM %I c
     LEFT JOIN LATERAL (
       SELECT af.factor
       FROM adjustment_factors af
       WHERE af.stock_id = $1
         AND af.ex_date > (c.bucket_time AT TIME ZONE ''Asia/Kolkata'')::DATE
       ORDER BY af.ex_date ASC
       LIMIT 1
     ) adj ON TRUE
     WHERE c.stock_id = $1
       AND c.bucket_time >= $2
       AND c.bucket_time <= $3
     ORDER BY c.bucket_time ASC',
    v_rel
  ) USING p_stock, p_from, p_to;
END;
$$;

