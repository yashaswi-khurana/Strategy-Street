# SS-010 — Upstox Market Feed V3 Protobuf WebSocket Client

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-007 |
| Blocks | SS-011 |
| Size | ~2 days |
| Stack | Go, Protobuf, `nhooyr/websocket` or `gorilla/websocket` |

## Goal

Maintain one resilient, authorised WebSocket connection to Upstox Market Data Feed V3 that decodes binary
Protobuf frames into typed tick events — the only source of real-time data in the platform.

## Scope

**In scope**
- `internal/upstox/feed`:
  - `Connect(ctx) error` — requests the feed URL (`GET /v3/feed/market-data-feed/authorize`), dials the
    returned `wss://` URL with the `Authorization: Bearer` header, reads binary frames.
  - Generated Protobuf types (checked in) for `FeedResponse`, `Feed`, `LTPC`, `OHLC`, `MarketFullFeed`,
    `IndexFullFeed`, `MarketOHLC`, `OptionGreeks`.
  - `Subscribe(ctx, keys []string, mode Mode)` / `Unsubscribe(ctx, keys []string)` where
    `Mode = ltpc | full | option_greeks`; sends JSON control frames on the same socket.
  - Decoding loop → `chan Tick` (`{InstrumentKey, LTP, CP, Vol, OI, TS, TotalVolume}`).
  - Reconnect: exponential backoff with jitter (1 s → 30 s), automatic re-subscribe of the **current**
    subscription set, and a `Connected bool` + `Reconnects` metric.
  - Frame-level validation: skip unknown feed types, tolerate partial/degraded frames, expose
    `MalformedFrames` counter.
- Latency instrumentation: `tick_receive_lag_ms` histogram using the vendor's `ltt` timestamp.

**Out of scope**
- Deciding which instruments to subscribe to (SS-012).
- Aggregating ticks into candles (SS-011).
- Order/portfolio updates feeds.

## Small architecture

```text
feed.Client
 ├─ auth: GET /v3/feed/market-data-feed/authorize  → { "data": { "authorized_redirect_uri": "wss://…" } }
 ├─ dial(redirect_uri, header{Authorization: Bearer <token>})
 ├─ reader goroutine: ReadMessage(Binary) → proto.Unmarshal(FeedResponse)
 │      for each feedType feeds/…: decode(ltpc|full) → ticks <- Tick
 ├─ writer goroutine: serialized JSON control frames {guid,method,data} (subscribe/unsubscribe/change_mode)
 └─ supervisor: watchdog on pong/deadline → close → backoff → re-dial → replay subscription set
```

* Only **one** connection per process, owned by `internal/streamer`; `feed.Client` exposes
  `Subscribe`/`Unsubscribe` that are internally serialised through a single writer goroutine (no concurrent
  writes to the socket).
* Subscription state is kept in the client itself (`map[string]Mode`) because reconnect must replay it — this
  is the client's only piece of state, guarded by a mutex, and it is queried (not duplicated) by SS-012.
* `proto` regeneration is scripted: `make proto` runs `protoc --go_out=… market_data.proto`; generated files
  are committed so builds need no protoc.
* Vendor sends a `{"type":"heartbeat"}` text frame — must not be treated as an error.

## Tasks

- [ ] Vendor `market_data.proto` checked in + `make proto` + generated Go.
- [ ] Authorize call + dial + read loop + decode into `Tick`.
- [ ] Serialised control-frame writer with `guid` correlation and subscribe/unsubscribe/mode-change.
- [ ] Reconnect supervisor with backoff, subscription replay, and a hard `ctx` cancellation path.
- [ ] Unit tests with a fake WS server (golang.org/x/net/websocket or `httptest` upgraded) sending real frame
      fixtures (checked-in `.bin` captures).
- [ ] Metrics: `feed_connected`, `feed_reconnects_total`, `feed_ticks_total{mode}`, `feed_malformed_total`,
      `tick_receive_lag_ms`.

## Acceptance criteria

1. Against a fake server that sends 3 captured binary frames, the client emits the expected 3 `Tick`s with
   correct LTP/volume/OI and instrument keys.
2. Killing the socket triggers reconnect with backoff and an automatic re-subscribe to the previously
   subscribed keys (asserted in the fake server's control-frame log).
3. A malformed frame increments `feed_malformed_total` and does not kill the reader.
4. Calling `Subscribe` from 50 goroutines concurrently never produces interleaved/partial JSON on the wire
   (fake server asserts each frame parses).
5. `ctx` cancellation closes the socket within 1 s and no goroutine leaks (`goleak`).

## Test plan

- Fake WS server + recorded vendor frames; golden test for decode.
- Reconnect storm test (server closes 10× in a row) → backoff grows, no duplicated subscribers.
- `go test -race -run . ./internal/upstox/feed`.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.9 and §4.1 (Upstox Market Data Feed V3 over Protobuf WebSocket,
must not consume the 2 RPS REST budget),
docs/issues/SS-010-upstox-ws-feed.md. Repo: Go 1.23, internal/upstox/auth gives TokenSource.Token(ctx).

Implement SS-010 only: internal/upstox/feed, a single resilient WebSocket client for Upstox Market Data Feed V3.
Requirements:
1. Vendor the market_data.proto (FeedResponse/Feed/LTPC/OHLC/MarketFullFeed/MarketOHLC) into internal/upstox/
   feed/proto, commit generated Go, and provide `make proto`.
2. Connect: GET https://api.upstox.com/v3/feed/market-data-feed/authorize with the bearer token, dial the
   returned wss:// URL with the Authorization header, and read binary frames.
3. Decode frames into a typed Tick{InstrumentKey, LTP, CP, Volume, OI, TotalVolume, ExchangeTS, RecvTS} sent on
   a channel; tolerate unknown feed types; count malformed frames instead of dying.
4. Subscribe/Unsubscribe/ChangeMode via JSON control frames serialised through ONE writer goroutine with guid
   correlation; keep the current subscription map internally so reconnect can replay it.
5. Supervisor: deadline/pong watchdog, exponential backoff with jitter (1s..30s), automatic re-dial and
   re-subscribe, Connected state, and clean shutdown on ctx cancel with no goroutine leaks.
6. Metrics: feed_connected, feed_reconnects_total, feed_ticks_total{mode}, feed_malformed_total, and a
   tick_receive_lag_ms histogram from the vendor timestamp.

Constraints: exactly one connection per process; never write to the socket from more than one goroutine;
must not touch REST candle endpoints; no DB writes in this issue.

Done when: captured binary frame fixtures decode to the expected ticks; killing the socket triggers backoff
reconnect plus automatic resubscribe (shown in the fake server's control-frame log); malformed frames are
counted and non-fatal; 50 concurrent Subscribe calls produce only well-formed JSON frames; ctx cancel closes
in <1s with goleak clean; -race passes.
```

## Notes / risks

- Market Feed V3 is only usable during market hours; tests must never depend on live market timing.
- Vendor may send `market_info`/index frames the platform doesn't need — filter by instrument key against
  SS-012's subscription set rather than decoding everything into memory.
