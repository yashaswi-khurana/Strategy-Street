# SS-020 — Redis Streams + gRPC Worker Pool with Progress Streaming

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-015, SS-018 |
| Blocks | SS-022 |
| Size | ~3 days |
| Stack | Redis 7 Streams, gRPC, Python, Go client |

## Goal

Run hundreds of strategy jobs asynchronously across a warm worker pool, with cancellation, retries, dead
letters and live progress — so no HTTP request ever waits on a backtest.

## Scope

**In scope**
- Job contract (`contracts/jobs.proto` + JSON mirror):
  - `BacktestJob{job_id, run_id, strategy_version_id, source_ref, stock_id, symbol, timeframe, from, to,
    params, cost_config_version, limits}`.
  - `Progress{job_id, run_id, stocks_done, stocks_total, percent, current_symbol, eta_seconds}`.
  - `JobResult{job_id, status, metrics, trades_uri, error, usage}` — large payloads stored as artifacts, not
    inline.
- Python producer/consumer: `engine/worker/consumer.py` using Redis Streams consumer groups (`XADD`,
  `XREADGROUP`, `XACK`, `XAUTOCLAIM`) with a claim timeout so a crashed worker's job is re-delivered.
- gRPC service `EngineService` (`RunBacktest(stream) returns (stream Progress|Result)`, `Health`), plus a
  Redis-Streams-only mode for simpler deployments (pick one as the default and document the other as an
  alternative).
- Go side: `internal/queue` — enqueue, `SubscribeProgress(runID)` fan-out channel, cancel by `run_id`,
  retry policy (3 attempts, exponential backoff), dead-letter stream `jobs:dead`.
- Idempotency: results keyed by `(job_id)`; a duplicate delivery returns the cached result instead of
  recomputing.
- Worker warm-up: preloaded pandas/numpy + sandbox image warm pool so per-job overhead < 200 ms.

**Out of scope**
- Deciding which jobs to enqueue (SS-022). Persisting metrics to Postgres (SS-023).

## Small architecture

```text
Go api ──XADD jobs:backtest──► Redis Stream ──XREADGROUP──► Python worker (pool of N)
   │                                                              │
   │◄──────────── gRPC stream Progress/Result ────────────────────┘
   │
   └── fan-out to /ws (run progress)

Redis keys: jobs:backtest (stream) | jobs:progress:<run_id> | jobs:result:<job_id> (hash, TTL 24h)
            jobs:dead (stream) | jobs:pending:<run_id> (set, for cancel)
```

```go
type Queue interface {
    Enqueue(ctx context.Context, job BacktestJob) error
    Progress(ctx context.Context, runID string) (<-chan Progress, func(), error)
    Cancel(ctx context.Context, runID string) error
}
```

* Cancellation is a Redis key (`cancel:<run_id>`) checked by workers every N bars (via SS-018's
  `cancel_check`), not a hard kill — the worker acknowledges and finishes cleanly.
* Backpressure: producers block when stream length > threshold; workers use `MaxLen` trimming for progress.
* Progress is best-effort (lossy) but monotonic: `stocks_done` only increases; the final `Result` is the source
  of truth.
* Visibility: `WorkerStats{in_flight, completed, failed, avg_duration_ms}` exported by both languages.

## Tasks

- [ ] `contracts/jobs.proto` + Go and Python codegen (`make proto`).
- [ ] Python consumer with consumer group, XAUTOCLAIM re-delivery, cancel checks, retry counting.
- [ ] gRPC server streaming progress + result; health check.
- [ ] Go `internal/queue` client with progress fan-out, cancellation, retry/DLQ.
- [ ] Idempotent result cache + artifact storage (local dir / S3-compatible, pluggable).
- [ ] Integration test: 100 jobs across 2 worker processes, one killed mid-flight → all jobs complete once.
- [ ] ADR `docs/adr/0020-grpc-vs-streams.md`.

## Acceptance criteria

1. Submitting 100 jobs with 2 workers completes all 100 exactly once; total wall time scales sub-linearly vs a
   single worker (≥ 1.7× speedup).
2. Killing a worker mid-job re-delivers that job via `XAUTOCLAIM` within the claim timeout and it completes.
3. A job that always fails is retried 3× then lands in `jobs:dead` with the error preserved.
4. `Cancel(run_id)` stops in-flight jobs within 2 s and reports `status=cancelled` for the run.
5. Progress stream emits monotonically increasing `percent` and a terminal `100`/`failed` event.
6. Worker RSS stays flat over 200 sequential jobs (no leak); per-job overhead < 200 ms excluding compute.

## Test plan

- Integration tests with real Redis + real gRPC (testcontainers), 2 worker processes.
- Chaos test: kill -9 a worker, assert exactly-once completion.
- Load test: 500 trivial jobs, measure throughput and p95 latency.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.8 (gRPC + worker queue, progress streaming),
docs/issues/SS-020-queue-and-workers.md. Repo: Go api (chi, pgx, slog, prometheus), Python engine with
backtest/vectorized.py (SS-018) exposing run(df, strategy, config, cancel_check), sandbox runner (SS-015),
Redis 7 and docker-compose already present.

Implement SS-020 only: the async job transport between Go and Python.
1. contracts/jobs.proto with BacktestJob{job_id, run_id, strategy_version_id, source_ref, stock_id, symbol,
   timeframe, from, to, params, cost_config_version, limits}, Progress{job_id, run_id, stocks_done,
   stocks_total, percent, current_symbol, eta_seconds}, JobResult{job_id, status, metrics, trades_uri, error,
   usage}. Generate Go and Python stubs via `make proto`.
2. Python worker: Redis Streams consumer group (XADD/XREADGROUP/XACK/XAUTOCLAIM) with claim timeout, retry
   counting (3 attempts then jobs:dead), cancellation via a cancel:<run_id> key polled through the backtest
   cancel_check, and result caching keyed by job_id (duplicate delivery returns the cached result). Preload
   pandas/numpy and keep a warm sandbox so per-job overhead is under 200ms.
3. gRPC EngineService: RunBacktest(stream) returning a stream of Progress and a terminal JobResult, plus Health.
   Keep a Redis-only mode as a documented alternative and pick one as default in ADR 0020.
4. Go internal/queue: Queue interface with Enqueue, Progress(ctx, runID) (<-chan Progress, cancelFunc, error),
   Cancel(ctx, runID); retry with exponential backoff; dead-letter stream; prometheus metrics for queue depth,
   in-flight, failures, job duration.
5. Artifact storage for trades/equity blobs via a small pluggable interface (local dir default).

Constraints: no backtest orchestration decisions here (SS-022 owns fan-out); no Postgres writes for metrics
(SS-023); progress may be lossy but must be monotonic; cancellation must be cooperative, not SIGKILL.

Done when: 100 jobs on 2 workers complete exactly once with >=1.7x speedup over 1 worker; kill -9 of a worker
re-delivers and completes its job; a poisoned job retries 3x then appears in jobs:dead; Cancel stops in-flight
work in <2s; progress is monotonic ending at 100 or failed; RSS is flat over 200 jobs. Include the chaos-test
transcript.
```

## Notes / risks

- A warm pool of pandas processes plus a warm sandbox is the difference between 5 s and 0.5 s per stock; make
  the pool size configurable and measured.
- Progress writes can dominate Redis traffic for a 500-stock run — throttle to 1 update/second per run.
