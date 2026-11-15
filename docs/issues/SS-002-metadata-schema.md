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
- Go `internal/store/metadata` repository: `GetStockBySymbol`, `ListStocksBySector`, `IsTradingDay`,
  `SessionFor(date)`.
- `make migrate-up/down/status`; idempotent seeds (`make seed`).

**Out of scope**
- Candle tables (SS-003), corporate actions (SS-006).
- Upstox instrument-master sync (add as a note; hard-coded seeds are fine here).

## Small architecture

```sql
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
  exchange   TEXT NOT NULL,
  session_date DATE NOT NULL,
  open_time  TIME NOT NULL,   -- 09:15
  close_time TIME NOT NULL,   -- 15:30
  label      TEXT,            -- 'muhurat', 'special'
  PRIMARY KEY (exchange, session_date)
);
```

* All timestamps are `TIMESTAMPTZ`; the application always computes "today" in **Asia/Kolkata**, never UTC.
* `symbol` is the human/UI key; `instrument_key` is the Upstox key. Never key by name.
* Seeds live in `db/seeds/*.sql` and are `INSERT … ON CONFLICT DO NOTHING` so they're re-runnable.

## Tasks

- [ ] Write up/down migrations for the four tables + indexes.
- [ ] Seed sectors (IT, Banking, Pharma, Auto, FMCG, Energy, Metals, Telecom, Realty, PSU Bank, Media, Infra).
- [ ] Seed ~50 stocks with correct `instrument_key`, sector, lot size, tick size.
- [ ] Seed NSE holidays for the current + next calendar year; add `market_sessions` rows for muhurat sessions.
- [ ] Go repository with typed structs, `pgx` and a small interface (`MetadataStore`) for test doubles.
- [ ] ADR `docs/adr/0002-symbol-vs-instrument-key.md`.

## Acceptance criteria

1. `make migrate-up` on an empty DB creates all tables; `down` cleanly removes them.
2. `make seed` twice leaves identical row counts (idempotent).
3. `SELECT count(*) FROM stocks WHERE sector_id IS NULL` → 0.
4. Repository test against a real container (`testcontainers`): lookup by symbol returns the right
   `instrument_key`; unknown symbol returns a typed `ErrNotFound`.
5. `IsTradingDay('2027-01-26')` → false (Republic Day), `IsTradingDay('2027-01-27')` → true, using DB data only.

## Test plan

- Migration up/down/up cycle in CI.
- Repository integration tests via testcontainers with the seed applied.
- A unit test for the IST date helper (UTC 20:00 → next IST day).

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §5.1 (metadata tables), docs/issues/SS-002-metadata-schema.md.
Repo already has: api/ (Go 1.23, pgx, chi), db/migrations wired to golang-migrate, deploy/docker-compose.yml
with timescaledb 2.15.3-pg16.

Implement SS-002 only (metadata schema + repository). Create migrations 0001_metadata.{up,down}.sql with
tables sectors, stocks, exchange_holidays, market_sessions exactly as specified in the issue (TIMESTAMPTZ,
Asia/Kolkata semantics, unique instrument_key and symbol, partial index on active stocks by sector).
Add db/seeds/*.sql that are idempotent (INSERT ... ON CONFLICT DO NOTHING) with 12 Indian sectors, ~50 real
NSE stocks with correct Upstox instrument_key format 'NSE_EQ|<ISIN>', lot size and tick size, plus NSE trading
holidays for 2026 and 2027 and two muhurat sessions.

Write Go package internal/store/metadata with structs, a MetadataStore interface, and pgx implementations of
GetStockBySymbol, ListStocksBySector, IsTradingDay, SessionFor. Return a typed ErrNotFound. All date logic
must convert to Asia/Kolkata explicitly.

Constraints: no candle tables, no corporate actions, no network calls. Migrations must be reversible and
safe to run twice. Seeds must not hardcode ids.

Done when: `make migrate-up && make seed && make seed` is clean; testcontainers integration test proves
symbol lookup, holiday detection (2027-01-26 false, 2027-01-27 true) and the IST date helper; `make test`
and `make lint` pass. Report the commands you ran.
```

## Notes / risks

- Hard-coded seeds will drift from the real Upstox instrument master; a later issue should replace them
  with a downloaded instrument dump. Keep repository signatures stable so that swap is invisible.
- `tick_size` matters for slippage (SS-016) — get real values, don't default everything to 0.05.
