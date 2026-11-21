# Contract: Corporate Actions & Price Adjustment (SS-006)
<!-- version: 1, status: FROZEN -->

## 1. Database Schema

### 1.1 `corporate_actions`
```sql
CREATE TABLE IF NOT EXISTS corporate_actions (
  id           BIGSERIAL PRIMARY KEY,
  stock_id     INT NOT NULL REFERENCES stocks(id),
  action_type  TEXT NOT NULL CHECK (action_type IN ('split','bonus','dividend','rights')),
  ex_date      DATE NOT NULL,
  ratio_from   NUMERIC(12,4),   -- split 1:5 => from=1, to=5; bonus 1:1 => from=1, to=1
  ratio_to     NUMERIC(12,4),
  dividend_amt NUMERIC(12,4),   -- cash dividend amount in INR
  source       TEXT NOT NULL,   -- e.g. 'NSE_CORPORATE_FEED', 'MANUAL_SEED'
  verified     BOOLEAN NOT NULL DEFAULT FALSE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stock_id, action_type, ex_date)
);

CREATE INDEX IF NOT EXISTS corporate_actions_stock_ex_date_idx ON corporate_actions (stock_id, ex_date);
```

### 1.2 `adjustment_factors`
```sql
CREATE TABLE IF NOT EXISTS adjustment_factors (
  stock_id INT NOT NULL REFERENCES stocks(id),
  ex_date  DATE NOT NULL,
  factor   NUMERIC(18,10) NOT NULL,   -- cumulative factor to multiply prices strictly before ex_date
  PRIMARY KEY (stock_id, ex_date)
);

CREATE INDEX IF NOT EXISTS adjustment_factors_stock_ex_date_idx ON adjustment_factors (stock_id, ex_date DESC);
```

### 1.3 `data_quality_issues`
```sql
CREATE TABLE IF NOT EXISTS data_quality_issues (
  id          BIGSERIAL PRIMARY KEY,
  stock_id    INT NOT NULL REFERENCES stocks(id),
  issue_type  TEXT NOT NULL,           -- 'unexplained_jump', 'unverified_action'
  check_date  DATE NOT NULL,
  details     JSONB NOT NULL DEFAULT '{}'::jsonb,
  resolved    BOOLEAN NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stock_id, issue_type, check_date)
);

CREATE INDEX IF NOT EXISTS data_quality_issues_stock_idx ON data_quality_issues (stock_id, resolved);
```

---

## 2. Mathematical Formulas (Backward Adjustment)

Backward adjustment keeps recent/current market prices unchanged (1.0 factor) while scaling historical prices down (or up) to maintain series continuity across corporate actions.

For a stock with verified corporate actions ordered chronologically:
1. **Split**:
   - `price_ratio = ratio_to / ratio_from`
   - `action_factor = 1.0 / price_ratio = ratio_from / ratio_to`
   - Example: 1:5 split (`from=1`, `to=5`) $\implies$ `action_factor = 1 / 5 = 0.20`.
2. **Bonus**:
   - If `ratio_to > ratio_from` (direct share multiplier): `action_factor = ratio_from / ratio_to`.
   - If `ratio_to <= ratio_from` (bonus shares per existing shares held): `action_factor = ratio_from / (ratio_from + ratio_to)`.
   - Example: 1:1 bonus (`from=1`, `to=1`) $\implies$ `action_factor = 1 / (1 + 1) = 0.50`.
   - Example: 1:2 bonus (1 bonus for 2 held, `from=2`, `to=1`) $\implies$ `action_factor = 2 / (2 + 1) = 2/3 = 0.6666666667`.
3. **Dividend**:
   - `action_factor = 1.0 - (dividend_amt / prev_close)`
   - Where `prev_close` is the closing price on the trading day strictly before `ex_date`.
4. **Cumulative Backward Factor**:
   - For any action at `ex_date`, the factor stored in `adjustment_factors` applies to prices strictly before that `ex_date`.
   - The cumulative factor stored at `ex_date` is the product of its own action factor and all later verified action factors:
     $$\text{factor}(T_i) = \prod_{j=i}^{k} f_j$$
   - For any candle timestamp $t$ on calendar date $D$, the applicable factor is:
     $$M(D) = \begin{cases} \text{factor}(\min \{ T \in \text{ex\_dates} \mid T > D \}) & \text{if } \exists T > D \\ 1.0 & \text{otherwise} \end{cases}$$

---

## 3. SQL Query-Time Adjustment Function

```sql
CREATE OR REPLACE FUNCTION adjusted_candles(
  p_stock INT,
  p_tf TEXT,
  p_from TIMESTAMPTZ,
  p_to TIMESTAMPTZ
)
RETURNS TABLE (
  bucket_time TIMESTAMPTZ,
  open NUMERIC,
  high NUMERIC,
  low NUMERIC,
  close NUMERIC,
  volume BIGINT
)
```
- Selects dynamically from `candles_<p_tf>` where `p_tf` $\in$ {`1s`, `1m`, `5m`, `10m`, `15m`, `30m`, `1h`, `3h`, `1d`, `15d`, `1mo`, `6mo`, `1y`}.
- `open`, `high`, `low`, `close` are multiplied by the cumulative factor and rounded to 4 decimals (`ROUND(..., 4)`).
- `volume` and `oi` are strictly unmodified.
- Raw candle hypertables are never updated.

---

## 4. Recomputation & Idempotency Rules

1. `RecomputeStockFactors(stock_id)`:
   - Runs in a transaction.
   - Deletes existing rows in `adjustment_factors` for `stock_id`.
   - Selects all verified rows from `corporate_actions` where `verified = TRUE` ordered by `ex_date DESC`.
   - For dividends, queries `candles_1d` (or latest daily close) strictly prior to `ex_date`.
   - Computes cumulative product backward from most recent to oldest.
   - Inserts into `adjustment_factors`.
   - Calling twice produces byte-identical rows.

2. Unverified Actions:
   - Rows with `verified = FALSE` are completely ignored during factor computation.
   - Flagged by the jump detection / audit job into `data_quality_issues` as `unverified_action`.

3. Jump Detection:
   - Compares consecutive daily close prices from `candles_1d`.
   - If $|(C_{t} - C_{t-1}) / C_{t-1}| > 0.35$ (i.e. $> \pm 35\%$) and no verified corporate action exists on day $t$, inserts a row into `data_quality_issues` with `issue_type = 'unexplained_jump'`.
   - Idempotent via `ON CONFLICT (stock_id, issue_type, check_date) DO NOTHING`.

