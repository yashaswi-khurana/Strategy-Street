# SS-002 — Migration Tool + Core Metadata Schema & Seeds

| | |
| --- | --- |
| Milestone | 0 — Foundations |
| Depends on | SS-001 |
| Blocks | SS-003, SS-005, SS-006, SS-021 |
| Size | ~2 days |
| Stack | PostgreSQL 16, golang-migrate |

## Goal

Create the reference-data spine — sectors, stocks, trading calendar — that candles, backtests and paper
trading all foreign-key into.

## Scope

**In scope**
- Versioned SQL migrations (`db/migrations/0001_metadata.up.sql` / `.down.sql`).
- Tables: `sectors`, `stocks`, `exchange_holidays`, `market_sessions` (special sessions/muhurat).
- Deterministic seed data: 12 sectors, ~50 NSE stocks with real `instrument_key`s, 2 years of NSE holidays.
- Go `internal/store/metadata` repository: `GetStockBySymbol`, `ListStocksBySector`, `IsTradingDay`, `SessionFor(date)`.
- `make migrate-up/down/status`; idempotent seeds (`make seed`).

**Out of scope**
- Candle tables (SS-003), corporate actions (SS-006).
- Upstox instrument-master sync (add as a note; hard-coded seeds are fine here).

## Small architecture

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

CREATE TABLE sectors (
  id              SERIAL PRIMARY KEY,
  name            TEXT NOT NULL UNIQUE,
  benchmark_index TEXT,                -- e.g. 'NIFTY IT'
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stocks (
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
CREATE INDEX stocks_sector_idx ON stocks(sector_id) WHERE is_active;

CREATE TABLE exchange_holidays (
  exchange TEXT NOT NULL,
  holiday  DATE NOT NULL,
  label    TEXT NOT NULL,
  PRIMARY KEY (exchange, holiday)
);

CREATE TABLE market_sessions (
  exchange     TEXT NOT NULL,
  session_date DATE NOT NULL,
  open_time    TIME NOT NULL,   -- 09:15
  close_time   TIME NOT NULL,   -- 15:30
  label        TEXT,            -- 'muhurat', 'special'
  PRIMARY KEY (exchange, session_date)
);
```

* All timestamps are `TIMESTAMPTZ`; the application always computes "today" in **Asia/Kolkata**, never UTC.
* `symbol` is the human/UI key; `instrument_key` is the Upstox key. Never key by name.
* Seeds live in `db/seeds/*.sql` and are `INSERT … ON CONFLICT DO NOTHING` so they're re-runnable.
