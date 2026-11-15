# SS-012 — Subscription Manager (Active-View) + Internal Market-Stream WS Hub

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-010, SS-011 |
| Blocks | SS-026, SS-030 |
| Size | ~2.5 days |
| Stack | Go, WebSocket |

## Goal

The platform must stream only what is currently being watched or traded. This issue builds the refcounted
subscription brain and the client-facing WebSocket hub that the Angular app talks to.

## Scope

**In scope**
- `internal/marketdata/submgr`:
  - `Acquire(source Source, stockID int32)` / `Release(source Source, stockID int32)` where
    `Source = {Kind: view|paper, ID: string}`.
  - Refs counted **per source**, so a chart closing does not unsubscribe a symbol used by a paper deployment.
  - Emits diffs to the feed client: `Subscribe(keys)` for newly needed, `Unsubscribe(keys)` for fully idle —
    with a 30 s linger before unsubscribing (prevents churn when a user flips between tabs).
  - Idle sweep job + `Snapshot()` for `/admin` diagnostics.
- `internal/httpapi/ws` (endpoint `/ws/market-stream`):
  - Protocol: client → server `SUBSCRIBE_VIEW {stock, tf, strategy_id?}`, `UNSUBSCRIBE_VIEW {stock}`,
    `PING`; server → client `SNAPSHOT {tf, candles[]}`, `CANDLE {tf, candle}`, `TICK {ltp, ts}`,
    `INDICATOR {name, ts, value}`, `TRADE {…}`, `BACKFILL_PROGRESS {…}`, `ERROR {code, message}`.
  - Per-connection room set; fan-out from the event bus; slow-consumer handling (drop `TICK`, never drop
    `CANDLE`/`TRADE`; disconnect after N consecutive overflows with a reason).
  - Auth hook (token in query/header) + connection limits per user; ping/pong keepalive.
- On subscribe: immediately send `SNAPSHOT` from the ring buffer (SS-011) then live deltas — so the chart never
  races the stream.

**Out of scope**
- Chart rendering (SS-030). Paper-trade event production (SS-026) — this issue only defines the channel.
- Multi-node fan-out via Redis pub/sub (note as future work; single-node hub here).

## Small architecture

```text
Angular ──WS──► hub.Conn (room set)
                 │  SUBSCRIBE_VIEW {stock, tf}
                 ├─► submgr.Acquire(view:<connID>, stock)  ──► feed.Subscribe (diff only)
                 ├─► ring.Snapshot(stock, tf) ──► SNAPSHOT frame
                 └─► room[stock] += conn

eventbus (BarClosed / Tick / Trade) ──► hub.dispatch ──► for each conn in room[stock]: push frame
```

```go
type Hub interface {
    HandleConn(ctx context.Context, w http.ResponseWriter, r *http.Request)
    Broadcast(stockID int32, msg Frame)      // used by paper engine for TRADE frames
    Stats() HubStats
}
```

* Rooms are keyed by `stockID`; the timeframe is a client-side view over the same tick stream, so one
  subscription serves every timeframe the client shows (fewer vendor keys, fewer DB reads).
* `Frame` is a typed struct with a `type` discriminator; all frames carry `seq` (monotonic per connection) and
  `ts` so the client can detect gaps.
* Backpressure: per-connection buffered channel (256). `TICK` frames are coalesced (keep latest per stock);
  `CANDLE`/`TRADE`/`SNAPSHOT` are never dropped.
* Lifecycle: `Release` on disconnect/Ctrl-C in a `defer`, so a crashed client cannot pin a subscription.

## Tasks

- [ ] `submgr` with per-source refcounts, diff computation, linger, idle sweep, snapshot.
- [ ] WS upgrade handler, frame codec, room registry, ping/pong, auth hook, connection limits.
- [ ] Snapshot-then-stream ordering guarantee (with a `seq` monotonicity test).
- [ ] Slow-consumer policy + `ws_dropped_total{type}` metric + structured disconnect reasons.
- [ ] Unit tests with a real `httptest` WS server and multiple synthetic clients.
- [ ] Contract doc `docs/contracts/market-stream.md` (frame schemas, example payloads).

