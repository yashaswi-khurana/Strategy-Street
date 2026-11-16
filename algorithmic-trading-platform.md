# Algorithmic Backtesting & Deterministic Paper Trading Platform

## System Architecture & Product Specification

---

# 1\. Executive Summary

This document defines the complete functional and technical specification for a full-stack algorithmic trading, multi-timeframe market data warehousing, cross-stock historical backtesting, and deterministic live paper-trading platform.

Users can:

- Submit custom trading strategies in Python via a seamless UI.
- Evaluate strategies across the entire stock universe and sector classifications.
- Visualize historical and real-time market data alongside strategy signals.
- Run automated deterministic paper trading against live Upstox market feeds.

## Core Technology Stack

| Component | Technology | Responsibilities |
| --- | --- | --- |
| Backend & Stream Orchestrator | Go (Golang) | Upstox API integration, 2 RPS token-bucket rate limiting, WebSocket connection multiplexing, REST APIs, and deterministic paper-trading order routing. |
| Frontend Client | Angular | Reactive real-time charting, stock/timeframe selectors, Python code editor integration (Monaco Editor), sector heatmaps, and strategy leaderboards. |
| Data Manipulation & Strategy Engine | Python | Vectorized and event-driven backtests, technical indicator computation, and live strategy signal evaluation via a plugin-based strategy runner. |
| Database | PostgreSQL + TimescaleDB | Multi-resolution OHLCV time-series tables, stock/sector metadata, user strategies, backtest results, and paper-trading ledgers. |
| Market Data Provider | Upstox API | REST Historical Candle API (limited to 2 requests/second) and Market Data Feed V3 Protobuf WebSocket. |

---

# 2\. Core Features (From Initial Requirements)

## 2.1 Multi-Resolution Historical & Real-Time Time-Series Storage

### 13 Distinct Timeframes

Dedicated storage tables/views for the following timeframes across every tracked stock:

- 1s
- 1m
- 5m
- 10m
- 15m
- 30m
- 1h
- 3h
- 1d
- 15d
- 1M (1 month)
- 6M (6 months)
- 1Y (1 year)

### Side-by-Side Live Persistence

Incoming live ticks from the active stream simultaneously:

1. Update the real-time chart.
2. Aggregate into the 1s and higher-timeframe tables in PostgreSQL.

## 2.2 Rate-Limit-Aware Live Data Streaming (Upstox 2 RPS Constraint)

### Active-View Subscription Switching

Because Upstox restricts REST calls to 2 requests per second (2 RPS), the Go backend dynamically subscribes/streams only the instrument(s):

- Currently viewed by the user.
- Actively used by paper-trading deployments.

Idle symbols are unsubscribed to conserve quota.

### Token-Bucket Rate Limiter

All historical backfill and REST metadata calls pass through a centralized Go `rate.Limiter` configured strictly at 2 requests/second.

Priority queue:

1. Live UI chart backfill.
2. Paper Trading warmup.
3. Background historical sync.

## 2.3 One-Click Python Strategy Onboarding ("Add Strategy")

### In-Browser Strategy Submission

Users click **Add Strategy**, write or paste a Python script implementing a standardized Strategy interface, and save it directly to the platform.

### Dynamic Strategy Loader

The Python execution worker dynamically loads user scripts as plugins without restarting the backend service.

## 2.4 Full-Universe & Sector-Wide Backtesting

### All-Stock Evaluation

Every submitted Python strategy is automatically backtested across the historical dataset of every stock in the database across selected timeframes.

### Sector & Stock-Strategy Matrix

The platform identifies:

- Which sector (e.g., IT, Banking, Pharma, Auto, FMCG) a given strategy performs best on.
- Which strategy is the top performer for any individual stock.

## 2.5 Automated Deterministic Paper Trading

### Live Strategy Execution

Strategies toggled for paper trading receive live bar/tick updates from the Go stream hub and deterministically generate:

- BUY
- SELL
- HOLD

