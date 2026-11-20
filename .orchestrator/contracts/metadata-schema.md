# CONTRACT: metadata-schema v1   status: FROZEN
- producer: db-engineer   consumers: backend-engineer, devops-engineer, qa-tester
- shared paths: db/migrations/**, api/internal/store/metadata/**

## Interface & Schema Specification

### 1. sectors Table
```sql
CREATE TABLE sectors (
  id              SERIAL PRIMARY KEY,
  name            TEXT NOT NULL UNIQUE,
  benchmark_index TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 2. stocks Table
```sql
CREATE TABLE stocks (
  id             SERIAL PRIMARY KEY,
  instrument_key TEXT NOT NULL UNIQUE, -- 'NSE_EQ|<ISIN>'
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
```

### 3. exchange_holidays Table
```sql
CREATE TABLE exchange_holidays (
  exchange TEXT NOT NULL,
  holiday  DATE NOT NULL,
  label    TEXT NOT NULL,
  PRIMARY KEY (exchange, holiday)
);
```

### 4. market_sessions Table
```sql
CREATE TABLE market_sessions (
  exchange     TEXT NOT NULL,
  session_date DATE NOT NULL,
  open_time    TIME NOT NULL,   -- e.g. 09:15:00
  close_time   TIME NOT NULL,   -- e.g. 15:30:00
  label        TEXT,            -- 'muhurat', 'special'
  PRIMARY KEY (exchange, session_date)
);
```

## Go Repository Interface (`MetadataStore`)
```go
type MetadataStore interface {
    GetStockBySymbol(ctx context.Context, symbol string) (*Stock, error)
    GetStockByInstrumentKey(ctx context.Context, instrumentKey string) (*Stock, error)
    ListStocksBySector(ctx context.Context, sector string) ([]*Stock, error)
    ListSectors(ctx context.Context) ([]*Sector, error)
    IsTradingDay(ctx context.Context, date time.Time) (bool, error)
    SessionFor(ctx context.Context, date time.Time) (*MarketSession, error)
}
```

## Errors and Edge Cases
- Missing entities MUST return a typed `*NotFoundError` matching `errors.Is(err, ErrNotFound)`.
- All timestamps evaluate and convert to `Asia/Kolkata` (`time.LoadLocation("Asia/Kolkata")`).
- Special market sessions (e.g. Muhurat) override standard weekend or holiday closures.

## Change Rule
Only the Orchestrator edits this file. A change bumps the version and issues a CHANGE directive to every consumer.

