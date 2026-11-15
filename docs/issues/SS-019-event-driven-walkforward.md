# SS-019 — Point-in-Time Event-Driven Engine + Walk-Forward (IS/OOS) Harness

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-018 |
| Blocks | SS-022, SS-026 (shares the bar-close contract) |
| Size | ~2.5 days |
| Stack | Python |

## Goal

Give strategies a strictly causal execution mode (`on_bar(current_bar, history_window)`) that is structurally
incapable of seeing the future, and a train/test split harness that exposes overfitting before deployment.

## Scope

**In scope**
- `engine/backtest/event_driven.py`:
  - Loop that yields `history_window` slices only (a read-only view, capped at `warmup_bars + N`), then calls
    `strategy.on_bar(current_bar, history)`; signals execute at the next bar's open via the **same** fill
    machinery as SS-018 (shared `ExecutionSimulator`).
  - `history_window` is a `pd.DataFrame` with a flag preventing in-place mutation that would leak state across
    bars (copy-on-write + a mutation guard).
  - Parity guarantee: an event-driven strategy that simply reproduces a vectorized signal column must produce
    **identical** trades to SS-018 within tolerance 0 (`tests/test_engine_parity.py`).
- Walk-forward harness `engine/backtest/walkforward.py`:
  - `split(df, is_pct=0.7, mode="chronological"|"rolling", folds=n)`.
  - `run_walkforward(df, strategy_factory, param_grid, config)` → per-fold metrics, in-sample vs out-of-sample
    comparison, degradation ratio, and a `overfit_flag` heuristic.
  - Parameter grid support (from spec §3.10): cartesian product, deterministic ordering, run-count cap.
- `engine/backtest/param_grid.py`: grid expansion + result ranking by a chosen metric with tie-breaks.
- Output schema extension: `folds[] {is_metrics, oos_metrics, params, degradation}`.

**Out of scope**
- Running these across the whole universe (SS-020/022). Optimising with ML (out of scope entirely).

## Small architecture

```text
for i in range(warmup, len(df)):
    history = df.iloc[max(0,i-window):i]        # strictly < i, no copy leak
    sig = strategy.on_bar(df.iloc[i], history)
    simulator.on_signal(t=i, signal=sig)        # fills at i+1 open, shared with SS-018
```

* `history_window` rows are exposed via `HistoryView` wrapper that raises on `.iloc[len]` beyond the window and
  on mutation — a guard against the exact bug the spec calls out.
* Pointer discipline: never hand the full `df` to `on_bar`, never pass `future_bars`, never expose `len(df)`.
* The shared `ExecutionSimulator` class is extracted from SS-018 so both engines cannot drift; that refactor is
  part of this issue (with SS-018's golden tests kept green).
* Walk-forward ordering is strictly chronological; folds never overlap in the OOS segment; a rolling mode
  produces expanding/rolling windows with documented sizes.
* `overfit_flag` = true when median OOS Sharpe < 0.5 × median IS Sharpe or OOS profit factor < 1 while IS > 1.5.

## Tasks

- [ ] Extract `ExecutionSimulator` from SS-018; keep goldens passing (refactor commit).
- [ ] `on_bar` loop + `HistoryView` guard + warmup handling.
- [ ] Parity test: vectorized vs event-driven identical trades on 3 fixtures.
- [ ] `walkforward.split` + `run_walkforward` + degradation/overfit heuristics.
- [ ] `param_grid` expansion + ranking with deterministic tie-breaks.
- [ ] CLI `python -m engine.backtest.walkforward --csv … --grid fast=9,21 --folds 4`.
- [ ] ADR `docs/adr/0019-walkforward-semantics.md`.

## Acceptance criteria

1. A reference strategy implemented both ways produces identical trade lists and metrics (exact equality).
2. `HistoryView` raises if the strategy tries to read row `i` (current) or `i+1`; a test proves the guard works.
3. `split(is_pct=0.7)` on a 1 000-bar frame yields 700 in-sample and 300 out-of-sample bars with no overlap and
   no time travel (IS end < OOS start).
4. `run_walkforward` with 4 folds and a 4-combination grid returns 16 result rows, each with IS/OOS metrics and a
   `degradation` value; the sums of trade counts reconcile with a manual count.
5. A deliberately overfit strategy (parameter memorising the IS period) yields `overfit_flag=true`.
6. Total runtime for 4 folds × 4 params × 50k bars is < 10 s locally.

## Test plan

- Parity test (the centrepiece) across 3 fixture datasets and 2 execution paths.
- Guard tests proving no full-frame access.
- Determinism: run twice, compare JSON hashes.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.6 (look-ahead prevention, IS/OOS split) and §3.10 (parameter
grid), docs/issues/SS-019-event-driven-walkforward.md. Repo: engine/backtest/vectorized.py from SS-018 with
its fill/cost/metrics pipeline and golden tests; engine/sdk BaseStrategy has on_bar.

Implement SS-019 only.
1. Refactor: extract the shared ExecutionSimulator (signal -> next-bar-open fill -> participation cap ->
   slippage -> charges -> trades/equity) out of SS-018 into engine/backtest/execution.py and reuse it in both
   engines. SS-018 goldens must still pass unchanged (separate refactor commit).
2. engine/backtest/event_driven.py: iterate bars, hand on_bar(current_bar, history_window) a HistoryView of rows
   strictly before the current bar, capped at warmup_bars + N. HistoryView must raise on reading the current or
   a future row and on mutation, so a strategy cannot leak state or peek ahead. Signals execute through the
   shared simulator at the next bar's open.
3. Parity guarantee test: the same logic written vectorized and event-driven must produce identical trades and
   metrics (exact equality) across 3 fixtures.
4. engine/backtest/walkforward.py: split(df, is_pct=0.7, mode=chronological|rolling, folds=n) with no overlap
   and IS strictly before OOS; run_walkforward(df, factory, param_grid, config) returning folds[] with
   is_metrics, oos_metrics, params, degradation; an overfit_flag heuristic (OOS sharpe < 0.5*IS sharpe, or OOS
   PF < 1 while IS PF > 1.5). Deterministic param-grid expansion with ranking and stable tie-breaks.
5. CLIs for both engines, JSON output, and an ADR 0019 documenting fold semantics, degradation definition and
   what the overfit flag does and does not mean.

Constraints: never pass the full frame or len(df) to user code; no DB/network/sandbox here; runtime for
4 folds x 4 params x 50k bars must stay under 10s.

Done when: parity test passes exactly; HistoryView guard tests pass; split has zero overlap and correct sizes;
walkforward returns 4x4=16 rows with reconciling trade counts; a deliberately overfit strategy raises the flag;
double-run JSON is byte-identical.
```

## Notes / risks

- Copy-on-write views can silently share memory with the source frame; the mutation guard must be tested, not
  assumed.
- Walk-forward on intraday data must split by **sessions**, not row counts, or a fold can straddle a gap
  (weekend). Use `marketcal` to define fold boundaries.