### Win Rate & Ledger Tracking

Tracks:

- Simulated fills.
- Open positions.
- Realized/unrealized P&L.
- Real-time Win Rate (%) per strategy and per stock.

## 2.6 Interactive Stock & Strategy Charting

### Unified Historical + Real-Time Canvas

Plots historical candles seamlessly stitched with live incoming ticks for any selected stock and timeframe (1s to 1Y).

### Strategy Visual Overlays

Displays:

- Custom indicators (moving averages, bands, oscillators).
- Buy/Sell entry and exit markers.
- Equity/drawdown sub-chart.

---

# 3\. Missing Critical Features (Added to Complete the Platform)

To make this platform production-ready, accurate, and safe to run, the following 10 missing architectural and domain features have been added to the specification.

## 3.1 Python Code Sandboxing & Security Isolation

**Problem:**

Allowing users to paste arbitrary Python scripts via an "Add Strategy" button introduces severe remote code execution (RCE) risks.

Examples:

- `import os`
- `os.system("rm -rf /")`
- Infinite `while True` loops.
- Unauthorized database/network access.

**Added Feature:**

### AST Static Analysis

Parse the Python Abstract Syntax Tree (`ast` module) upon submission to:

- Block forbidden imports (`os`, `sys`, `subprocess`, `socket`, `urllib`, `requests`, `shutil`, `open`).
- Allow only whitelisted libraries (`numpy`, `pandas`, `ta`, `math`, `scipy`).

### Ephemeral Container / Worker Sandboxing

Execute user strategies inside resource-constrained workers:

- Docker/gVisor or Linux nsjail with seccomp filters.
- Strict CPU time limits (e.g., 5 seconds per stock backtest).
- Memory caps (512 MB).

## 3.2 Realistic Market Friction, Slippage & Indian Tax/Brokerage Engine

**Problem:**

Backtesting and paper trading on raw 1s or 1m prices without transaction costs makes losing high-frequency strategies appear wildly profitable.

**Added Feature:**

### Indian Statutory Cost Calculator

Automatically deduct:

- Brokerage (e.g., Rs. 20 or 0.03% cap).
- Securities Transaction Tax (STT).
- Exchange Transaction Charges (NSE/BSE).
- SEBI Turnover Fees.
- Stamp Duty.
- 18% GST on every simulated trade.

### Slippage & Spread Model

- Configurable tick slippage (e.g., 0.05% or 1–2 ticks).
- Volume participation caps (cannot fill 10,000 shares on a 1-second candle that only had 200 volume).

## 3.3 Comprehensive Quantitative Metrics (Beyond Win Rate)

**Problem:**

A strategy can have a 90% win rate (winning Rs. 10 nine times) and still lose money if the 10% loss is Rs. 500. Win rate alone is misleading.

**Added Feature:**

Compute and display:

- Net Profit/Loss.
- Profit Factor (Gross Profit / Gross Loss).
- Expectancy per Trade.
- Maximum Drawdown (%) & Duration.
- Sharpe Ratio.
- Sortino Ratio.
- Calmar Ratio.
- CAGR.
- Average Holding Period.

## 3.4 Corporate Actions & Market Calendar Hygiene

**Problem:**

Unadjusted stock splits (e.g., 1:5 split), bonus issues, or dividends look like massive 80% overnight crashes in historical tables, corrupting backtest signals.

**Added Feature:**

### Adjustment Factor Pipeline

Store:

- Raw OHLCV prices.
- Split/dividend-adjusted OHLCV prices.

### NSE/BSE Trading Session Filter

- Enforce market hours (09:15:00 to 15:30:00 IST).
- Ignore pre-market/post-market stray ticks unless configured.
- Maintain an exchange holiday calendar table.

### Missing Bar Policy

Forward-fill (`ffill`) zero-volume seconds/minutes without fabricating phantom volume.

## 3.5 Database Write-Amplification Mitigation (Hypertables & Continuous Aggregates)

