# SS-030 — Unified Historical + Live Chart Module

| | |
| --- | --- |
| Milestone | 5 — Frontend |
| Depends on | SS-012, SS-029 |
| Blocks | SS-033 |
| Size | ~3 days |
| Stack | Angular 18, `lightweight-charts` |

## Goal

The core screen of the product: one canvas that shows historical candles for any symbol/timeframe, seamlessly
stitched to the live stream, with strategy overlays, buy/sell markers and an equity/drawdown sub-chart.

## Scope

**In scope**
- `features/chart`:
  - Candle pane (lightweight-charts): OHLC, volume histogram, crosshair with IST timestamps, timezone-aware axis.
  - Timeframe selector (all 13 from the spec, with 1s live-only marked), symbol search/typeahead from
    `GET /stocks?q=`.
  - Data loading: `GET /api/v1/candles/:symbol?tf=&from=&to=` for history, then WS `SNAPSHOT` + `CANDLE` for live.
    Stitching rule: de-duplicate by `bucket_time`, **update** the last bar when the same bucket arrives again,
    append when a newer bucket arrives, and never insert a bar older than the newest except via an explicit
    backfill merge. Gaps are filled per SS-005 policy and visually marked (hollow style) — never silently.
  - Backfill UX: on a gap, show a subtle "loading history…" pill using `BACKFILL_PROGRESS` frames, then merge the
    newly fetched range without losing the live tail.
  - Overlays: line series for `plot_overlays`, separate panes for `plot_subcharts` (RSI etc.), driven by a
    strategy-version selector (`GET /strategy-versions/:id/indicators`).
  - Markers: buy/sell entries from a backtest run (`GET /backtests/:runId/results/:symbol`) and/or live paper
    trades — distinct shapes/colours, tooltip with price, qty, PnL.
  - Sub-chart: equity curve + drawdown area chart synced to the main pane's time scale.
  - Performance: 20k bars render in < 300 ms; live updates at 1/s must not cause full re-renders (incremental
    `update()` calls only); `requestAnimationFrame` batching for tick frames.
- Component API: `<app-chart [symbol] [timeframe] [overlays] [markers] [mode]>` — dumb component, all data via
  inputs, all intents via outputs (testable without the network).

**Out of scope**
- Order entry from the chart (kill switch/paper controls live in SS-033).
- Drawing tools/indicators authored in the browser (indicators come from strategies).

## Small architecture

```text
ChartComponent
 ├─ inputs: symbol, timeframe, overlayConfig, markers, mode(backtest|paper|live)
 ├─ data: ChartDataService
 │     ├─ loadHistory(symbol, tf, from, to) → candles[]      (REST, paginated)
 │     ├─ stream.watch(symbol, tf) → SNAPSHOT/CANDLE frames
 │     └─ merge(existing, incoming) → {updated[], appended[]}  (pure, unit-tested)
 ├─ render: LightweightChartsAdapter (createSeries/updateBar/setMarkers/setPaneData)
 └─ outputs: barHover, rangeChange, markerClick
```

* The merge function is the heart of the issue and lives in its own pure module
  (`mergeCandles(existing, incoming, {tf})`), exhaustively tested: duplicate bucket, out-of-order arrival,
  gap-fill bar (`is_filled`), session rollover, and timeframe switch (full reset).
* Timeframes are a first-class map `{'1s','1m',…,'1y'}` shared with the backend enum; anything unknown is a type
  error, not a runtime string.
* The adapter isolates the charting library so swapping `lightweight-charts` for `ECharts` touches one file.
* Watermark/badge shows "LIVE" vs "HISTORICAL" and the last tick time, so users never mistake a frozen chart for
  a live one.

## Tasks

- [ ] `ChartDataService` (REST history + WS intents + backfill progress).
- [ ] `mergeCandles` pure module + exhaustive tests.
- [ ] `LightweightChartsAdapter` (candles, volume, overlay lines, subchart panes, markers).
- [ ] Symbol search + timeframe selector + URL sync (`/chart?symbol=TCS&tf=5m&version=12`).
- [ ] Stitching/backfill UX, `is_filled` styling, LIVE/HISTORICAL badge.
- [ ] Equity/drawdown sub-chart with synced time scale.
- [ ] Performance test harness (20k bars, 1 update/s for 60 s, assert no memory growth).

