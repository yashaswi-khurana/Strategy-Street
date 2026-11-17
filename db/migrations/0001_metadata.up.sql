-- SS-002: Core Metadata Schema (Sectors, Stocks, Exchange Holidays, Market Sessions)
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

CREATE TABLE IF NOT EXISTS sectors (
  id              SERIAL PRIMARY KEY,
  name            TEXT NOT NULL UNIQUE,
  benchmark_index TEXT,                -- e.g. 'NIFTY IT'
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS stocks (
  id             SERIAL PRIMARY KEY,
  instrument_key TEXT NOT NULL UNIQUE, -- 'NSE_EQ|INE002A01018'
  symbol         TEXT NOT NULL UNIQUE, -- 'RELIANCE'
  company_name   TEXT NOT NULL,
  exchange       TEXT NOT NULL DEFAULT 'NSE',
  sector_id      INT REFERENCES sectors(id),
  lot_size       INT NOT NULL DEFAULT 1,
  tick_size      NUMERIC(10,4) NOT NULL DEFAULT 0.05,
  is_active      BOOLEAN NOT NULL DEFAULT TRUE,
  listing_date   DATE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS stocks_sector_idx ON stocks(sector_id) WHERE is_active;

CREATE TABLE IF NOT EXISTS exchange_holidays (
  exchange TEXT NOT NULL,
  holiday  DATE NOT NULL,
  label    TEXT NOT NULL,
  PRIMARY KEY (exchange, holiday)
);

CREATE TABLE IF NOT EXISTS market_sessions (
  exchange     TEXT NOT NULL,
  session_date DATE NOT NULL,
  open_time    TIME NOT NULL,   -- 09:15
  close_time   TIME NOT NULL,   -- 15:30
  label        TEXT,            -- 'muhurat', 'special'
  PRIMARY KEY (exchange, session_date)
);