**Problem:**

Maintaining 13 separate physical tables (`candles_1s` through `candles_1y`) and running 13 INSERT/UPDATE queries on every single live tick will bottleneck PostgreSQL I/O.

**Added Feature:**

- Write live data only to `candles_1s` (and `candles_1m`), partitioned by time and stock\_id using TimescaleDB Hypertables.
- Use Continuous Aggregates / Materialized Views to automatically and incrementally roll up 1m data into:
  - 5m
  - 10m
  - 15m
  - 30m
  - 1h
  - 3h
  - 1d
  - 15d
  - 1M
  - 6M
  - 1Y

This eliminates application-side write overhead for higher-timeframe aggregation.

## 3.6 Look-Ahead Bias Prevention & Walk-Forward Validation

**Problem:**

Python scripts given the full DataFrame at once can accidentally peek at future rows (`df['close'].shift(-1)`), producing fake 100% win rates in backtests that fail in live paper trading.

**Added Feature:**

### Point-in-Time Execution Mode

Provide an event-driven execution mode:

python

```python
on_bar(current_bar, history_window)
```

In addition to vectorized mode.

Also implement a static linter that flags negative shifts (`.shift(-n)`).

### In-Sample / Out-of-Sample Split

Allow users to split historical data:

- 70% training/optimization.
- 30% out-of-sample testing.

This helps detect overfitting before deploying to paper trading.

## 3.7 Portfolio, Capital & Risk Management Layer

**Problem:**

Deterministic paper trading needs rules for how much quantity to buy and when to halt a runaway strategy.

**Added Feature:**

### Virtual Wallet & Position Sizing

Each paper-trading deployment receives:

- Configurable virtual capital (e.g., Rs. 1,00,000).
- Sizing rules:
  - Fixed Capital.
  - % of Equity.
  - Fixed Lot.

### Order Types & Risk Guards

Support:

- Market.
- Limit.
- Stop-Loss (SL).
- Trailing Stop-Loss.

Also implement an automated **Daily Max Loss Kill-Switch** that pauses a strategy if daily drawdown exceeds a configured threshold.

## 3.8 Asynchronous Task Queue & Go-Python IPC Bridge

**Problem:**

Running a Python strategy across 500+ stocks over 5 years of 1-minute data takes several seconds to minutes; running it synchronously in an HTTP request will time out.

**Added Feature:**

### gRPC + Worker Queue

Go dispatches backtest jobs to a Python worker pool via:

- Redis Streams.
- gRPC.

### Real-Time Progress Streaming

Go pushes job progress to the Angular frontend via WebSocket.

Example:

text

```text
142 / 500 stocks completed
```

## 3.9 Upstox OAuth2 Daily Token Lifecycle & Protobuf WebSocket Multiplexer

**Problem:**

Upstox access tokens expire daily at 3:30 AM IST, and Upstox Market Data Feed V3 sends binary Protobuf packets over WebSocket rather than REST polling.

**Added Feature:**

### Automated Token Handshake Service

- Scheduled morning auth flow.
- Token health monitor in Go.

### Single WebSocket Multiplexer + 2 RPS Historical Backfiller

Use:

- Upstox's binary Protobuf WebSocket for real-time tick streaming (which does not consume the 2 RPS REST quota).
- The 2 RPS REST budget strictly for historical candle backfilling when a user switches stocks/timeframes.

## 3.10 Strategy Versioning & Parameter Optimization Grid

**Problem:**

When users tweak a Python script (e.g., changing RSI period from 14 to 21), overwriting the script destroys comparability with past backtests and active paper trades.

**Added Feature:**

### Immutable Strategy Versions (v1, v2, v3)

Every edit creates a version snapshot tied to:

- Its backtest run.
- Its paper-trading ledger.

### Hyperparameter Dict Support

Users can define tunable parameters (e.g., `fast_ma = 9`, `slow_ma = 21`) to compare parameter sets across sectors.

