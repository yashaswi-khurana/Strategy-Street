# SS-016 — Indian Brokerage, Tax & Slippage Engine

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-013 |
| Blocks | SS-017, SS-018, SS-026 |
| Size | ~2 days |
| Stack | Python, `decimal` |

## Goal

Every simulated fill is charged realistic Indian statutory costs plus configurable slippage and volume caps,
so high-frequency strategies cannot look profitable on frictionless prices.

## Scope

**In scope**
- `engine/costs/india.py`:
  - `Charges` dataclass: brokerage, STT, exchange txn, SEBI, stamp duty, GST — each with the rate used.
  - `calculate(side, segment, order_value, qty, price) -> Charges` and `total()`.
  - Correct asymmetry: STT/brokerage on both legs for intraday equity, delivery STT only on sell, stamp duty
    only on buy, GST only on brokerage+txn+SEBI.
  - Config-driven rates in `engine/costs/config.yaml` (dated, "as of" field) — never hardcoded in logic.
- `engine/costs/slippage.py`:
  - Models: `fixed_ticks(n)`, `percent(p)`, `volume_participation(cap_pct)`; compose additively.
  - `fill_price(side, ref_price, tick_size, model) -> price` (buy pays up, sell receives less).
- `engine/costs/participation.py`: `max_fillable(bar_volume, cap_pct)` → deterministic integer qty, so a
  200-volume 1s bar can't fill 10 000 shares.
- Round-trip helper `round_trip_pnl(entry, exit, qty, segment, config) -> NetPnl{gross, charges, net}`.
- `docs/domain/indian-costs.md` with a worked example table the tests assert against.

**Out of scope**
- Order matching/fills for paper trading (SS-026). Metrics (SS-017).

## Small architecture

```python
@dataclass(frozen=True)
class Charges:
    brokerage: Decimal; stt: Decimal; exchange: Decimal
    sebi: Decimal; stamp: Decimal; gst: Decimal
    def total(self) -> Decimal: ...

def charges_for(side: Side, segment: Segment, turnover: Decimal, cfg: CostConfig) -> Charges
```

Default rates (config, "as of 2026-01-01", equity intraday):
| Component | Rate |
| --- | --- |
| Brokerage | min(0.03% of turnover, ₹20) per executed order |
| STT | 0.025% on **sell** turnover (intraday), 0.1% both legs (delivery) |
| Exchange txn | 0.00297% (NSE) |
| SEBI | ₹10 per crore |
| Stamp duty | 0.003% on **buy** only |
| GST | 18% on (brokerage + exchange + SEBI) |

* `Decimal` everywhere at the boundary; convert to float only for vectorised math, and round to paise with
  `ROUND_HALF_UP` in exactly one helper (`money()`).
* Slippage is applied to the **reference price** before charges, and the participation cap reduces quantity
  *before* both — order of operations is part of the contract and tested.
* All functions are pure and deterministic; a golden table of 12 worked examples (buy/sell × intraday/delivery ×
  big/small order) pins behaviour.

## Tasks

- [ ] `CostConfig` loader (YAML/JSON, validated, versioned with an "effective_from" date).
- [ ] `charges_for` with segment/side asymmetry + `money()` rounding helper.
- [ ] Slippage models + `fill_price` composition; tick-size aware rounding to valid tick.
- [ ] Participation cap + integer qty.
- [ ] `round_trip_pnl` + golden table tests.
- [ ] `docs/domain/indian-costs.md` + ADR `docs/adr/0016-cost-model-assumptions.md` (rounding, per-order vs
      per-trade brokerage, GST base).

## Acceptance criteria

1. A ₹1,00,000 intraday buy then sell with default config deducts exactly the documented amount (asserted to
   the paisa against the hand-computed golden table) and net = gross − charges.
2. Brokerage is capped at ₹20 per order: a ₹10,00,000 order still pays ₹20.
3. Stamp duty is charged only on buy; STT for intraday only on sell; delivery STT on both legs.
4. `fill_price(BUY, 100.00, tick=0.05, fixed_ticks(2)) == 100.10`; for SELL `99.90`.
5. With `volume_participation(0.10)` and bar volume 200, max fillable qty is 20 (integer, deterministic).
6. Repeating any calculation yields identical `Decimal` values (no float drift).

## Test plan

- Golden table parametrised tests (12+ cases) with expected values to the paisa.
- Property test: charges are monotonically increasing in turnover until the brokerage cap.
- Cross-check against a published broker calculator spreadsheet for one real trade.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.2 (Indian statutory cost calculator + slippage/volume
participation), docs/issues/SS-016-cost-engine.md. Repo has Python engine/ package with pytest/ruff/mypy.

Implement SS-016 only: engine/costs/{india.py,slippage.py,participation.py,config.yaml}.
1. Config-driven rates with an effective date and full YAML schema: brokerage min(0.03%, Rs 20) per order,
   STT (0.025% sell side intraday; 0.1% both legs delivery), exchange transaction charges 0.00297% NSE, SEBI
   Rs 10/crore, stamp duty 0.003% buy side only, 18% GST on (brokerage + exchange + SEBI).
2. charges_for(side, segment, turnover, cfg) -> Charges dataclass with each component; total(); all arithmetic
   in Decimal with a single money() rounding helper (ROUND_HALF_UP to paise). Explain the GST base and
   per-order brokerage in ADR 0016.
3. Slippage models: fixed_ticks(n), percent(p), volume_participation(cap_pct), composable; fill_price(side,
   ref_price, tick_size, model) rounds to a valid tick and applies adverse direction (buy pays up, sell
   receives less). Order of operations is fixed and documented: participation cap on qty -> slippage price ->
   charges.
4. round_trip_pnl(entry, exit, qty, segment, cfg) -> {gross, charges, net}.
5. docs/domain/indian-costs.md with a worked example table; tests must assert those exact numbers to the paisa.

Constraints: no randomness, no floats at the money boundary, no imports outside stdlib (decimal/dataclasses/
yaml), no dependency on the SDK or the sandbox.

Done when: the 12-case golden table (buy/sell x intraday/delivery x large/small) matches to the paisa;
brokerage caps at Rs 20 on a Rs 10,00,000 order; stamp duty buy-only and STT asymmetry proven; fill_price
examples give 100.10 for BUY and 99.90 for SELL with 2 ticks at 0.05; participation 10% of 200 volume yields
qty 20; repeated runs produce identical Decimals. Show the golden table in the PR.
```

## Notes / risks

- Statutory rates change (budget day). Keeping them in dated YAML means a backtest can use the rates that
  applied to its period — document that backtests default to the *current* config and make it overridable.
- Do not model brokerage as a percentage of turnover without the cap; that alone can flip a strategy's sign.
