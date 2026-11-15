# SS-018 — Vectorized Single-Stock Backtest Engine

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-017 |
| Blocks | SS-019, SS-022 |
| Size | ~2.5 days |
| Stack | Python, pandas, numpy |

## Goal

The first end-to-end simulation: given a price frame and a strategy, produce trades, an equity curve, metrics
and an equity/drawdown series — with costs, slippage, participation caps and no look-ahead leakage.

## Scope

**In scope**
- `engine/backtest/vectorized.py`:
  - `run(df, strategy, config) -> BacktestResult` where `config` carries timeframe, initial capital, sizing
    rule, cost config, slippage model, participation cap, long-only flag.
  - Order generation at `signal` bar **t+1 open** (next-bar execution) — never at the signal bar's close; this
    single rule kills most accidental look-ahead.
  - Position model: flat/long (shorting behind a flag), one position at a time by default, pyramiding off.
  - Sizing: `fixed_capital`, `pct_equity`, `fixed_qty` (reuses SS-025's rule enum shape).
  - Integrates SS-016 for fills/charges and SS-017 for metrics; produces `equity_series` (per bar) and
    `drawdown_series` for charting.
  - `BacktestResult` JSON schema: `{metrics, trades[], equity[{ts, value}], drawdown[{ts, value}],
    indicator_series{name: [{ts, value}]}, meta{...}}`, size-capped.
- CLI: `python -m engine.backtest.run --csv fixture.csv --strategy ema_cross --tf 5m --capital 100000`.
- Validation: refuses frames with NaN/unsorted timestamps/duplicate buckets (reuses SS-005 policies).

**Out of scope**
- Event-driven/point-in-time mode and walk-forward (SS-019).
- Full-universe fan-out and job queue (SS-020/022); DB access (the engine receives frames).

## Small architecture

```text
load frame ─► sanity policy (sort, dedupe, gap-fill) ─► strategy.generate_signals
           ─► shift signals by 1 bar (execution lag)  ─► position sizing
           ─► fills: participation cap ─► slippage price ─► charges (SS-016)
           ─► trades[] + equity[] ─► metrics (SS-017) ─► BacktestResult
```

* Vectorised where possible, but the fill loop is explicit and ordered — one trade at a time, deterministic
  iteration, no groupby surprises.
* Costs are charged per executed order (entry and exit legs separately), matching SS-016's per-order brokerage.
* Long-only by default; `allow_short` is a config flag with its own tests (SELL closes long vs opens short).
* Cancellation: `run(..., cancel_check: Callable[[], bool])` polled every 10 000 bars so SS-020 can abort.
* `meta` records `rules_version`, `cost_config_version`, `signal_lag_bars=1`, data range, bar count — required
  for reproducibility and for the paper-trading parity checks in SS-028.

## Tasks

- [ ] `BacktestResult` types + JSON schema + size guard.
- [ ] Signal→order→fill pipeline with next-bar execution and explicit no-look-ahead test.
- [ ] Sizing rules + participation cap integration.
- [ ] Equity/drawdown/indicator series construction (IST-aware timestamps).
- [ ] CLI + `--json out.json` and human-readable summary.
- [ ] Fixture suite: 5 synthetic scenarios with hand-computed expectations (one winner, one loser, one
      no-trade, one cap-limited, one shorting).
- [ ] ADR `docs/adr/0018-execution-model.md` (next-bar open, no partial fills, one position at a time).

## Acceptance criteria

1. A deterministic fixture (known prices, one buy signal, one sell signal) yields exactly the hand-computed
   `net_pnl` including charges, and `trades` with correct entry/exit timestamps **one bar after** the signals.
2. Removing the execution lag would change results — a test asserts entry price equals the *next* bar's open.
3. `pct_equity` sizing recalculates quantity per trade from current equity; `fixed_capital` does not.
4. Participation cap: a signal on a 200-volume 1s bar with a 10% cap can fill at most 20 shares; the residual
   is dropped (not partially carried), and this is recorded in `meta.limitations`.
5. `equity[-1].value == initial_capital + Σ net_pnl` exactly (to the paisa).
6. A frame with duplicate timestamps or NaN closes is rejected with a typed error before any computation.
7. `cancel_check` returning true stops within 1 s and returns a `cancelled` status.

## Test plan

- Golden-fixture tests with committed expected JSON.
- Contrarian test: a momentum strategy on a monotonically falling synthetic series must lose money.
- Cross-check buy_and_hold metrics against analytically computed values.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.4, §3.2, §3.3, §6, docs/issues/SS-018-vectorized-backtest.md.
Repo: engine/sdk BaseStrategy (SS-013), engine/costs (SS-016), engine/metrics (SS-017), engine/marketcal
mirror (SS-005).

Implement SS-018 only: engine/backtest/vectorized.py — a single-stock vectorized backtester producing trades,
equity and metrics.
1. API: run(df, strategy, config, cancel_check=None) -> BacktestResult where config has timeframe, initial
   capital, sizing (fixed_capital|pct_equity|fixed_qty), cost config, slippage model, participation cap,
   allow_short (default false), and long-only one-position-at-a-time semantics by default.
2. Execution model: signals generated on bar t are executed at bar t+1 OPEN (signal_lag_bars=1). Never fill at
   the signal bar's close. Document and test this explicitly.
3. Fills: apply participation cap to quantity first (integer, residual dropped), then slippage on the
   reference price, then per-order charges via SS-016 on each leg. Costs are charged per executed order.
4. Output BacktestResult: metrics (SS-017), trades[], equity[{ts,value}], drawdown[{ts,value}],
   indicator_series from plot_overlays/plot_subcharts, and meta{rules_version, cost_config_version,
   signal_lag_bars, timeframe, bars, from, to, limitations[]}. Include a JSON schema and a size guard.
5. Input hygiene: reject NaN prices, unsorted or duplicate bucket_time, empty frames — reuse the SS-005 gap
   policy decorator rather than ad-hoc fixes.
6. CLI: python -m engine.backtest.run --csv fixture.csv --strategy ema_cross --tf 5m --capital 100000 [--json].
   cancel_check is polled every 10k bars and returns status=cancelled within 1s.

Constraints: no DB, no network, no sandbox in this issue; deterministic iteration order; timestamps IST-aware;
long-only unless allow_short is set (with its own tests).

Done when: the hand-computed fixture matches to the paisa with entries one bar after signals; the execution-lag
test fails if lag is set to 0; pct_equity vs fixed_capital sizing behave differently as specified; a 10% cap on
200 volume fills at most 20 shares and records the limitation; equity[-1] equals initial capital plus summed
net PnL; bad frames are rejected with typed errors; cancellation works. Commit the fixture goldens.
```

## Notes / risks

- "Vectorized" must not become "signal at close, fill at close" — the whole point is the one-bar lag. Keep the
  lag a named config value recorded in `meta`.
- Shorting on Indian equities needs margin/borrow modelling that this issue doesn't do; keep shorts behind a
  flag and label results as indicative.
