# ADR 0006: Corporate Actions Backward Adjustment & Dividend Method

## Status
Accepted

## Context
When a listed company undergoes a corporate action (such as a 1:5 stock split, a 1:1 bonus issue, or a significant cash dividend), the market price per share experiences an instantaneous artificial step-change on the `ex_date`.
For example, when a ₹2,500 stock splits 1:5 on 2024-06-14, the post-split shares open near ₹500.
If unadjusted:
1. Technical indicators (e.g. 50-day moving average, RSI, Bollinger Bands) observe a phantom 80% market crash, generating catastrophic false sell signals in backtests (violating Milestone 1 requirements for SS-020).
2. Cross-year performance and drawdowns become meaningless.
3. Raw data in TimescaleDB cannot simply be rewritten in place because regulatory compliance, auditability, and trade reconciliation require historical execution prices to remain immutable truth.

## Decisions

### 1. Backward Adjustment vs Forward Adjustment
We adopt **Backward Adjustment** as the foundational standard for Strategy Street:
- **Backward Adjustment**: Multiplies all historical prices *prior* to the `ex_date` by the cumulative adjustment factor ($f < 1.0$). Current and recent market prices remain identical to live quote prices ($f = 1.0$).
  - *Why*: Traders and quant models compare indicators to the current market price. An order placed today uses today's nominal price. If forward adjustment were used, historical prices would stay fixed and today's price would be distorted, confusing live execution engines and human operators.
- **Volume and Open Interest**: Volume and open interest are **never adjusted**. Volume represents physical contracts/shares traded at that specific instant; altering volume distorts market microstructure analysis, VWAP calculations, and liquidity filters.

### 2. Action Factor Mathematics
All calculations are strictly backward-looking:
1. **Stock Split**:
   $$\text{factor} = \frac{\text{ratio\_from}}{\text{ratio\_to}}$$
   For a 1:5 split (`from=1, to=5`): $\text{factor} = 1 / 5 = 0.20$.
2. **Bonus Issue**:
   $$\text{factor} = \frac{\text{ratio\_from}}{\text{ratio\_from} + \text{ratio\_to}}$$
   For a 1:1 bonus (`from=1, to=1`): $\text{factor} = 1 / (1 + 1) = 0.50$.
   If direct conversion ratio is stored (`from=1, to=2`): $\text{factor} = 1 / 2 = 0.50$.
3. **Cash Dividend**:
   $$\text{factor} = 1.0 - \frac{\text{dividend\_amt}}{\text{prev\_close}}$$
   Where `prev_close` is the unadjusted closing price on the trading session immediately preceding the `ex_date`.
   *Rationale*: A cash dividend extracts cash from the firm's balance sheet. On the ex-dividend date, the stock opens approximately `prev_close - dividend_amt` lower. Scaling earlier prices by $1 - (\text{dividend} / \text{prev\_close})$ neutralizes the dividend drop in continuous total-return series without inventing synthetic cashflows.
4. **Cumulative Product**:
   If a stock experiences corporate actions at ex-dates $T_1 < T_2 < \dots < T_k$, the cumulative backward factor for any candle at date $D$ is the product of all action factors strictly occurring after $D$:
   $$M(D) = \prod_{T_i > D} f_i$$

### 3. Query-Time Dynamic Adjustment Function (`adjusted_candles`)
Adjustment is applied **at query time** inside PostgreSQL via a stored function `adjusted_candles(p_stock, p_tf, p_from, p_to)`.
- **Zero Raw Mutation**: Raw hypertables (`candles_1s`, `candles_1m`) and continuous aggregates (`candles_5m`..`candles_1y`) store 100% pure raw trade data (`is_adjusted = false`).
- **No Python Reimplementation**: Python backtesting clients query `adjusted_candles()` directly. Neither the Python engine nor external scripts re-implement adjustment factor accumulation, preventing cross-language formula drift.
- **Index-Backed Lateral Join**: The function performs a `LEFT JOIN LATERAL` on `adjustment_factors` using index `(stock_id, ex_date ASC)`, evaluating in single-digit microseconds per range scan.

### 4. Verification Gate & Quality Quarantine
- Only corporate actions with `verified = TRUE` participate in factor computation.
- Unverified actions are stored in `corporate_actions` with `verified = FALSE` and quarantined in `data_quality_issues` as `unverified_action` until approved by an administrator.
- An overnight price jump $> \pm 35\%$ with no verified corporate action triggers an automatic `unexplained_jump` issue in `data_quality_issues`.

### 5. Idempotent Recomputation
`make ca-recompute --stock=SYMBOL` recalculates `adjustment_factors` deterministically by wiping prior factors for the symbol and re-accumulating the chain of verified actions. Re-running the command $N$ times results in identical state.

## Consequences

### Positive
- Cross-language parity: Go API and Python engine receive identical adjusted price streams directly from the database.
- Immutability: Raw trade and hypertable rows remain pristine and audit-compliant.
- Safety: Malicious or erroneous unverified seeds cannot silently alter historical price charts.
- Idempotency: Factor tables can be dropped and rebuilt at any time from `corporate_actions`.

### Negative / Trade-offs
- Query overhead: Range queries using `adjusted_candles()` incur a lightweight lateral join compared to direct hypertable scans.
- Dependency on daily closes: Dividend factor computation requires valid `candles_1d` close prices on the day preceding `ex_date`.

