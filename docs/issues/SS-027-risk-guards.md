# SS-027 — Risk Guards: Stop-Loss/Trailing, Exposure Caps & Daily Max-Loss Kill Switch

| | |
| --- | --- |
| Milestone | 4 — Paper trading |
| Depends on | SS-026 |
| Blocks | SS-028, SS-033 |
| Size | ~2 days |
| Stack | Go, PostgreSQL |

## Goal

Stop a paper deployment from destroying itself (or the platform's credibility) when a strategy goes wrong:
per-trade stops, exposure limits, and an automatic daily kill switch that pauses trading.

## Scope

**In scope**
- `internal/risk` with a `PreTradeCheck` + `OnFill`/`OnBar` hook set consumed by SS-026:
  - `max_position_value_pct` (per symbol, % of equity), `max_open_positions`,
    `max_daily_loss_pct`, `max_daily_loss_abs`, `max_trades_per_day`,
    `cooldown_after_consecutive_losses`.
  - Stop-loss / take-profit / trailing stop per position: `SL{pct|abs}`, `TP{pct|abs}`,
    `trailing{pct|abs, activation}` evaluated on each bar with documented intrabar priority
    (SL before TP when both are crossed in one bar — conservative).
  - **Daily max-loss kill switch**: cumulative day PnL (realised + unrealised marked at bar close) breaches
    threshold → flatten positions, block new entries, transition deployment `ACTIVE → PAUSED` with
    `kill_reason=DAILY_MAX_LOSS`, emit `RiskBreach`. Reset at the next trading session (IST), never mid-day.
  - Consecutive-loss cooldown: N losses → block entries for M bars, recorded in the ledger.
- Table `risk_events` (deployment_id, rule, severity, context JSONB, bar_ts, created_at) — every block is
  auditable, not silent.
- `risk_config_json` schema (validated on deploy) + defaults per risk profile (`conservative|balanced|aggressive`).
- Endpoint `GET /api/v1/paper-trading/deployments/{id}/risk` → current limits, breaches today, cooldown state.

**Out of scope**
- Live order routing to the real market, margin/leverage modelling.
- UI (SS-033) beyond the data it needs.

## Small architecture

```text
OnBarClosed(deployment, bar)
  ├─ mark-to-market open positions ─► day_pnl
  ├─ StopEvaluator: SL / TP / trailing (priority SL > TP, trailing updates high-water mark)
  ├─ DailyLossGuard: day_pnl <= -max_daily_loss → flatten + PAUSE + RiskBreach
  └─ CooldownGuard: consecutive losses >= N → block entries for M bars

PreTradeCheck(deployment, order)
  ├─ reject if paused/killed, outside session, cooldown active, trades_today >= cap
  ├─ reject if position value > pct of equity, or open positions >= max
  └─ resize qty down to the largest allowed (never silently ignore a cap)
```

* All guards are **pure functions of state** (`RiskState{day_pnl, open_positions, trades_today,
  consecutive_losses, high_water_marks}`) so they are table-testable and deterministic — same reason SS-026 is
  serialised.
* Day boundary uses `marketcal.TradingDays` in IST; a position carried overnight resets only the *daily* counters,
  not the stops.
* Resize-vs-reject is explicit per rule (`max_position_value_pct` resizes, `max_open_positions` rejects) and both
  outcomes are recorded in `risk_events`.
* Kill switch flattens via MARKET orders through SS-026's fill path so all costs are still charged — a kill is a
  real trade, not a bookkeeping erase.

## Tasks

- [ ] `risk_config` schema + profiles + validation on deploy.
- [ ] `RiskState` derivation from the ledger (single source of truth, no duplicate counters).
- [ ] Stop/TP/trailing evaluator with intrabar priority rules.
- [ ] Daily loss guard + flatten + PAUSE transition + session reset.
- [ ] Pre-trade checks with resize/reject decisions + `risk_events` writes.
- [ ] Endpoint + OpenAPI; unit tests per rule; integration test through SS-026 with a losing fixture.
- [ ] ADR `docs/adr/0027-risk-priority.md` (rule order, resize vs reject, intrabar ambiguity).

## Acceptance criteria

1. With a 3% daily max loss on ₹1 00 000, a day reaching −₹3 100 flattens positions, pauses the deployment with
   `kill_reason=DAILY_MAX_LOSS`, and blocks the next day's entries only after the session reset (not before).
2. Kill-switch flattening charges costs (assert charges > 0 on the closing trades).
3. `max_open_positions=2`: the third entry is rejected with `risk_events.rule=max_open_positions`; the first two
   are untouched.
4. `max_position_value_pct=20`: a signal wanting 40% of equity is **resized** to 20% (qty adjusted, reason
   logged), not rejected.
5. Trailing stop on a long with 5% trail: after a high-water mark of ₹120 the stop sits at ₹114 and a bar with
   low ₹113 exits at the documented price.
6. Both SL and TP crossed in one bar → SL wins (conservative), asserted explicitly.
7. Cooldown: 3 consecutive losses block entries for the next 10 bars, then re-enable automatically.
8. Every block/resize/kill produces exactly one `risk_events` row with context.

## Test plan

- Table-driven tests per rule with a synthetic `RiskState`.
- End-to-end test: SS-026 + SS-027 replaying a fixture designed to lose 4% in a day.
- Determinism: identical fixture → identical risk events.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.7 (order types, risk guards, daily max-loss kill switch),
docs/issues/SS-027-risk-guards.md. Repo: Go; SS-026 paper engine exposes PreTradeCheck/OnBar/OnFill hooks and
a serialised per-deployment worker; SS-025 owns deployment states (ACTIVE/PAUSED/KILLED) and wallets;
SS-016 supplies costs for flattening; marketcal (SS-005) supplies session/day boundaries.

Implement SS-027 only: internal/risk.
1. RiskConfig (validated JSON on deploy) with max_position_value_pct, max_open_positions, max_daily_loss_pct,
   max_daily_loss_abs, max_trades_per_day, cooldown_after_consecutive_losses, and per-position SL/TP/trailing
   (pct or absolute, trailing with activation threshold). Ship conservative/balanced/aggressive profiles.
2. Derive RiskState (day_pnl, open_positions, trades_today, consecutive_losses, high_water_marks) from the
   ledger — never maintain duplicate counters.
3. OnBarClosed hooks: mark-to-market, then stop evaluation with documented intrabar priority (SL before TP;
   trailing updates the high-water mark on the bar's favourable extreme), then the daily loss guard, then the
   cooldown guard.
4. Daily max-loss kill switch: when day PnL breaches the threshold, flatten open positions with MARKET orders
   through the normal fill path (costs charged), transition ACTIVE->PAUSED with kill_reason=DAILY_MAX_LOSS, emit
   RiskBreach, and reset daily counters only at the next IST trading session.
5. PreTradeCheck: reject when paused/killed, outside session, in cooldown, or at max_open_positions /
   max_trades_per_day; RESIZE (not reject) when the order value exceeds max_position_value_pct of equity —
   resized quantity must be the largest allowed. Record every block/resize/kill in a risk_events table with
   rule, severity and context JSONB.
6. GET /api/v1/paper-trading/deployments/{id}/risk returning limits, today's breaches and cooldown state.
   Update contracts/openapi.yaml and write ADR 0027 covering rule ordering and resize-vs-reject semantics.

Constraints: guards must be pure functions of state (deterministic, table-testable); no margin/leverage; no UI;
no silent blocks — every decision is an audit row.

Done when: the 3% daily-loss fixture flattens, pauses with the right kill_reason and charges costs; entries stay
blocked until the next session; third position is rejected while the first two survive; a 40%-of-equity order is
resized to 20%; trailing stop math (120 high, 5% trail -> stop 114) exits at the documented price when low is
113; SL wins when both SL and TP are crossed; cooldown enables after 10 bars; one risk_events row per decision.
```

## Notes / risks

- Intrabar OHLC cannot tell you whether SL or TP hit first; the conservative choice (SL) must be visible in the
  UI or users will report "impossible" losses.
- Flattening on the kill switch with MARKET orders during a halt/illiquid bar can produce absurd fills — cap
  slippage and log a `flatten_degraded` event when participation caps prevent full exit.
