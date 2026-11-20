# ADR 0004: Bucket Origins, Continuous Aggregate Refresh Policies & Retention Lifecycle

## Status
Accepted

## Context
Strategy Street stores and serves market data across 13 distinct timeframes:
- Raw Hypertables: `candles_1s`, `candles_1m`
- Intraday Continuous Aggregates (from `candles_1m`): `candles_5m`, `candles_10m`, `candles_15m`, `candles_30m`, `candles_1h`, `candles_3h`
- Session-aligned Daily Continuous Aggregate (from `candles_1m`): `candles_1d`
- Macro Calendar Continuous Aggregates (from `candles_1d`): `candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y`

### The Problem: UTC vs. Wall-Clock Alignment
In PostgreSQL and TimescaleDB, the default origin for `time_bucket(INTERVAL, TIMESTAMPTZ)` is the UTC Unix epoch (`1970-01-01 00:00:00Z`). 
Because Indian Standard Time (IST) is UTC+05:30 with no Daylight Saving Time, relying on UTC origins produces severe misalignments for National Stock Exchange (NSE) trading hours (09:15 to 15:30 IST):
1. **1-Hour Buckets**: With UTC or 00:00 IST origin, hourly buckets fall on `09:00:00`, `10:00:00`, etc. The first bar of the day (09:15 to 10:00) contains only 45 minutes of trading activity, distorting volume and indicator profiles.
2. **10-Minute & 30-Minute Buckets**: 09:15 is not a multiple of 10 or 30 minutes from midnight (555 minutes from midnight = 55.5 x 10m, 18.5 x 30m). A 00:00 origin generates a 09:10-09:20 bar with only 5 minutes of data.
3. **Daily Buckets**: A midnight UTC or midnight IST origin chops intraday sessions or fails to cleanly align with the 09:15 market opening.
4. **Hierarchical Continuous Aggregates**: TimescaleDB 2.15 requires that continuous aggregates built over other continuous aggregates (e.g. `candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y` derived from `candles_1d`) share the exact same `origin` timestamp.

## Decision

### 1. Unified Bucket Origin Constant
All continuous aggregates across the entire platform use the unified bucket origin:
```sql
TIMESTAMPTZ '2015-01-01 09:15:00+05:30'  -- Equivalent to '2015-01-01 03:45:00Z'
```
This origin satisfies four simultaneous criteria:
1. **Wall-clock 09:15 IST anchor**: Hourly bars land on `09:15`, `10:15`, `11:15`, `12:15`, `13:15`, `14:15`, `15:15` IST.
2. **Session-enclosed Daily bars**: Intraday trading hours (09:15 to 15:30) fall completely within the `[09:15:00, next_day 09:15:00)` interval. Every daily bar's volume is guaranteed to equal the sum of its exactly 375 one-minute bars with zero overnight contamination.
3. **Integer Multiples**: The interval between `1970-01-01 09:15:00+05:30` and `2015-01-01 09:15:00+05:30` is exactly 16,436 days. Because $16436 \equiv 0 \pmod{1}$, $16436 \times 24 \equiv 0 \pmod{1}$, and $16436 \times 24 \times 60 \equiv 0 \pmod{5, 10, 15, 30}$, this anchor is mathematically identical to `1970-01-01 09:15:00+05:30` for all intraday and daily buckets while also providing an explicit, clean modern anchor for 15-day buckets.
4. **Hierarchical Cagg Compatibility**: `candles_1d` and all macro rollups (`candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y`) share this identical origin, permitting TimescaleDB hierarchical continuous aggregates without origin mismatch errors.

### 2. Timeframe-to-Relation Resolution in Go
The Go candle repository resolves any `Timeframe` enum directly to its database relation name via `Timeframe.TableName()` and `Timeframe.RelationName()`:
- `1s` $\rightarrow$ `candles_1s`
- `1m` $\rightarrow$ `candles_1m`
- `5m` $\rightarrow$ `candles_5m`
- `10m` $\rightarrow$ `candles_10m`
- `15m` $\rightarrow$ `candles_15m`
- `30m` $\rightarrow$ `candles_30m`
- `1h` $\rightarrow$ `candles_1h`
- `3h` $\rightarrow$ `candles_3h`
- `1d` $\rightarrow$ `candles_1d`
- `15d` $\rightarrow$ `candles_15d`
- `1mo` $\rightarrow$ `candles_1mo`
- `6mo` $\rightarrow$ `candles_6mo`
- `1y` $\rightarrow$ `candles_1y`

