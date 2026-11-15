# SS-033 — Paper Trading Dashboard: Blotter, Equity Curve, Live Win Rate, Kill Switch

| | |
| --- | --- |
| Milestone | 5 — Frontend |
| Depends on | SS-028, SS-030 |
| Blocks | — |
| Size | ~2.5 days |
| Stack | Angular 18, ECharts |

## Goal

Give paper trading an operational cockpit: deploy a strategy version, watch fills land in real time, see honest
PnL/win-rate numbers, and pull the plug in one click.

## Scope

**In scope**
- `features/paper`:
  - Deployments list: status (DRAFT/ACTIVE/PAUSED/KILLED), symbol, timeframe, version, capital, equity,
    total return, win rate, trades today, small sparkline of equity.
  - Deploy flow: pick strategy version + symbol + timeframe, capital, sizing mode/value, risk profile (or custom
    limits) → `POST /paper-trading/deploy` then `activate`; validation errors surfaced per field
    (capital floor, duplicate active deployment `409`).
  - Deployment detail: header with status + actions (activate/pause/resume/kill with confirmation), wallet panel
    (cash, equity, realized/unrealized, day PnL), live win-rate card, risk panel (limits, breaches today,
    cooldown) from `GET .../risk`, open positions table, blotter (trades with cursor pagination), equity curve and
    drawdown chart (`/equity-curve`), and a link to the chart with live markers.
  - Realtime: `PAPER_STATS` frames update wallet chips + win rate (throttled render, no layout thrash); `TRADE`
    frames prepend a blotter row with a brief highlight animation; a toast on `RiskBreach`.
  - Kill switch UX: destructive confirmation naming the consequence ("flatten positions, cannot resume"), then
    optimistic state update with rollback on error.
  - Empty/edge states: not-yet-warmed-up (waiting for bars), paused, killed, no trades yet (`n/a`, never 0%).
- Shared: reuse `MetricValue` pipe (SS-032) and chart adapter (SS-030) for the equity curve.

**Out of scope**
- New server logic (SS-025/026/027/028 already provide everything). Real-money trading.

## Small architecture

```text
PaperStore (signals per deployment)
 ├─ REST: summary, detail, trades (keyset), equity-curve, risk
 ├─ WS: PAPER_STATS (patch chips), TRADE (prepend blotter row), RiskBreach (toast)
 └─ actions: activate/pause/resume/kill -> optimistic patch + rollback + refetch

DeploymentDetailComponent
 ├─ WalletPanel, WinRateCard, RiskPanel, PositionsTable, BlotterTable, EquityChart
```

* WS updates mutate only the affected deployment's store slice; the blotter is prepend-only and capped in memory
  (page history via REST).
* Optimistic action pattern: patch → call → on error revert and toast the server message; the kill action is never
  optimistic (always waits for confirmation from the server).
* Win rate and PnL come from the server aggregate (SS-028) — the UI never recomputes stats from the visible blotter
  page (which would be wrong by construction).
* Day PnL resets visually at session rollover using the backend's IST day boundary, not the browser clock.

## Tasks

- [ ] `PaperStore` + realtime patch handling + REST pagination.
- [ ] Deployments list + deploy form with validation and 409 handling.
- [ ] Detail page: wallet, win rate, risk panel, positions, blotter, equity/drawdown chart.
- [ ] Action buttons with optimistic/confirmed semantics + toasts.
- [ ] Empty/edge states, sparkline, highlight animation for new trades.
- [ ] Component tests with fixtures; Playwright: deploy → receive a trade → pause → kill.

## Acceptance criteria

1. Deploying with ₹5 000 shows an inline error; with ₹1 00 000 the deployment appears as `DRAFT`, and activating
   flips it to `ACTIVE` without a page reload.
2. A `PAPER_STATS` frame updates equity/win-rate chips within 500 ms and does not re-render the blotter.
3. A `TRADE` frame prepends exactly one blotter row with a highlight; duplicate frames (same trade id) do not
   duplicate rows.
4. Win rate shows `n/a` before the first trade (not `0%`) and matches the server aggregate afterwards.
5. Kill asks for confirmation naming the consequence, then the status becomes `KILLED` and every action button is
   disabled; a server error leaves the previous status and shows a toast.
6. Pause stops new trades (no blotter growth) while positions and equity stay visible.
7. Equity + drawdown chart renders ≥ 1 000 points from `/equity-curve` and stays in sync with the selected
   timeframe.
8. The deploy form surfaces the duplicate-active `409` with an actionable message and a link to the existing
   deployment.

## Test plan

- Store tests with synthetic WS frames (ordering, duplicates, unknown deployment).
- Component fixture tests for each state (DRAFT/ACTIVE/PAUSED/KILLED/no-trades).
- Playwright end-to-end with a mocked backend stream.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.5 (win rate & ledger tracking), §3.7 (wallet, sizing, kill switch),
§9, docs/issues/SS-033-paper-dashboard.md. Repo: Angular 18 shell (SS-029) with MarketStreamService, MetricValue
pipe and chart adapter (SS-030/SS-032); backend provides POST /paper-trading/deploy,
POST /paper-trading/deployments/{id}/activate|pause|resume|kill, GET /paper-trading/summary, GET deployments/{id},
GET deployments/{id}/trades?cursor=, GET deployments/{id}/equity-curve?tf=, GET deployments/{id}/risk, and WS
frames PAPER_STATS, TRADE, RiskBreach.

Implement SS-033 only: features/paper.
1. Deployments list with status, symbol, timeframe, version, capital, equity, total return, win rate, trades today
   and an equity sparkline.
2. Deploy form: strategy version, symbol, timeframe, capital, sizing mode/value, risk profile or custom limits ->
   POST deploy then activate; per-field validation (capital floor) and a clear duplicate-active 409 message linking
   to the existing deployment.
3. Detail page: status header + activate/pause/resume/kill actions, wallet panel (cash, equity, realized/unrealized,
   day PnL), live win-rate card, risk panel (limits, today's breaches, cooldown), open positions, blotter with
   cursor pagination, equity + drawdown chart, and a link to /chart with live markers.
4. Realtime: PAPER_STATS patches chips within 500ms without re-rendering the blotter; TRADE prepends one
   highlighted row and deduplicates by trade id; RiskBreach shows a toast. Never recompute stats from the visible
   blotter page — always use the server aggregate.
5. Actions: optimistic patch + rollback + toast for activate/pause/resume; kill requires a destructive confirmation
   naming the consequence and is never optimistic (wait for the server), then disables all actions.
6. Edge states: warmup-pending, paused, killed, no trades (show n/a, never 0%), day-PnL reset at the backend's IST
   boundary (not the browser clock).

Constraints: no new server logic, no real-money trading, reuse the existing MetricValue pipe and chart adapter,
throttle realtime renders to animation frames.

Done when: deploy validation and activation work without reload; PAPER_STATS updates chips in <500ms; TRADE
prepends one row and dedupes; win rate is n/a before the first trade; kill confirms, disables actions, and rolls
back on error; pause freezes new trades while keeping positions visible; the equity chart renders 1000+ synced
points; the 409 path is actionable. Include Playwright evidence for deploy -> trade -> pause -> kill.
```

## Notes / risks

- Showing a win rate from a 3-trade sample invites overconfidence — display the sample size next to every win-rate
  number and suppress percentages until a configurable minimum (e.g. 20 trades).
- The dashboard must never imply real orders are being placed; a permanent "simulated" badge belongs in the header.
