# SS-006 — Corporate Actions & Split/Dividend Adjustment Pipeline

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-002 |
| Blocks | SS-020 (backtests must never see a phantom 80% crash) |
| Size | ~2 days |
| Stack | PostgreSQL, Go |

## Goal

Make historical prices comparable across splits, bonuses and dividends so indicators and cross-year backtests
are not corrupted by artificial gaps.

## Scope

**In scope**
- Table `corporate_actions` (splits, bonuses, dividends, rights) with `ex_date`, ratios, source, `verified`.
- Adjustment-factor computation: cumulative backward factor per stock, applied to OHLC (not volume).
- Views `v_candles_adj_<tf>` (or a `prices_adjusted()` SQL function) used by backtest loaders; raw tables
  remain untouched.
- A detection job that flags unexplained overnight jumps (> ±35% with no corporate action) into
  `data_quality_issues` for manual review.
- Go repository `internal/store/corporate` + `make ca-recompute --stock=RELIANCE`.

**Out of scope**
- Fetching corporate actions from a paid vendor (seeds + CSV import are enough).
- Adjusting volume or open interest.

## Small architecture

```sql
CREATE TABLE corporate_actions (
  id           BIGSERIAL PRIMARY KEY,
  stock_id     INT NOT NULL REFERENCES stocks(id),
  action_type  TEXT NOT NULL CHECK (action_type IN ('split','bonus','dividend','rights')),
  ex_date      DATE NOT NULL,
  ratio_from   NUMERIC(12,4),   -- split 1:5 => from=1, to=5
  ratio_to     NUMERIC(12,4),
  dividend_amt NUMERIC(12,4),
  source       TEXT NOT NULL,
  verified     BOOLEAN NOT NULL DEFAULT FALSE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stock_id, action_type, ex_date)
);

CREATE TABLE adjustment_factors (
  stock_id INT NOT NULL REFERENCES stocks(id),
  ex_date  DATE NOT NULL,
  factor   NUMERIC(18,10) NOT NULL,   -- multiply prices strictly before ex_date
  PRIMARY KEY (stock_id, ex_date)
);
```

* Backward adjustment: `factor(action) = 1 / price_ratio` for splits/bonuses; dividends use
  `1 - div/prev_close`. Cumulative factor = product of all later actions.
* The loader applies adjustment **at query time** through a single SQL function
  `adjusted_candles(stock_id, tf, from, to)` so Python never re-implements it:

```sql
CREATE FUNCTION adjusted_candles(p_stock INT, p_tf TEXT, p_from TIMESTAMPTZ, p_to TIMESTAMPTZ)
RETURNS TABLE (bucket_time TIMESTAMPTZ, open NUMERIC, high NUMERIC, low NUMERIC,
               close NUMERIC, volume BIGINT) ...
```

* Only **verified** actions adjust prices by default; unverified ones are stored and listed in the UI as
  "pending review" (prevents a bad seed from silently rewriting history).
* Idempotent recompute: truncate factors for the stock, recompute from `corporate_actions`.

## Tasks

- [ ] Migrations for `corporate_actions`, `adjustment_factors`, `data_quality_issues`.
- [ ] Factor computation (Go, tested against hand-computed examples) + `make ca-recompute`.
- [ ] `adjusted_candles()` SQL function over any timeframe relation, keyed by `p_tf`.
- [ ] Jump detector job + CLI (`make ca-detect --stock=...`).
- [ ] Seed 6 real historical actions (e.g., a 1:1 bonus, a 1:5 split, two dividends) for test stocks.
- [ ] ADR `docs/adr/0006-adjustment-method.md` (backward vs forward, dividend handling).

## Acceptance criteria

1. For a stock with a 1:5 split on 2024-06-14 and pre-split close of 2500, `adjusted_candles` returns
   500 for the day before the ex-date and 500 for the ex-date itself — no visible jump.
2. Volume is unchanged by adjustment.
3. Running `make ca-recompute` twice yields identical `adjustment_factors` (idempotent).
4. An unverified action leaves prices unadjusted and appears in `data_quality_issues`/report output.
5. A 60% overnight drop with no corporate action raises exactly one `data_quality_issues` row.

## Test plan

- Unit tests on factor math: split, bonus 1:1, 1:2, dividend-only, and a chain (split then bonus).
- Integration test: seed candles + action, assert adjusted series continuity at the ex-date.
- Idempotency test for the recompute job.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.4 (corporate actions hygiene),
docs/issues/SS-006-corporate-actions.md. Repo: Go 1.23 + pgx, TimescaleDB with candles_1s..candles_1y
(IST-aligned bucket_time, is_adjusted flag currently false), metadata tables from SS-002.

Implement SS-006 only. Create tables corporate_actions (stock_id, action_type in split/bonus/dividend/rights,
ex_date, ratio_from, ratio_to, dividend_amt, source, verified, unique on stock/action/ex_date),
adjustment_factors (stock_id, ex_date, factor NUMERIC(18,10)) and data_quality_issues.

Implement backward adjustment: for splits/bonuses factor = 1/price_ratio for prices strictly before ex_date;
for dividends factor = 1 - dividend/prev_close. Cumulative factor is the product of all later verified
actions. Add a SQL function adjusted_candles(p_stock int, p_tf text, p_from timestamptz, p_to timestamptz)
returning bucket_time/open/high/low/close/volume that selects from the correct timeframe relation and
multiplies OHLC by the cumulative factor — volume and OI are never adjusted. Only verified actions adjust
prices; unverified ones only surface in a pending-review list.

Add a Go repository + CLI: `make ca-recompute --stock=RELIANCE` (idempotent: recompute factors from scratch)
and `make ca-detect --stock=...` which flags overnight moves beyond ±35% with no corporate action by inserting
into data_quality_issues. Seed six real actions for test stocks including a 1:5 split and two dividends.
Write an ADR explaining backward vs forward adjustment and the dividend formula.

Constraints: raw candle rows must never be mutated; Python must never reimplement adjustment; all date math
in Asia/Kolkata.

Done when: the 1:5 split example yields a continuous series (2500 -> 500 with no jump), volume unchanged,
recompute is idempotent, unverified actions don't move prices, and a 60% unexplained drop produces exactly
one data_quality_issues row. Include the SQL output proving the adjusted series.
```

## Notes / risks

- Dividends materially change long-horizon backtest returns; make the dividend formula configurable and
  documented, not an implicit average.
- `adjusted_candles` as a SQL function can be slow for full-universe scans — add a materialized
  `adjusted_candles_daily` later if SS-020 profiling demands it.
