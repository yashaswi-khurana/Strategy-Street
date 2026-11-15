# SS-032 — Backtest Runner, Sector Heatmap & Stock-Strategy Matrix

| | |
| --- | --- |
| Milestone | 5 — Frontend |
| Depends on | SS-024, SS-029 |
| Blocks | — |
| Size | ~3 days |
| Stack | Angular 18, ECharts (or CSS grid for heatmap) |

## Goal

Turn hundreds of backtest rows into decisions: live progress while it runs, then a sector heatmap and a
stock × strategy matrix that answer "where does this strategy work?" at a glance.

## Scope

**In scope**
- `features/backtests`:
  - Run list + run detail with status chips (`QUEUED/RUNNING/COMPLETED/COMPLETED_WITH_ERRORS/FAILED/CANCELLED`),
    progress bar driven by `BACKTEST_PROGRESS` WS frames (with REST fallback polling every 5 s if the socket is
    down), ETA, cancel button, coverage warnings (`422` payload listing under-covered symbols).
  - Results table: sortable columns (net pnl, return %, win rate, profit factor, Sharpe, Sortino, Calmar, MDD,
    expectancy, trades), pagination via keyset cursor, CSV export of the current filter, per-row link to the chart
    with markers preloaded (`/chart?symbol=…&tf=…&run=…`).
  - Distribution panel: histogram of per-stock returns, count of profitable vs losing stocks, `insufficient_data`
    count.
- `features/analytics`:
  - Sector heatmap: sectors × metrics (score, win rate, median Sharpe, PF, trade count), colour-scaled with an
    accessible palette + numeric labels (never colour-only), click-through to filtered results.
  - Stock-strategy matrix: rows = symbols, columns = strategies (top N by score), cells = score/return, sortable,
    sticky headers, virtual scrolling for 500 rows, cell click → detail drawer with the score breakdown from
    SS-024.
  - "Best strategy for this stock" card reusable by the chart page.
- Shared: `MetricValueComponent` (formats money in ₹ with Indian digit grouping, percentages, ratios, `n/a` for
  nulls — never 0), `RunProgressService` (WS + polling fallback + terminal state handling).

**Out of scope**
- Paper trading dashboards (SS-033). Building new metrics (SS-017/023/024).

## Small architecture

```text
BacktestRunStore (signals)
  ├─ WS BACKTEST_PROGRESS -> progress signal (throttled render)
  └─ REST fallback poll (5s) whenever connectionState !== 'live'

ResultsTable -> keysetCursor pagination, server-side sort (whitelist mirrored in the UI types)
SectorHeatmap -> pure mapper(rows, metric) -> {cells[{value, colour, label, tooltip}]}
MatrixView -> virtual scroll + column pinning; cells derived from GET /analytics/... payload
```

* Server-side sorting/pagination only — the client never loads 500 rows to sort them locally (it does load one
  page of ≤ 50).
* The heatmap's colour scale is computed per run (min/max of the visible metric) and always printed as a number in
  the cell, so colour-blind users and screenshots stay readable.
* Progress rendering is throttled to animation frames; a late/duplicate frame can't move the bar backwards
  (monotonic guard).
* All money formatting goes through one pipe: ₹ + `Intl.NumberFormat('en-IN')`, two decimals, negative in red with
  an explicit minus sign; ratios to 2 dp; `null` renders `n/a` in muted grey.

## Tasks

- [ ] Run list/detail + progress service with WS + polling fallback and cancel.
- [ ] Results table with server sort/pagination, CSV export, chart deep-link.
- [ ] Distribution panel (histogram + profitable/losing counts).
- [ ] Sector heatmap with accessible palette, tooltips and drill-down.
- [ ] Matrix view with virtual scroll, sticky headers and detail drawer.
- [ ] Shared formatting pipe/component + unit tests for Indian number formatting and nulls.
- [ ] Component tests with fixtures; Playwright flow for a completed run.

## Acceptance criteria