---

# 4\. System Architecture & Data Flow

## 4.1 High-Level Component Architecture

### Angular Frontend (SPA)

**Charting Module:**

- TradingView lightweight-charts or ECharts canvas rendering.
- Historical + live candles.
- Volume histograms.
- Indicator overlays.
- Buy/sell markers.

**Strategy Studio:**

- Embedded Monaco Editor.
- Python syntax highlighting.
- Autocomplete for the Strategy SDK.
- Launched via the "Add Strategy" button.

**Analytics & Leaderboard Dashboard:**

- Sector performance heatmaps.
- Stock-vs-strategy matrix tables.
- Live paper-trading blotter.

### Go Backend Gateway & Engine

**Upstox Connector:**

- Manages OAuth2 tokens.
- Decodes V3 Protobuf WebSocket ticks.
- Enforces the 2 RPS Token-Bucket Rate Limiter (`golang.org/x/time/rate`) for REST historical candle fetches.

**Subscription Manager:**

- Tracks active Angular client rooms.
- When a user opens RELIANCE on 5m, Go subscribes the live feed for RELIANCE.
- Streams ticks to the Angular client over an internal WebSocket.
- Writes ticks to PostgreSQL.

**Deterministic Paper-Trading Engine:**

- Listens to completed candle events from the internal event bus.
- Invokes the Python strategy evaluator.
- Processes simulated fills through the Slippage/Tax engine.
- Updates the paper portfolio.

### Python Data & Strategy Engine

**Sandbox Runner:**

- Validates user AST.
- Loads historical OHLCV slices from PostgreSQL (via ConnectorX / asyncpg for zero-copy speed).
- Executes `Strategy.generate_signals(df)`.
- Writes vector metrics back to PostgreSQL.

### PostgreSQL + TimescaleDB Database

Stores:

- Metadata.
- 13 timeframe candle tables/views.
- Strategy versions.
- Backtest run summaries.
- Trade logs.
- Live paper positions.

---

# 5\. Database Schema Design (PostgreSQL)

## 5.1 Core Metadata & Strategy Tables

### stocks

| Column | Description |
| --- | --- |
| id | Primary Key |
| instrument\_key | Unique (e.g., `NSE_EQ\|INE002A01018`) |
| symbol | Stock symbol (e.g., RELIANCE) |
| company\_name | Company name |
| sector\_id | Foreign Key to sectors |
| lot\_size | Lot size |
| tick\_size | Minimum price movement |
| is\_active | Active status |

### sectors

| Column | Description |
| --- | --- |
| id | Primary Key |
| name | Sector name (e.g., Banking, IT, Pharma, Auto, Energy) |
| benchmark\_index | Associated benchmark index |

### strategies

| Column | Description |
| --- | --- |
| id | Primary Key |
| user\_id | Strategy owner |
| name | Strategy name |
| description | Strategy description |
| current\_version | Current version |
| created\_at | Creation timestamp |

### strategy\_versions

| Column | Description |
| --- | --- |
| id | Primary Key |
| strategy\_id | Foreign Key to strategies |
| version\_number | Version number |
| python\_code | Python source code (TEXT) |
| parameters\_json | Strategy parameters (JSONB) |
| ast\_hash | AST hash |
| created\_at | Creation timestamp |

## 5.2 Multi-Timeframe OHLCV Storage (13 Timeframes)

To satisfy the requirement for separate timeframe access across all 13 intervals (1s, 1m, 5m, 10m, 15m, 30m, 1h, 3h, 1d, 15d, 1M, 6M, 1Y), each table shares a uniform schema optimized with composite primary keys (`stock_id`, `bucket_time DESC`).

### Timeframe Storage Configuration

