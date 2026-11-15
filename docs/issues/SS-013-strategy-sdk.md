# SS-013 — Strategy SDK Contract, Template & Reference Strategies

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-001 |
| Blocks | SS-014, SS-016, SS-018, SS-031 |
| Size | ~1.5 days |
| Stack | Python 3.12, pandas, numpy |

## Goal

Define the single contract every user strategy implements, ship the template the "Add Strategy" editor
pre-populates, and provide reference strategies that later issues use as fixtures.

## Scope

**In scope**
- `engine/sdk/base_strategy.py`:
  - `class BaseStrategy` with: `params: dict`, `generate_signals(df) -> DataFrame` (vectorized mode),
    `on_bar(current_bar, history_window) -> Signal` (event-driven mode), optional
    `on_start/on_stop`, `plot_overlays`, `plot_subcharts`, `warmup_bars` property derived from params.
  - Input frame contract: `bucket_time, open, high, low, close, volume, oi` (sorted, IST-aware index or
    column — pick one and document).
  - Output contract: `signal ∈ {1, -1, 0}` and optional indicator columns; validation raises
    `StrategyContractError` with actionable messages.
- `engine/sdk/validator.py` — output validation used by both engine and worker (signal dtype, NaN handling,
  no mutation of input, column dtypes).
- `engine/sdk/template.py` — the exact template text from spec §6, importable as a string constant (so the
  API and the Monaco editor share one source).
- Reference strategies in `engine/sdk/examples/`: `ema_cross.py`, `rsi_mean_reversion.py`,
  `buy_and_hold.py` (used to sanity-check cost/metrics engines).
- `engine/sdk/__init__.py` re-exports; `docs/contracts/strategy-sdk.md` with the full contract + examples.

**Out of scope**
- Sandboxing (SS-015), linting (SS-014), running backtests (SS-018).

## Small architecture

```python
class BaseStrategy(ABC):
    params: dict[str, Any] = {}
    plot_overlays: list[str] = []
    plot_subcharts: list[str] = []

    def __init__(self, **overrides): self.params = {**self.params, **overrides}

    @abstractmethod
    def generate_signals(self, df: pd.DataFrame) -> pd.DataFrame: ...

    # optional event-driven hook; default implementation delegates to a cached vectorized result
    def on_bar(self, current_bar: pd.Series, history: pd.DataFrame) -> int: ...
```

* `Signal` is an `IntEnum` (`BUY=1, SELL=-1, HOLD=0`) so `on_bar` can return a typed value; `generate_signals`
  writes ints into `df["signal"]`.
* Validation rules (all raise `StrategyContractError`):
  required columns present, `bucket_time` strictly increasing, `signal` in `{−1,0,1}`, no `inf`,
  `plot_overlays`/`plot_subcharts` columns actually exist in the returned frame.
* Determinism: the SDK forbids depending on `len(df)` (full history length) — expose `df` slices only, and
  document that vectorized strategies must be causal (that's SS-014's job to enforce).
* The template constant is imported by the Go API (`contracts/strategy_template.py` mirrored or served by an
  endpoint) so the editor and the backend can never disagree.

## Tasks

- [ ] `BaseStrategy`, `Signal`, `StrategyContractError`, validation helpers.
- [ ] Template constant identical to spec §6 + a test asserting the spec's example runs end-to-end on a
      synthetic frame.
- [ ] Three reference strategies with unit tests on fixed OHLCV fixtures (asserted signal positions).
- [ ] `warmup_bars` derivation from params (e.g. slow EMA length) used by backtest loaders.
- [ ] Contract doc with a table of every fields' types and semantics.

## Acceptance criteria

1. The spec's `UserStrategy` template runs on a 500-row synthetic frame and returns `signal` with at least one
   `1` and one `-1`.
2. A strategy returning `signal` as floats `[1.0, 0.0]` raises `StrategyContractError` naming the column and
   dtype.
3. Missing `close`, unsorted `bucket_time`, a NaN in `close`, and a `plot_overlays` name that doesn't exist
   each produce a distinct, actionable error message.
4. `buy_and_hold` returns `1` on the first tradeable bar and `0` afterwards.
5. `warmup_bars` for `ema_cross(ema_slow=21, rsi_period=14)` = 21.

## Test plan

- `pytest` with a fixture generator (`engine/tests/fixtures/ohlcv.csv`).
- Parametrised negative tests for every validation branch.
- Determinism test: same input → identical output twice.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §6 (Standardized Python Strategy SDK Contract + default template),
docs/issues/SS-013-strategy-sdk.md. Repo has engine/ (Python 3.12, pytest, ruff, mypy) from SS-001.

Implement SS-013 only: the strategy SDK in engine/sdk.
1. base_strategy.py: BaseStrategy with params dict + override in __init__, abstract generate_signals(df) ->
   DataFrame, optional on_bar(current_bar, history) -> Signal, plot_overlays/plot_subcharts lists, on_start/
   on_stop hooks, and a warmup_bars property derived from params. Provide a Signal IntEnum (BUY=1, SELL=-1,
   HOLD=0) and StrategyContractError.
2. Input/output contract: input columns bucket_time, open, high, low, close, volume, oi sorted ascending with
   IST-aware timestamps; output must keep those columns and add `signal` in {-1,0,1} plus any indicator columns
   named in plot_overlays/plot_subcharts. Validate: required columns, strictly increasing bucket_time, signal
   dtype/values, no inf, declared plot columns actually present, input frame not mutated. Each violation raises
   StrategyContractError with an actionable message naming the offending column/value.
3. template.py exporting the exact template from spec §6 as a string constant so the Angular Monaco editor and
   the API share one source of truth.
4. engine/sdk/examples/: ema_cross.py, rsi_mean_reversion.py, buy_and_hold.py, each tested against a committed
   OHLCV fixture.
5. docs/contracts/strategy-sdk.md documenting every field's type and semantics, including the causality rule
   and how warmup_bars is used to slice history.

Constraints: no sandboxing, no linting, no backtest execution in this issue; numpy/pandas only (pandas_ta may
be used in examples); no network or filesystem access in SDK code.

Done when: the spec template runs end-to-end on a 500-row synthetic frame producing both buy and sell signals;
negative tests produce distinct actionable errors for float signals, missing close, unsorted bucket_time, NaN
close and an undeclared overlay; buy_and_hold behaves as specified; warmup_bars(ema_slow=21, rsi_period=14)=21;
ruff/mypy/pytest clean.
```

## Notes / risks

- The template in the spec imports `pandas_ta` while the sandbox whitelist in §3.1 mentions `ta`. Pick one
  (recommend `pandas_ta`) and document it in the whitelist ADR (SS-014) so accepted code always runs.
- `on_bar` defaulting to vectorized results would undermine point-in-time semantics; instead make it raise
  `NotImplementedError` so strategies opt in explicitly (SS-019 consumes it).