## Acceptance criteria

1. Selecting TCS/5m loads ~500 historical bars, then live bars append; `bucket_time` values are unique and
   strictly increasing in the series.
2. A live `CANDLE` for the same bucket as the last historical bar updates it in place (no duplicate, no jump).
3. A `is_filled` bar renders with the hollow/dashed style and a tooltip noting it was forward-filled.
4. Switching timeframe resets the series completely (no cross-timeframe contamination), and switching back
   re-fetches rather than showing stale data.
5. Buy/sell markers appear at the correct bars for a backtest run and tooltips show price/qty; live paper trades
   appear within 1 s of the `TRADE` frame.
6. 20 000 bars render in < 300 ms and a 60 s live session shows no unbounded memory growth (browser heap check).
7. All 13 timeframes are selectable; `1s` shows a "live only — history from session start" note.
8. Component unit tests run without a network or a real socket (adapter + services mocked).

## Test plan

- Unit tests for `mergeCandles` (≥ 15 cases incl. out-of-order and duplicate buckets).
- Component tests with fake services asserting series calls.
- Manual perf script + Playwright screenshot baseline.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.6 (unified historical + real-time canvas, overlays, markers),
§7 (side-by-side live persistence), docs/issues/SS-030-chart-module.md. Repo: Angular 18 standalone app with
core/api generated client and core/ws MarketStreamService (SS-029) exposing watch(stock, tf) intents and typed
frames SNAPSHOT/CANDLE/TICK/INDICATOR/TRADE/BACKFILL_PROGRESS; backend provides GET /api/v1/candles/:symbol?tf=&
from=&to=, /stocks?q=, /strategy-versions/:id/indicators, /backtests/:runId/results/:symbol.

Implement SS-030 only: features/chart with lightweight-charts.
1. <app-chart [symbol] [timeframe] [overlays] [subcharts] [markers] [mode]> — a dumb component: all data via
   inputs, intents via outputs, no direct HTTP/socket usage inside it.
2. ChartDataService: load history via REST, then stream.watch(symbol, tf) for SNAPSHOT/CANDLE; expose a pure
   mergeCandles(existing, incoming, {tf}) that de-duplicates by bucket_time, updates the last bar for the same
   bucket, appends newer buckets, ignores stale older bars (except explicit backfill merges), fills gaps per the
   SS-005 policy, and marks is_filled bars with a hollow/dashed style. Unit-test it exhaustively (duplicate
   bucket, out-of-order, gap-fill, session rollover, timeframe switch reset).
3. Rendering via a LightweightChartsAdapter (candles, volume histogram, overlay line series, separate panes for
   subcharts, markers). Incremental update() only — never rebuild the series on a live tick; batch tick-driven
   updates with requestAnimationFrame.
4. Timeframe selector covering all 13 timeframes (1s marked live-only), symbol typeahead from /stocks?q=, and URL
   sync /chart?symbol=TCS&tf=5m&version=12 so views are shareable.
5. Backfill UX: BACKFILL_PROGRESS frames drive a "loading history" pill, and the fetched range merges without
   dropping the live tail. LIVE/HISTORICAL badge with last tick time.
6. Buy/sell markers for a backtest run and live paper trades (distinct styles, tooltip with price/qty/PnL), plus
   an equity + drawdown sub-chart synced to the main time scale.

Constraints: no order entry, no in-browser indicators, no library code outside the adapter file; strict TS; all
timestamps IST on the axis.

Done when: TCS/5m loads 500 bars then appends live with unique increasing buckets; a same-bucket live candle
updates in place; is_filled bars are visually distinct with an explanatory tooltip; timeframe switching fully
resets; markers land on the right bars and live trades appear within 1s; 20k bars render under 300ms with no
memory growth over 60s; all 13 timeframes selectable; tests run with mocked services.
```

## Notes / risks

- Historical + live stitching is where nearly every charting bug lives; keep `mergeCandles` pure and covered, and
  never let the chart component own merge logic.
- 1s timeframes generate 22 500 bars/session — cap initial load and virtualise/paginate older history rather than
  sending a year of 1s data.
