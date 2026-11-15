# SS-017 — Quantitative Metrics Engine

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-016 |
| Blocks | SS-018, SS-023, SS-024 |
| Size | ~2 days |
| Stack | Python, numpy/pandas |

## Goal

Turn a trade list plus an equity curve into the full metric set the spec demands, with definitions pinned down
so numbers are comparable across strategies, sectors and timeframes.

## Scope

**In scope**
- `engine/metrics/`:
  - `compute(trades: list[Trade], equity: Series, bars_per_year: int, rf: float) -> Metrics`.
  - Metrics: `total_trades, wins, losses, win_rate_pct, gross_profit, gross_loss, net_pnl, return_pct,
    profit_factor, expectancy, avg_win, avg_loss, payoff_ratio, max_drawdown_pct, max_drawdown_duration_days,
    sharpe, sortino, calmar, cagr, avg_holding_period, exposure_pct, turnover`.
  - Annualisation factor derived from the **timeframe** (`1m → 375*252`, `1d → 252`, `1s → 22500*252`), not
    guessed from row count.
- Edge-case policy (explicitly tested): zero trades → `None`s not zeros; zero losses → `profit_factor=inf`
  represented as `null` + `flags: ["no_losses"]`; < 2 equity points → `sharpe=None`;
  constant equity → `sharpe=0` with a flag.
- Trade schema (`engine/metrics/types.py`): `entry_time, exit_time, side, qty, entry_price, exit_price,
  gross_pnl, charges, net_pnl, holding_bars, is_win`.
- JSON-safe serialisation (`Metrics.to_dict()`) with paise-rounded money and 4-dp ratios.
- `docs/domain/metric-definitions.md` — the formula for every field, so the UI tooltip and the engine agree.

**Out of scope**
- Producing trades/equity (SS-018/SS-026). Sector scoring (SS-024).

## Small architecture

```text
trades[] ──► equity_curve(trades, initial_capital) ──► returns series
                     │                                     │
                     ├─ drawdown(equity) ──► max_dd, duration
                     └─ ratios(returns, ann_factor, rf) ──► sharpe, sortino, calmar
trades[] ──► trade_stats() ──► win_rate, PF, expectancy, avg holding
                          └──► assemble Metrics (+ flags)
```

Key formulas (document them verbatim in the docs file):
* `profit_factor = Σ net_pnl>0 / |Σ net_pnl<0|`
* `expectancy = net_pnl / total_trades`
* `sharpe = (mean(returns) − rf/ann) / std(returns, ddof=1) * sqrt(ann)`
* `sortino` uses downside deviation of returns below `rf/ann`, `calmar = cagr / max_dd_pct`
* `max_drawdown_duration` measured in calendar days between the peak and the recovery bar
* `CAGR = (final/initial)^(365/days) − 1`

All ratio functions are pure `numpy` over a `float64` returns array; money stays `Decimal` until the boundary.
Deterministic: no random, no timezone-dependent "now".

## Tasks

- [ ] `types.py` (Trade, Metrics) + `to_dict()` with rounding.
- [ ] `equity.py` (curve construction, exposure, turnover).
- [ ] `ratios.py` (sharpe, sortino, calmar, cagr) with annualisation by timeframe.
- [ ] `drawdown.py` (max dd %, duration, recovery).
- [ ] `trade_stats.py` (win rate with zero/negligible-PnL policy, PF, expectancy, payoff).
- [ ] `compute()` orchestrator + `flags` for degenerate cases.
- [ ] Definition doc + 10 hand-built fixtures with expected values (including pathological cases).

## Acceptance criteria

1. The spec's motivating example — 9 wins of ₹10 and 1 loss of ₹500 — reports `win_rate_pct=90`,
   `net_pnl=-410`, `profit_factor=0.18`, negative expectancy. (Proves win rate alone isn't the headline.)
2. A monotonically rising equity curve reports `max_drawdown_pct=0`, `sharpe=None` or `0` with a flag, and no
   `Infinity`/`NaN` anywhere in the JSON (JSON-schema validated).
3. Zero-trade run returns `total_trades=0` and every ratio `None` with `flags=["no_trades"]`.
4. `profit_factor` is `None` + `["no_losses"]` when there are no losing trades.
5. Sharpe for a known synthetic returns series matches the hand-computed value to 1e-9.
6. `ann_factor` for `1m` equals `375*252` and appears in the output metadata.

## Test plan

- Fixture table tests (10 scenarios) with expected JSON golden files.
- Property tests: `max_dd >= 0`, `win_rate ∈ [0,100]`, `PF >= 0`, no NaNs.
- Cross-validation: recompute Sharpe/Sortino with an independent implementation (e.g. `empyrical` in a dev
  extra) — assert closeness within 1e-6.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.3 (comprehensive quantitative metrics), §5.3 (metrics columns),
docs/issues/SS-017-metrics-engine.md. Repo: Python engine/ with Decimal-based cost helpers from SS-016.

Implement SS-017 only: engine/metrics as pure, deterministic functions.
1. Types: Trade (entry_time, exit_time, side, qty, entry_price, exit_price, gross_pnl, charges, net_pnl,
   holding_bars, is_win) and Metrics with total_trades, wins, losses, win_rate_pct, gross_profit, gross_loss,
   net_pnl, return_pct, profit_factor, expectancy, avg_win, avg_loss, payoff_ratio, max_drawdown_pct,
   max_drawdown_duration_days, sharpe, sortino, calmar, cagr, avg_holding_period, exposure_pct, turnover, and a
   flags list.
2. compute(trades, equity, bars_per_year, rf) orchestrating equity.py, ratios.py, drawdown.py, trade_stats.py.
   Annualisation must be derived from the timeframe (1s -> 22500*252, 1m -> 375*252, 1d -> 252) and echoed in
   the output metadata.
3. Degenerate-case policy implemented and tested: no trades -> ratios None with flags=["no_trades"]; no losing
   trades -> profit_factor None with flags=["no_losses"]; constant equity -> sharpe 0 with a flag; fewer than
   2 equity points -> sharpe None. JSON output must never contain NaN or Infinity.
4. Money rounded to paise, ratios to 4 decimals, in one serialisation helper.
5. docs/domain/metric-definitions.md stating the exact formula for every field (UI tooltips reuse it).

Constraints: no randomness, no dependence on wall clock, no DB/network, no imports outside numpy/pandas/stdlib.
Every metric needs a hand-computed fixture.

Done when: the 9-wins-of-Rs-10 plus 1-loss-of-Rs-500 case reports win_rate 90, net_pnl -410, profit_factor
0.18 and negative expectancy; rising-equity case has 0 drawdown with no NaN/Infinity in validated JSON;
zero-trade and no-loss cases follow the flag policy; Sharpe matches an independent implementation within 1e-6;
the 10 fixture goldens pass. Paste the fixture table.
```

## Notes / risks

- Sharpe on 1-second bars is famously misleading (microstructure noise). Report it but also compute a
  daily-resampled Sharpe and label both, or the leaderboard will rank HFT noise first.
- Keep `None` vs `0` distinction strict; the UI must render "n/a", never a fake zero.