| Table / Hypertable Name | Interval | Ingestion Method | Retention / Compression |
| --- | --- | --- | --- |
| `candles_1s` | 1 Second | Direct stream aggregation from live Upstox ticks | Compressed after 7 days |
| `candles_1m` | 1 Minute | Rollup from 1s + Upstox 1m REST backfill | Compressed after 30 days |
| `candles_5m` | 5 Minutes | Continuous Aggregate / Trigger from 1m | Compressed after 90 days |
| `candles_10m` | 10 Minutes | Continuous Aggregate / Trigger from 1m | Compressed after 90 days |
| `candles_15m` | 15 Minutes | Continuous Aggregate / Trigger from 1m | Compressed after 90 days |
| `candles_30m` | 30 Minutes | Continuous Aggregate / Trigger from 1m + REST backfill | Compressed after 180 days |
| `candles_1h` | 1 Hour | Continuous Aggregate / Trigger from 30m | Standard indexed |
| `candles_3h` | 3 Hours | Continuous Aggregate / Trigger from 1h | Standard indexed |
| `candles_1d` | 1 Day | Rollup from intraday + Upstox Daily REST backfill | Permanent uncompressed |
| `candles_15d` | 15 Days | Rollup from 1d | Permanent uncompressed |
| `candles_1mo` | 1 Month | Rollup from 1d | Permanent uncompressed |
| `candles_6mo` | 6 Months | Rollup from 1mo | Permanent uncompressed |
| `candles_1y` | 1 Year | Rollup from 1mo | Permanent uncompressed |

### Common Candle Table Schema

Every `candles_<tf>` table contains:

| Column | Data Type | Description |
| --- | --- | --- |
| stock\_id | INT | Foreign Key → `stocks.id` |
| bucket\_time | TIMESTAMPTZ | NOT NULL |
| open | NUMERIC(12, 4) | Opening price |
| high | NUMERIC(12, 4) | Highest price |
| low | NUMERIC(12, 4) | Lowest price |
| close | NUMERIC(12, 4) | Closing price |
| volume | BIGINT | Traded volume |
| oi | BIGINT | Open interest (default 0) |
| is\_adjusted | BOOLEAN | Adjustment status (default TRUE) |

## 5.3 Backtesting & Paper Trading Tables

### backtest\_runs

| Column | Description |
| --- | --- |
| id | Primary Key |
| strategy\_version\_id | Foreign Key to strategy\_versions |
| timeframe | Backtest timeframe |
| start\_date | Historical start date |
| end\_date | Historical end date |
| status | QUEUED, RUNNING, COMPLETED, FAILED |
| created\_at | Creation timestamp |

### backtest\_stock\_metrics

| Column | Description |
| --- | --- |
| id | Primary Key |
| backtest\_run\_id | Foreign Key to backtest\_runs |
| stock\_id | Foreign Key to stocks |
| sector\_id | Foreign Key to sectors |
| total\_trades | Total executed trades |
| win\_rate\_pct | Win rate percentage |
| net\_pnl | Net profit/loss |
| return\_pct | Return percentage |
| profit\_factor | Profit factor |
| sharpe\_ratio | Sharpe ratio |
| max\_drawdown\_pct | Maximum drawdown |
| expectancy | Trade expectancy |

### backtest\_sector\_summary

| Column | Description |
| --- | --- |
| id | Primary Key |
| backtest\_run\_id | Foreign Key to backtest\_runs |
| sector\_id | Foreign Key to sectors |
| avg\_win\_rate\_pct | Average win rate |
| avg\_return\_pct | Average return |
| median\_sharpe | Median Sharpe ratio |
| best\_stock\_id | Foreign Key to stocks |
| composite\_score | Sector composite score |

### paper\_deployments

| Column | Description |
| --- | --- |
| id | Primary Key |
| strategy\_version\_id | Foreign Key to strategy\_versions |
| stock\_id | Foreign Key to stocks |
| timeframe | Trading timeframe |
| initial\_capital | Initial virtual capital |
| current\_equity | Current equity |
| status | ACTIVE, PAUSED, KILLED |
| win\_rate\_pct | Win rate |
| total\_trades | Total trades |

