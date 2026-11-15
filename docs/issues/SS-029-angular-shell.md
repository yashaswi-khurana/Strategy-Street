# SS-029 — Angular Shell: Routing, Generated API Client, Resilient WS Service

| | |
| --- | --- |
| Milestone | 5 — Frontend |
| Depends on | SS-001 (API contract from SS-003 onward can be mocked) |
| Blocks | SS-030, SS-031, SS-032, SS-033 |
| Size | ~2 days |
| Stack | Angular 18 (standalone components), TypeScript, RxJS |

## Goal

One app shell every later UI issue plugs into: typed API access generated from OpenAPI, a self-healing WebSocket
service, shared state, and a layout that makes charts/strategy/dashboards feel like one product.

## Scope

**In scope**
- App structure: standalone components, lazy routes `/chart`, `/strategies`, `/strategies/:id`,
  `/backtests`, `/backtests/:runId`, `/paper`, `/analytics`, `/settings`.
- `core/api`: generated client from `contracts/openapi.yaml` (`openapi-generator` or `ng-openapi-gen`) with
  interceptors: base URL from environment, request-id propagation, error-toast mapping using the shared error
  envelope, 401 → auth redirect hook.
- `core/ws`: `MarketStreamService` wrapping the dashboard protocol from SS-012:
  - reconnect with exponential backoff + jitter, resume subscriptions after reconnect,
    `visibilitychange` pause/resume (stop streaming in a hidden tab), heartbeat watchdog,
    typed frame models + a `frame$` multiplexed by `stock`/`type`.
  - A `connectionState$` signal (`connecting|live|degraded|offline`) surfaced in the header.
- `core/state`: signal-based stores (`StockStore`, `TimeframeStore`, `UiStore`) — selection is URL-driven so a
  chart link is shareable.
- Layout: header (connection badge, market status from a `GET /healthz`-derived endpoint), side nav, toast host,
  loading skeletons, dark theme tokens, responsive down to 1280 px.
- Testing: Jest/Karma unit tests for services, one Playwright smoke test (app loads, WS status renders).

**Out of scope**
- Any charting (SS-030), editor (SS-031), dashboards (SS-032/033).
- Real auth UI (a dev token injector is enough; note the auth issue as follow-up).

## Small architecture

```text
app.config.ts ─ providers: HttpClient(withFetch), API_BASE_URL, interceptors, WsConfig
AppComponent ─ shell layout ─ router-outlet
  core/api/ApiService        (generated, typed)
  core/ws/MarketStreamService ── socket ── frame$ (typed union) ── components filter by type
  core/state/*Store          (signals, computed selectors)
```

Frame union (mirrors SS-012 exactly, generated-adjacent and unit-tested against sample payloads):

```ts
type Frame =
  | { type: 'SNAPSHOT'; tf: string; candles: Candle[]; seq: number; ts: string }
  | { type: 'CANDLE';   tf: string; candle: Candle;  seq: number; ts: string }
  | { type: 'TICK';     ltp: number; ts: string; seq: number }
  | { type: 'TRADE' | 'PAPER_STATS' | 'BACKTEST_PROGRESS' | 'INDICATOR' | 'ERROR'; ... };
```

* Subscription intent is declarative: components call `stream.watch('TCS','5m')`; the service diffs intents and
  sends `SUBSCRIBE_VIEW`/`UNSUBSCRIBE_VIEW`, so no component manages socket messages directly.
* Reconnect replays the current intent set; while `degraded`, a banner offers "reconnecting…" and the chart keeps
  the last snapshot (no blank screens).
* Frame `seq` gaps are detected per type; on gap > 0 the service requests a fresh `SNAPSHOT` (self-healing).
* Strict TypeScript (`strict`, `noUncheckedIndexedAccess`), `OnPush` everywhere, no `any` in app code.

## Tasks

