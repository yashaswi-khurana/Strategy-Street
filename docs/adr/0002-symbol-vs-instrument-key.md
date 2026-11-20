# ADR 0002: Symbol vs. Instrument Key Identification

## Status

Accepted

## Context

In Indian equities trading (NSE / BSE), every tradable asset has multiple identifiers across different contexts:
1. **Ticker Symbol** (`symbol` e.g., `RELIANCE`, `TCS`, `INFY`): The standard market ticker symbol familiar to traders, users, and analysts. Used in UI search, URL routes, strategy parameters, and charting headers.
2. **Broker Instrument Key** (`instrument_key` e.g., `NSE_EQ|INE002A01018`): The Upstox protocol key composed of exchange segment (`NSE_EQ`) and ISIN (`INE...`) or exchange token. This exact key is mandatory for subscribing to Upstox binary WebSocket feeds, querying historical market candle archives, and dispatching execution orders.
3. **Internal Relational Identifier** (`id SERIAL PRIMARY KEY`): A compact 32-bit/64-bit integer identifier optimized for TimescaleDB hypertable chunking (e.g., `candles_1m(stock_id, bucket_time)`), foreign key indexing, and join performance across billions of OHLCV rows.
4. **Company Name** (`company_name` e.g., `Reliance Industries Limited`): Verbose descriptive text that varies across brokers, filing sources, and registrars.

Without a formal architectural decision, systems frequently conflate these keys or attempt lookups by company name, creating fragile string matching, broker lock-in, or slow index scans across massive market candle tables.

## Decision

We establish a strict dual-key identification strategy with synthetic relational identity:

1. **`stocks.symbol` (TEXT NOT NULL UNIQUE)**:
   - Sole public/human-facing identifier.
   - Used in REST/gRPC endpoints, front-end Angular components, CLI commands, and strategy definition files.
   - Case-sensitive uppercase convention.

2. **`stocks.instrument_key` (TEXT NOT NULL UNIQUE)**:
   - Sole broker/data-provider identifier.
   - Follows Upstox specification format `[EXCHANGE]_[SEGMENT]|[ISIN_OR_TOKEN]`.
   - Used by the live data ingestion engine, Upstox rate limiter (2 RPS), WebSocket subscriber, and broker order routing adapters.

3. **`stocks.id` (SERIAL PRIMARY KEY)**:
   - Sole relational foreign key target for time-series candles, strategy backtests, and paper trading position tables.
   - Never exposed as an external API contract; keeps hypertable indexes lean and partition-friendly.

4. **Lookup Rules**:
   - Lookups by company name are strictly prohibited for system logic.
   - The Go repository layer (`internal/store/metadata`) provides typed lookups: `GetStockBySymbol` for API/strategy handlers, and `GetStockByInstrumentKey` for feed ingestion handlers.

5. **Broker Master Independence**:
   - Initial seed data provides verified deterministic mappings for core NSE assets.
   - The repository interface (`MetadataStore`) isolates the rest of the application so that automated Upstox instrument master sync (SS-005) can update or refresh records without breaking repository consumers.

## Consequences

### Positive
- **Clear Domain Boundaries**: UI and strategy logic remain independent of Upstox-specific key formats.
- **Query Performance**: Time-series hypertable rows foreign-key to a compact integer `stock_id`, keeping candle indices compact in memory.
- **Idempotency & Data Integrity**: Unique constraints on both `symbol` and `instrument_key` prevent duplicate stock entries and enable idempotent upserts / seeds.
- **Provider Agility**: If alternative brokers or data feeds (e.g., Zerodha Kite, Dhan) are integrated later, the domain model remains intact by adding provider mappings rather than rewriting tickers.

### Negative
- Live feed ingestion requires mapping inbound `instrument_key` to internal `stock_id` on first encounter, requiring an in-memory concurrent map cache in the streaming engine.