### paper\_trades

| Column | Description |
| --- | --- |
| id | Primary Key |
| deployment\_id | Foreign Key to paper\_deployments |
| side | BUY/SELL |
| entry\_time | Entry timestamp |
| entry\_price | Entry price |
| exit\_time | Exit timestamp |
| exit\_price | Exit price |
| qty | Trade quantity |
| gross\_pnl | Gross profit/loss |
| charges\_and\_taxes | Charges and taxes |
| net\_pnl | Net profit/loss |
| is\_win | BOOLEAN |

---

# 6\. Standardized Python Strategy SDK Contract

When a user clicks "Add Strategy" in the Angular UI, the editor pre-populates a clean Python template so users only need to write their indicator logic and entry/exit rules.

## Default Python Strategy Template

python

```python
from sdk.base_strategy import BaseStrategy
import pandas as pd
import pandas_ta as ta


class UserStrategy(BaseStrategy):

    # Optional tunable parameters
    params = {
        "ema_fast": 9,
        "ema_slow": 21,
        "rsi_period": 14
    }

    def generate_signals(self, df: pd.DataFrame) -> pd.DataFrame:
        """
        Input df columns:
        ['bucket_time', 'open', 'high', 'low', 'close', 'volume']

        Must return df with:
          - 'signal': 1 (BUY / LONG), -1 (SELL / EXIT / SHORT), 0 (HOLD)
          - Optional indicator columns to plot on the Angular chart
        """

        df["ema_fast"] = ta.ema(
            df["close"],
            length=self.params["ema_fast"]
        )

        df["ema_slow"] = ta.ema(
            df["close"],
            length=self.params["ema_slow"]
        )

        df["rsi"] = ta.rsi(
            df["close"],
            length=self.params["rsi_period"]
        )

        df["signal"] = 0

        buy_cond = (
            (df["ema_fast"] > df["ema_slow"]) &
            (df["ema_fast"].shift(1) <= df["ema_slow"].shift(1)) &
            (df["rsi"] > 50)
        )

        sell_cond = (
            (df["ema_fast"] < df["ema_slow"]) &
            (df["ema_fast"].shift(1) >= df["ema_slow"].shift(1))
        )

        df.loc[buy_cond, "signal"] = 1
        df.loc[sell_cond, "signal"] = -1

        # Specify which columns should be overlaid on the stock chart
        self.plot_overlays = ["ema_fast", "ema_slow"]
        self.plot_subcharts = ["rsi"]

        return df
```

## Deterministic Paper-Trading Execution Guarantee

To guarantee that live paper trading is 100% deterministic:

1. Strategy evaluation triggers strictly on bar close (e.g., at 09:16:00.000 when the 09:15 1-minute candle closes) or deterministic tick sequence numbers.
2. Given the exact same sequence of historical/live bars in PostgreSQL, re-running the paper trader produces the exact same:
   - Entry timestamps.
   - Quantities.
   - Slippage deductions.
   - Win rates.

---

# 7\. Upstox 2 RPS Rate-Limit & Live Stream Management

## Dynamic Context Tracking

When the user selects:

text

```text
Stock = TCS
Timeframe = 5m
```

In Angular, it emits a WebSocket event:

json

```json
{
  "event": "SUBSCRIBE_VIEW",
  "stock": "TCS",
  "tf": "5m"
}
```

Go then unsubscribes any previously viewed stock that is not currently used by an active paper-trading deployment.

## Smart Gap Backfill (2 RPS Queue)

1. Go checks the latest `bucket_time` for TCS in `candles_5m`.
2. If a gap exists between the last stored timestamp and `now()`, Go enqueues a backfill task into the 2 RPS Token-Bucket Limiter.

go

```go
rate.NewLimiter(rate.Limit(2), 1)
```

3. Once fetched, Go bulk-upserts the candles into PostgreSQL:

sql

```sql
ON CONFLICT (stock_id, bucket_time)
DO UPDATE
```