- [ ] Generate the client + interceptors + environment config; wire `make web`.
- [ ] `MarketStreamService` with intent diffing, backoff, resume, visibility handling, seq-gap resync.
- [ ] Signal stores + URL-synced selection.
- [ ] Shell layout, theme tokens, toasts, skeletons, connection badge.
- [ ] Unit tests for the WS service (fake WebSocket), interceptor tests, Playwright smoke.
- [ ] ADR `docs/adr/0029-frontend-state.md` (signals vs NgRx, why intent-based WS).

## Acceptance criteria

1. Navigating to each lazy route loads its (placeholder) component with no console errors and < 2 s first paint
   locally.
2. `watch('TCS','5m')` then `watch('INFY','5m')` sends exactly one `UNSUBSCRIBE_VIEW` + one `SUBSCRIBE_VIEW`,
   deduplicating repeated calls for the same symbol.
3. Killing the WS triggers backoff reconnects and, on reconnect, re-sends the full intent set automatically
   (asserted against a fake socket).
4. A missing `seq` triggers a `SNAPSHOT` re-request; a `degraded` banner appears and clears.
5. Hiding the tab pauses the socket (no messages processed) and showing it resumes within 1 s.
6. An API 500 with the standard error envelope shows one toast with `code` + `message` and logs the `request_id`.
7. `tsc --noEmit`, lint and unit tests pass; the Playwright smoke test is green.

## Test plan

- Fake WebSocket harness driving every connection state transition.
- Interceptor unit tests for success/error/401 paths.
- Playwright smoke: load `/chart`, assert header badge and no errors.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §4.1 (Angular frontend responsibilities) and §9 (WS market-stream
protocol), docs/issues/SS-029-angular-shell.md, docs/contracts/market-stream.md (frame schemas from SS-012).
Repo: web/ is an Angular 18 workspace created in SS-001; contracts/openapi.yaml documents the REST API.

Implement SS-029 only: the application shell.
1. Structure: standalone components, lazy routes /chart, /strategies, /strategies/:id, /backtests,
   /backtests/:runId, /paper, /analytics, /settings with placeholder pages.
2. core/api: generate a typed client from contracts/openapi.yaml, add interceptors for base URL, request-id,
   error-envelope -> toast mapping, and 401 -> redirect. No `any` in app code, strict TS with
   noUncheckedIndexedAccess, OnPush everywhere.
3. core/ws MarketStreamService: declarative watch(stock, tf) intents diffed into SUBSCRIBE_VIEW/UNSUBSCRIBE_VIEW
   frames; exponential backoff with jitter; automatic re-subscription of the full intent set after reconnect;
   heartbeat watchdog; visibilitychange pause/resume; per-type seq gap detection that requests a fresh SNAPSHOT;
   connectionState$ signal (connecting|live|degraded|offline). Typed Frame union matching the SS-012 contract.
4. core/state: signal stores for stock/timeframe selection, URL-synced so links are shareable; UI store for
   toasts/loading.
5. Shell layout: header with connection badge, side nav, toast host, skeletons, dark theme tokens, responsive to
   1280px.
6. Tests: unit tests with a fake WebSocket covering all connection transitions, interceptor tests, and a
   Playwright smoke test asserting the app loads and the badge renders.

Constraints: no charting, no editor, no dashboards (later issues); no real auth UI (dev token injector only);
components must never touch the raw socket.

Done when: all routes load cleanly; watch('TCS') then watch('INFY') emits one unsubscribe and one subscribe with
dedupe; killing the socket reconnects and replays intents; a seq gap re-requests a snapshot; tab hide/show pauses
and resumes; a 500 shows one toast with request_id logged; tsc/lint/tests/Playwright pass.
```

## Notes / risks

- Generating the client from OpenAPI early prevents hand-written models drifting from the Go contract — keep it
  in a `make web-api` target that CI re-runs.
- Intent-based WS subscription is the single most important design choice here: without it every component will
  race the socket for subscriptions.