## Acceptance criteria

1. Two clients subscribing to TCS cause exactly **one** vendor subscribe for TCS; closing one keeps it
   subscribed; closing both unsubscribes after the linger period.
2. A paper deployment holding TCS keeps it subscribed even when every chart closes.
3. A new subscriber receives `SNAPSHOT` (ring buffer contents, ascending `bucket_time`) before any live
   `CANDLE` for that symbol — asserted via `seq` ordering.
4. A client that stops reading while 10 000 `TICK` frames are broadcast receives coalesced ticks and is
   disconnected with `reason=slow_consumer`; `CANDLE` frames were never dropped for healthy clients.
5. Disconnecting abruptly releases all refs (assert `Snapshot()` empty, `goleak` clean).

## Test plan

- Hub tests with 3 synthetic clients, different stocks/tfs.
- Refcount matrix test (view+paper, multiple views, linger timing with a fake clock).
- Load smoke: 50 connections × 1 000 frames, assert p99 fan-out latency < 5 ms locally.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §7 (SUBSCRIBE_VIEW event, active-view switching) and §9
(WS /ws/market-stream), docs/issues/SS-012-subscriptions-and-hub.md. Repo: Go 1.23 + chi; feed.Client from
SS-010 exposes Subscribe/Unsubscribe(keys, mode) and maintains its own subscription map; SS-011 provides a
per-(stock,tf) ring buffer Snapshot and an event bus of BarClosed/Tick events.

Implement SS-012 only: internal/marketdata/submgr and the /ws/market-stream hub.
1. submgr: Acquire(source, stockID)/Release(source, stockID) with refcounts per source kind (view|paper),
   diffing into feed.Subscribe/Unsubscribe calls, a 30s linger before unsubscribing idle symbols, an idle sweep
   job, and Snapshot() for diagnostics. A chart closing must never unsubscribe a symbol used by a paper
   deployment.
2. WS hub: upgrade /ws/market-stream, parse SUBSCRIBE_VIEW {stock, tf, strategy_id?} / UNSUBSCRIBE_VIEW /
   PING, and emit typed frames SNAPSHOT, CANDLE, TICK, INDICATOR, TRADE, BACKFILL_PROGRESS, ERROR — each with
   a monotonic per-connection seq and ts. On subscribe, send SNAPSHOT from the ring buffer before any live
   CANDLE for that symbol. Provide Broadcast(stockID, Frame) for the paper engine and Stats().
3. Backpressure: 256-deep per-connection queue, coalesce TICK (keep latest per stock), never drop
   CANDLE/TRADE/SNAPSHOT; disconnect slow consumers with reason=slow_consumer and a metric.
4. Auth hook (bearer token or query token) and per-user connection limits; ping/pong keepalive.
5. Always Release refs on disconnect (defer) so a crashed client cannot pin a vendor subscription.

Constraints: single-node hub (no Redis fan-out yet — document as future work); no strategy evaluation; no DB
writes; frames documented in docs/contracts/market-stream.md with example payloads.

Done when: two clients on TCS produce one vendor subscribe; view+paper refcounts behave per the matrix test;
SNAPSHOT always precedes live CANDLE (seq proof); slow consumer is coalesced-then-disconnected while healthy
clients lose nothing; abrupt disconnect leaves no refs and goleak is clean; -race passes.
```

## Notes / risks

- Sending a full snapshot on every timeframe switch will be wasteful; cap `SNAPSHOT` to the requested window
  and rely on SS-009 backfill for deeper history.
- Multi-node deployment will need Redis pub/sub for `TRADE`/`CANDLE` fan-out — keep `Broadcast` behind an
  interface to allow that swap.