`IsWritable()` strictly forbids application writes to continuous aggregates (`5m` through `1y`). The database engine autonomously maintains higher timeframes via refresh policies.

### 3. Continuous Aggregate Refresh Windows
All refresh policies are declared in `db/migrations/0004_policies.up.sql`:

| View | Source | Bucket Interval | Schedule Interval | `start_offset` | `end_offset` | Refresh Window Span |
|---|---|---|---|---|---|---|
| `candles_5m` | `candles_1m` | 5 min | 1 min | 2 hours | 1 min | 24 buckets |
| `candles_10m` | `candles_1m` | 10 min | 2 min | 4 hours | 1 min | 24 buckets |
| `candles_15m` | `candles_1m` | 2 min | 6 hours | 1 min | 24 buckets |
| `candles_30m` | `candles_1m` | 30 min | 5 min | 12 hours | 1 min | 24 buckets |
| `candles_1h` | `candles_1m` | 1 hour | 10 min | 24 hours | 1 min | 24 buckets |
| `candles_3h` | `candles_1m` | 3 hours | 15 min | 3 days | 1 min | 24 buckets |
| `candles_1d` | `candles_1m` | 1 day | 1 hour | 7 days | 1 hour | 7 buckets |
| `candles_15d` | `candles_1d` | 15 days | 1 day | 60 days | 1 day | 4 buckets |
| `candles_1mo` | `candles_1d` | 1 month | 1 day | 90 days | 1 day | ~3 buckets |
| `candles_6mo` | `candles_1d` | 6 months | 1 day | 500 days | 1 day | > 2 buckets |
| `candles_1y` | `candles_1d` | 1 year | 1 day | 1000 days | 1 day | > 2 buckets |

`end_offset => INTERVAL '1 minute'` ensures the currently open unclosed minute bar is not locked or partially baked into higher aggregates, while keeping refresh latency under 60 seconds.

### 4. Compression & Retention Policies
Aligned with architectural specification §5.2:

| Relation | Type | Compression Policy | Retention Policy | Notes |
|---|---|---|---|---|
| `candles_1s` | Hypertable | 7 days | 7 days | High-velocity raw tick aggregate |
| `candles_1m` | Hypertable | 30 days | 30 days | High-velocity raw minute bars |
| `candles_5m` | Cont. Aggregate | 90 days | None (Permanent) | Segmentby `stock_id`, Orderby `bucket_time DESC` |
| `candles_10m` | Cont. Aggregate | 90 days | None (Permanent) | Segmentby `stock_id`, Orderby `bucket_time DESC` |
| `candles_15m` | Cont. Aggregate | 90 days | None (Permanent) | Segmentby `stock_id`, Orderby `bucket_time DESC` |
| `candles_30m` | Cont. Aggregate | 180 days | None (Permanent) | Segmentby `stock_id`, Orderby `bucket_time DESC` |
| `candles_1h` | Cont. Aggregate | None | None (Permanent) | Standard indexed |
| `candles_3h` | Cont. Aggregate | None | None (Permanent) | Standard indexed |
| `candles_1d`..`1y` | Cont. Aggregate | None | None (Permanent) | Permanent uncompressed daily and macro series |

## Consequences
- **Positive**:
  - Live ingestion writes once to `candles_1m` (and `candles_1s`); all 11 higher timeframes update automatically with zero application write amplification.
  - Backtest queries reading `candles_1h` or `candles_1d` execute orders of magnitude faster without ad-hoc group-by calculations.
  - 1h bars land cleanly at 09:15, 10:15, etc., matching trader expectations and charting libraries.
  - Memory consumption is tightly bounded by automated chunk compression and raw tick retention policies.
- **Negative / Operational Considerations**:
  - Continuous aggregate definitions must be dropped and recreated if aggregation SQL logic changes.
  - Test suite must use `CALL refresh_continuous_aggregate(...)` to force immediate synchronization during deterministic integration tests.

