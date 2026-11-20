# CONTRACT: candle-store v1   status: FROZEN
- producer: backend-engineer   consumers: live-streamer, paper-engine, backtest-engine
- shared paths: api/internal/store/candles/**

## Interface & Schema Specification

### 1. Hypertables
- `candles_1s` and `candles_1m`
- Chunk interval: 1 day (`INTERVAL '1 day'`)
- Primary key: `(stock_id, bucket_time)`
- Index: `(stock_id, bucket_time DESC)`

### 2. Timeframe Enum
- Supported: `1s`, `1m`, `5m`, `10m`, `15m`, `30m`, `1h`, `3h`, `1d`, `15d`, `1mo`, `6mo`, `1y`
- Raw writable by application: `1s`, `1m` (higher intervals strictly rejected on write)
- Reference epoch: `1970-01-01T00:00:00+05:30` (IST)

### 3. Go CandleStore Interface
```go
type CandleStore interface {
    UpsertBatch(ctx context.Context, tf Timeframe, bars []Candle) error
    Range(ctx context.Context, stockID int, tf Timeframe, from, to time.Time) ([]Candle, error)
    Latest(ctx context.Context, stockID int, tf Timeframe) (Candle, error)
    LatestTime(ctx context.Context, stockID int, tf Timeframe) (time.Time, error)
    CopyBatch(ctx context.Context, tf Timeframe, bars []Candle) error
}
```

### 4. Constraints & Guarantees
- `bucket_time` must be in UTC and exactly aligned with `Timeframe.Align(t)`.
- Price fields (`open`, `high`, `low`, `close`) use `pgtype.Numeric` internally, formatted as fixed-point base-10 decimals.
- `high >= low`, `high >= open`, `high >= close`, `low <= open`, `low <= close`.
- `volume >= 0`, `oi >= 0`.
- In SS-003, `is_adjusted = FALSE` is strictly enforced.

