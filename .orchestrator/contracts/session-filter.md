# CONTRACT: session-filter v1   status: FROZEN
- producer: backend-engineer   consumers: live-streamer, paper-engine, backtest-engine, chart-service
- shared paths: api/internal/marketcal/**, engine/engine/marketcal.py, engine/marketcal.py, engine/testdata/sessions.json

## Interface & Specification

### 1. Market Session Boundaries
- Timezone: `Asia/Kolkata` (`UTC+05:30`) fixed offset.
- Half-open interval: `[open, close)`.
- Normal session: `09:15:00` to `15:30:00` IST.
- A tick at `15:30:00` IST belongs to NO bar (`IsOpen(t) == false`).
- Seeded holidays and weekends return `Session(day) -> ok = false` unless overridden by `market_sessions`.

### 2. Bar Timing Invariants
- `BarsPerSession`:
  - `1s`: 22,500 bars
  - `1m`: 375 bars
  - `5m`: 75 bars
  - `10m`: 38 bars
  - `15m`: 25 bars
  - `30m`: 13 bars
  - `1h`: 7 bars
  - `3h`: 3 bars
  - `1d`: 1 bar
- `NextBarClose(now, tf)`:
  - Deterministic and pure function of `now` and `tf`.
  - Zero dependence on wall-clock seconds.
  - Returns `ok = false` outside active session hours.

### 3. Missing-Bar Policy & Decorator
- `FillPolicy`:
  - `CarryForward`: repeats previous close for open/high/low/close with volume = 0, oi = previous.oi, and `is_filled = true`.
  - `Skip`: returns series without additions.
- Read-time only: filled bars must NEVER be written to the database.
- Schema Honest: `is_filled BOOLEAN NOT NULL DEFAULT FALSE` in candle tables; continuous aggregates aggregate via `bool_and(is_filled)`.

### 4. Cross-Language Guarantees
- Go (`api/internal/marketcal`) and Python (`engine/marketcal.py`) implementations must match byte-for-byte across shared fixtures in `engine/testdata/sessions.json`.