4. Go streams the dataset to Angular.

## Side-by-Side Live DB Ingestion

Live ticks from Upstox:

- Update an in-memory ring buffer in Go for sub-millisecond chart delivery to Angular.
- A background Go flush routine batch-writes completed 1s bars to PostgreSQL every second using COPY / batch upsert.

---

# 8\. Analytics: Sector Scoring & Stock-Strategy Matrix

## 8.1 Sector Composite Score Formula

To rank which sector a strategy excels in (rather than relying on a single outlier stock), the platform computes a Sector Composite Score for each `(strategy, sector, timeframe)` tuple.

### Formula

text

```text
Sector Score =
    0.35 * Normalized(Avg Win Rate)
  + 0.35 * Normalized(Median Sharpe Ratio)
  + 0.30 * Normalized(Profit Factor)
  - Penalty(Max Drawdown)
```

Displayed in Angular as a Sector Leaderboard.

Example:

text

```text
Strategy: EMA Crossover v2

Highest Performing Sector:
NIFTY IT
Score: 84.2
Win Rate: 68.4%

Lowest Performing Sector:
FMCG
Score: 41.0
```

## 8.2 Best Strategy Per Stock Lookup

Querying `backtest_stock_metrics` indexed by:

text

```text
(stock_id, timeframe, composite_score DESC)
```

Immediately surfaces:

- The highest-ranking strategy for any selected stock.
- Its live paper-trading Win Rate.

This information is displayed in the stock chart view.

---

# 9\. REST & WebSocket API Specification (Go Backend)

| Method / Protocol | Endpoint / Channel | Description |
| --- | --- | --- |
| POST | `/api/v1/strategies` | Validate AST, save Python strategy via "Add Strategy", and trigger initial backtest job. |
| POST | `/api/v1/backtests/run` | Queue an all-stock backtest across specified timeframes (1s to 1Y). |
| GET | `/api/v1/candles/:symbol` | Fetch historical candles (`?tf=5m&from=...&to=...`); triggers 2 RPS backfill if missing. |
| GET | `/api/v1/analytics/sectors/:strategy_id` | Return sector-by-sector win rate, Sharpe, PnL, and composite rankings. |
| GET | `/api/v1/analytics/stocks/:symbol/best-strategies` | Return top strategies ranked for a specific stock and timeframe. |
| POST | `/api/v1/paper-trading/deploy` | Start deterministic live paper trading for a `(strategy_version_id, stock_id, tf)`. |
| GET | `/api/v1/paper-trading/summary` | Return active paper trades, live win rate, realized/unrealized PnL, and trade log. |
| WS | `/ws/market-stream` | Multiplexed WebSocket: receives `{ stock, tf, strategy_id }` subscription and pushes live candles + strategy indicator overlays + paper trade fills. |

---

# 10\. Implementation Roadmap

## Phase 1 — Data Layer & Upstox 2 RPS Streamer

**Technology:** Go + PostgreSQL

Tasks:

- Set up PostgreSQL hypertables/continuous aggregates for the 13 timeframes.
- Build the Go 2 RPS token-bucket rate limiter.
- Implement active-view Upstox streaming.
- Implement database persistence.

## Phase 2 — Python Sandbox & All-Stock Backtester

Tasks:

- Build the AST validator.
- Implement the BaseStrategy SDK.
- Build the Indian brokerage/slippage calculator.
- Implement the parallelized all-stock backtest worker pool.

## Phase 3 — Deterministic Paper Trading Engine

Tasks:

- Connect live bar-close events in Go to the Python signal evaluator.
- Implement the virtual order book.
- Track live Win Rates.

## Phase 4 — Angular Frontend & Analytics Dashboards

Tasks:

- Build the unified historical + real-time chart view.
- Implement the "Add Strategy" Monaco modal.
- Build the Sector Leaderboard.
- Build the Stock-Strategy Matrix.

---

# End of Specification
