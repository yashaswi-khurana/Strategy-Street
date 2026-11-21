# SS-006 — Corporate Actions & Split/Dividend Adjustment Pipeline

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-002, SS-003, SS-004, SS-005 |
| Blocks | SS-020 (backtests must never see a phantom 80% crash) |
| Size | ~2 days |
| Stack | PostgreSQL 16 / TimescaleDB, Go 1.23 |

## Goal

Make historical prices comparable across splits, bonuses, dividends, and rights issues so indicators and cross-year backtests are not corrupted by artificial price discontinuities.

## Scope

**In scope**
- Table `corporate_actions` (splits, bonuses, dividends, rights) with `ex_date`, ratios, source, `verified`.
- Table `adjustment_factors` (stock_id, ex_date, factor NUMERIC(18,10)) storing cumulative backward adjustment factors.
- Table `data_quality_issues` storing flagged jumps and quarantined actions.
- Backward adjustment factor computation in Go (`internal/store/corporate`).
- Database SQL function `adjusted_candles(p_stock INT, p_tf TEXT, p_from TIMESTAMPTZ, p_to TIMESTAMPTZ)` supporting all 13 timeframe relations.
- CLI jobs `ca-recompute` (`make ca-recompute --stock=RELIANCE`) and `ca-detect` (`make ca-detect --stock=...`).
- Historical seeds with 6 real corporate actions.
- ADR `docs/adr/0006-adjustment-method.md`.

**Out of scope**
- Adjusting volume or open interest.
- Mutating raw hypertable candle rows.

## Acceptance Criteria

1. For a stock with a 1:5 split on 2024-06-14 and pre-split close of 2500, `adjusted_candles` returns 500 for the day before the ex-date and 500 for the ex-date itself — no visible jump.
2. Volume is unchanged by adjustment.
3. Running `make ca-recompute` twice yields identical `adjustment_factors` (idempotent).
4. An unverified action leaves prices unadjusted and appears in `data_quality_issues`/report output.
5. A 60% overnight drop with no corporate action raises exactly one `data_quality_issues` row.

