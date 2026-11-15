# SS-011 — Tick → 1s/1m Aggregator, Ring Buffer & Batched Writer

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-003, SS-005, SS-010 |
| Blocks | SS-012, SS-026 |
| Size | ~2.5 days |
| Stack | Go, PostgreSQL |

## Goal

Turn a high-rate tick stream into exactly one on-time "bar closed" event per `(stock, timeframe)`, keep the last
few minutes of bars hot in memory for instant chart delivery, and write to Postgres in batches instead of once
per tick.

## Scope

**In scope**
- `internal/marketdata/agg`:
  - `Aggregator` maintaining an in-progress bar per `(stockID, tf)` for `1s`, `1m` (and optionally wider TFs
    for live charts without waiting for a CAGG refresh).
  - Emits `BarClosed{StockID, TF, Candle}` on an internal event bus **exactly once**, at the aligned boundary,
    with a deterministic `bucket_time`.
  - Ring buffer: last N bars per `(stock, tf)` (default 900) for chart fast-path reads.
- `internal/marketdata/writer`: batched persistence — accumulate closed 1s/1m bars, flush every second or every
  1 000 bars, `CopyBatch`/`UpsertBatch` from SS-003, with backpressure (bounded channel; drop-oldest for 1s
  with a counter, block for 1m).
- Session gating: ticks outside `marketcal.IsOpen` are dropped (counter), and bars are force-closed at session
  end so no bar spans the close.
- Determinism: the aggregator is a **pure function of the tick sequence** — identical input ticks (same order,
  same timestamps) produce identical bars and identical `BarClosed` order. No wall-clock reads except a
  pluggable `Clock` interface.
- Metrics: ticks in, bars closed per tf, flush latency, buffer depth, dropped ticks.

**Out of scope**
- Who subscribes to what (SS-012). Strategy evaluation (SS-026).

## Small architecture

```text
Tick ─► sessionGate ─► Aggregator ─┬─ ring buffer (fast chart reads)
                                    ├─ BarClosed events ─► eventbus ─► paper engine / WS hub
                                    └─ closed bars ─► writer.queue ─► flush(1s) ─► CandleStore.UpsertBatch

Clock interface:  Now() time.Time        // real or simulated
```

```go
type Aggregator interface {
    OnTick(t Tick) []BarClosed              // returns 0..n closed bars, in deterministic order
    Snapshot(stockID int32, tf Timeframe) []Candle
    ForceClose(stockID int32, tf Timeframe, at time.Time) (BarClosed, bool)
}
```

* Late ticks (timestamp earlier than the current bar's start) are attributed to their correct bucket if that
  bucket is still open, otherwise counted as `late_dropped` — never mutating a bar already emitted.
* A bucket is closed when `tick.TS >= bucketStart+dur`; the closing tick belongs to the **next** bucket.
* Writer flush is time-driven but the *content* is sequence-driven, so replaying a recorded tick file
  reproduces the exact same DB rows (this is the foundation of SS-028's determinism proof).
* Backpressure policy is explicit and documented: 1s bars are allowed to shed under load (retention is 7 days
  anyway), 1m bars are never dropped.

## Tasks

- [ ] `Aggregator` + `Clock` + `BarClosed` events; 1s and 1m bar construction with OHLCV/oi propagation.
- [ ] Ring buffer with `Snapshot` and eviction; concurrency-safe.
- [ ] `ForceClose` at session end + gap handling via `marketcal`.
- [ ] Writer with periodic flush, batched upsert, backpressure, and shutdown drain.
- [ ] Replay harness: feed a checked-in `testdata/ticks.jsonl` and diff resulting bars against a golden file.
- [ ] Metrics + an integration test with a real Timescale container asserting rows land correctly.

## Acceptance criteria

1. Feeding ticks at 09:15:00.100 … 09:16:00.000 produces exactly one 1m `BarClosed` for 09:15 with
   open/high/low/close/volume equal to the tick-derived values, and no 09:16 bar yet.
2. OHLC invariants: `low <= open,close <= high`, `volume = sum(tick volumes)`.
3. Replaying `testdata/ticks.jsonl` twice yields byte-identical bars and identical DB rows.
4. A tick arriving after its bucket closed does not modify the emitted bar (covered by a `late_dropped` counter).
5. At 15:30:00 IST the open bar is force-closed exactly once and no bar has `bucket_time >= 15:30`.
6. Writer flushes ≤ 1 s apart and loses no 1m bars when the DB is paused for 5 s (it buffers, then drains).

## Test plan

- Table-driven aggregation tests around every boundary (59.999 s, 60.000 s, 60.001 s).
- Golden replay determinism test (twice + shuffled-but-timestamp-sorted variant).
- Integration test: aggregator → writer → real DB → read back via `Range`.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.1 and §7 (side-by-side live persistence, ring buffer, batch
writes), docs/issues/SS-011-tick-aggregator.md. Repo: Go 1.23; candles.Timeframe + Candle + CandleStore
(UpsertBatch/CopyBatch) from SS-003; marketcal.IsOpen/Session/BarsPerSession from SS-005; feed.Tick stream
from SS-010 (chan Tick with LTP, volume, OI, exchange timestamp).

Implement SS-011 only: internal/marketdata/agg and internal/marketdata/writer.
1. Aggregator with a pluggable Clock (real + simulated) that consumes ticks and produces closed 1s and 1m
   bars, emitting BarClosed{StockID, TF, Candle} exactly once per bucket on an internal event bus at the
   aligned boundary UTC bucket time derived from the IST-aligned epoch. Late ticks after a bucket closed must
   increment a late_dropped counter and never mutate an emitted bar.
2. Ring buffer per (stock, tf) holding the last 900 bars with a concurrency-safe Snapshot() for chart reads.
3. Session gating: drop ticks when marketcal.IsOpen is false (counter), and ForceClose the open bar at 15:30
   IST so no bar crosses the close.
4. Writer: bounded queue of closed bars, flush every 1s or every 1000 bars via CandleStore.UpsertBatch/
   CopyBatch; 1s bars may shed under pressure (counter) but 1m bars must never be dropped; drain cleanly on
   shutdown.
5. Determinism: the aggregator must be a pure function of the tick sequence — no direct time.Now() outside the
   Clock. Provide a replay harness that reads testdata/ticks.jsonl and produces bars identical to a committed
   golden file.
6. Metrics: ticks_in, bars_closed{tf}, flush_latency, queue_depth, dropped_ticks, late_dropped.

Constraints: no strategy logic, no subscription decisions, no HTTP handlers. Higher timeframes are derived by
the database, not here (except the optional live 5m view if you add it behind a flag).

Done when: boundary tests around 59.999/60.000/60.001s pass; OHLC invariants hold; double replay of the ticks
fixture is byte-identical and lands identical DB rows; ForceClose fires exactly once at 15:30 IST; pausing the
DB for 5s loses no 1m bar; -race passes. Include the golden-file diff evidence.
```

## Notes / risks

- The optional live 5m aggregation duplicates SS-004's CAGG; keep it behind a flag and clearly marked as a
  chart-latency optimisation, never a source of truth.
- `BarClosed` ordering must be total and stable (sort by `bucket_time` then `stock_id`) because SS-026's
  determinism depends on it.
