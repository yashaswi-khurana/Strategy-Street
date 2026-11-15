# SS-008 — Upstox Historical REST Client + 2 RPS Token Bucket + Priority Queue

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-007 |
| Blocks | SS-009, SS-022 |
| Size | ~2 days |
| Stack | Go, `golang.org/x/time/rate` |

## Goal

All historical-candle traffic funnels through one client that can never exceed **2 requests/second**, with a
priority order (UI backfill > paper warmup > background sync) and clean retry/limit accounting.

## Scope

**In scope**
- `internal/upstox/historical`:
  - Typed request: `Fetch(ctx, instrumentKey, tf, from, to, priority)`.
  - Endpoint: `GET /v2/historical-candle/{instrument_key}/{interval}/{unit}/{to_date}/{from_date}` (and the
    intraday variant `…/intraday/…` for 1-minute today).
  - Per-timeframe max window table (e.g. 1m ≤ 1 month, 1h ≤ 3 months, 1d ≤ 1 year per request) and automatic
    windowing into multiple calls.
  - Response → `[]candles.Candle` normalisation (Upstox returns `[ts, o, h, l, c, v, oi]` arrays, oldest-first).
- Global limiter `rate.NewLimiter(rate.Limit(2), 1)` shared by **all** REST callers — enforced by a single
  `restClient` instance, plus a 3-goroutine priority scheduler (weighted queue).
- Retry policy: 429/5xx → exponential backoff (cap 30 s) with jitter; 401 → bubble `ErrUnauthorized`
  (never retry); max 3 attempts; each attempt consumes a token.
- Metrics/logs: `upstox_requests_total{priority,outcome}`, queue depth, wait duration.

**Out of scope**
- Deciding *what* to backfill (SS-009). Writing to the DB (SS-003/009).

## Small architecture

```go
type Priority int
const (P1UI Priority = iota; P2PaperWarmup; P3Background)

type Client interface {
    Fetch(ctx context.Context, key string, tf candles.Timeframe,
          from, to time.Time, p Priority) ([]candles.Candle, error)
}
```

```text
Fetch ─► window split ─► enqueue(job{p}) ─┬─► worker A ─┐
                                          ├─► worker B ─┼─► limiter.Wait(ctx) ─► HTTP ─► normalise
                                          └─► worker C ─┘                         │
                                                        retry(429/5xx) ◄──────────┘
```

* Priority is not just ordering: the scheduler holds **three FIFO deques** and always drains P1 before P2
  before P3, plus a starvation guard (a P3 job that waits > 60 s is promoted) so background sync can't stall
  forever.
* The limiter is process-wide by construction: `NewClient()` takes the limiter as a dependency and
  `main` creates exactly one. A test asserts that two `Fetch` calls in the same second block the third.
* `unit` mapping per timeframe is a static table (`1s→seconds`, `1m→minutes`, … `1mo→months`), unit-tested.
* Requests must be context-cancellable; a cancelled job must not consume a retry.

## Tasks

- [ ] Client with windowing, unit mapping, response normalisation.
- [ ] Global limiter + three-deque priority scheduler with starvation guard.
- [ ] Retry/backoff with jitter, 401/429 classification, error types.
- [ ] Prometheus metrics + structured logs (request id, instrument, tf, priority, wait ms).
- [ ] Fake Upstox server test suite + limiter timing test (`testing/short` skip on CI lag).
- [ ] ADR `docs/adr/0008-rate-limit-budget.md` with the math on how much history 2 RPS can fetch per night.

## Acceptance criteria

1. 10 concurrent `Fetch` calls take ≥ 4 s wall time (2 RPS) and never exceed 2 requests in any 1 s window
   (asserted from the fake server's timestamps).
2. A P1 call issued while 20 P3 calls are queued is served first (assert order from the fake server log).
3. A 6-month 1m fetch is transparently split into ≥ 6 requests and returns one ordered, de-duplicated series.
4. A 429 response triggers a retry with backoff and finally returns a typed `ErrRateLimited` after 3 attempts.
5. A 401 returns `ErrUnauthorized` immediately with exactly 1 attempt.
6. `Fetch` with a cancelled context returns `context.Canceled` in < 100 ms.

## Test plan

- `httptest` fake Upstox recording timestamps and request order → assert RPS ceiling and priority order.
- Table test for window splitting across month boundaries and DST-free IST dates.
- `go test -race` on the scheduler.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.2, §7 (2 RPS constraint, priority queue),
docs/issues/SS-008-upstox-rest-2rps.md. Repo: Go 1.23, internal/upstox/auth provides TokenSource with
Token(ctx), candles package has a Timeframe enum with Duration()/Align() and a Candle struct,
config + slog + prometheus client already wired.

Implement SS-008 only: internal/upstox/historical, a client that is the single gateway to Upstox's
historical candle REST API. It must:
1. Fetch(ctx, instrumentKey, tf, from, to, priority) and map tf to Upstox interval/unit, splitting requests
   into the vendor's max window per timeframe (1m<=1 month, 1h<=3 months, 1d<=1 year, etc.) and merging the
   results into one ordered, de-duplicated []candles.Candle (Upstox returns [ts,o,h,l,c,v,oi] arrays).
2. Enforce a process-wide rate.NewLimiter(rate.Limit(2), 1) — one limiter instance injected at construction,
   never created per call.
3. Serve three priority deques (P1 UI backfill, P2 paper warmup, P3 background) with strict priority order and
   a starvation guard that promotes a P3 job waiting longer than 60s.
4. Retry 429/5xx with exponential backoff + jitter (cap 30s, max 3 attempts, each attempt consumes a token),
   return typed ErrUnauthorized immediately on 401 without retrying, and honour context cancellation without
   consuming retries.
5. Export metrics upstox_requests_total{priority,outcome}, queue depth, and wait duration, plus structured
   logs including instrument, timeframe, priority and wait time.

Constraints: nothing else in the codebase may call net/http for Upstox REST; no database writes here; no
sleep-based rate limiting (must use the limiter).

Done when: a fake Upstox server proves 10 concurrent fetches never exceed 2 req/s, a P1 request overtakes 20
queued P3 requests, a 6-month 1m fetch splits into >=6 calls returning one clean series, 429 retries then
returns ErrRateLimited, 401 fails immediately, and cancellation returns in <100ms; -race passes. Include ADR
0008 with the nightly fetch-budget math.
```

## Notes / risks

- Upstox occasionally returns bars with `volume=0` or duplicate timestamps; de-dupe deterministically
  (keep the last) and count it in a metric so SS-009's gap logic isn't poisoned.
- 2 RPS means ~7,200 requests/hour; a full 1m history for 500 stocks will take days. The background sync must
  therefore be incremental and resumable — that's SS-009.