1. Opening a running backtest shows live progress that never regresses and flips to the results table on
   completion without a manual refresh.
2. With the WS down, progress still updates via polling within 5 s and a "degraded" indicator is visible.
3. Cancel from the UI transitions the run to `CANCELLED` and stops progress updates.
4. Sorting by Sharpe desc then paging returns stable, non-duplicated rows (keyset cursor verified in the network
   tab / test).
5. CSV export matches the current sort/filter and includes every visible column with correct ₹ formatting.
6. Heatmap: a sector with `insufficient_data` is visually distinct (hatched/muted) and never reads as best;
   every cell shows its numeric value as text.
7. Matrix renders 500 rows × 8 strategies without jank (< 100 ms per scroll frame) and cell click opens the score
   breakdown drawer.
8. `null` metrics render `n/a`, never `0`, and money uses Indian digit grouping (₹1,00,000, not ₹100,000).

## Test plan

- Component tests with committed JSON fixtures for run/results/analytics.
- A11y check: heatmap contrast + keyboard navigation for the drawer.
- Perf test for the matrix with 500×8 synthetic rows.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §8 (sector scoring, best strategy per stock) and §2.4, §5.3, §9,
docs/issues/SS-032-analytics-dashboards.md. Repo: Angular 18 shell with generated API client and MarketStreamService
(SS-029); backend provides GET /backtests, /backtests/{id}, POST /backtests/{id}/cancel,
GET /backtests/{id}/results?sort=&cursor=&limit=, /results/{symbol}, /summary, /backtests/compare,
GET /analytics/sectors/{strategy_id}, GET /analytics/stocks/{symbol}/best-strategies, plus WS
BACKTEST_PROGRESS frames; SS-024 returns score_breakdown.

Implement SS-032 only: features/backtests and features/analytics.
1. Run list + run detail: status chips, progress bar fed by BACKTEST_PROGRESS (monotonic — never moves backwards),
   ETA, cancel button, coverage-warning display for 422 payloads, and an automatic switch to the results table on
   terminal status. Fall back to 5s REST polling whenever connectionState !== 'live', with a visible degraded hint.
2. Results table: server-side sort (whitelist) + keyset cursor pagination, columns for net pnl, return %, win
   rate, profit factor, Sharpe, Sortino, Calmar, MDD, expectancy, trades; CSV export of the current sort/filter;
   row link deep-links to /chart?symbol=...&tf=...&run=....
3. Distribution panel: histogram of per-stock returns, profitable vs losing counts, insufficient_data count.
4. Sector heatmap: sectors x metrics (score, win rate, median Sharpe, PF, trade count) with a per-run colour scale,
   numeric values printed in every cell, accessible palette, tooltips, insufficient_data cells hatched/muted, and
   click-through to filtered results.
5. Stock-strategy matrix: symbols x top strategies, virtual scrolling for 500 rows, sticky headers, cell = score/
   return, cell click opens a drawer showing the score_breakdown from SS-024. Extract a reusable
   "best strategy for this stock" card for the chart page.
6. Shared MetricValue component/pipe: ₹ with Intl.NumberFormat('en-IN'), 2dp for money, ratios 2dp, nulls -> 'n/a'
   in muted grey (never 0), negatives with an explicit minus and red.

Constraints: never load the full result set to sort/filter client-side; no new metric computation; no paper
dashboards (SS-033); accessibility: no colour-only encoding, keyboard-reachable drawer.

Done when: live progress is monotonic and completes without refresh; WS-down falls back to polling; cancel works;
sorting+paging never duplicates rows; CSV reflects the active filter; insufficient_data sectors are visually
distinct and never rank first; 500x8 matrix scrolls smoothly (<100ms/frame); nulls render n/a and ₹1,00,000 formats
correctly. Include fixtures and the a11y/perf results.
```

## Notes / risks

- Heatmap colour scales invite misreading across runs — pin the legend to the run and label the score as
  "relative within this run".
- Cancelling a run does not delete partial results; the table must clearly mark a `CANCELLED` run's data as partial.
