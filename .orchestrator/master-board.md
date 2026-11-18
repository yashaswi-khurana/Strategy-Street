# MASTER BOARD
<!-- owner: orchestrator. Workers read only. Status columns are mirrors of worker logs; logs are the truth for agent state. -->
- updated: 2026-11-18T16:00:00Z
- stale_after_minutes: 15

## OBJECTIVE
Milestone 1 / SS-003: Deliver high-performance TimescaleDB hypertables for raw resolutions (`candles_1s`, `candles_1m`) and the Go repository (`CandleStore`) with batch upsert, range retrieval, and high-speed copy capabilities.

## REQUIREMENTS
| id | requirement | acceptance | covered_by | status |
|---|---|---|---|---|
| R1 | Migration 0002_candles | Create `candles_1s` & `candles_1m` hypertables (1-day chunks, unique `(stock_id, bucket_time)`); clean down rollback | WP-CANDLE-1 | VERIFIED |
| R2 | Timeframe enum & Epoch alignment | `Timeframe` (1s..1y), `Align(t)` against `1970-01-01T00:00:00+05:30`, validator rejects non-UTC and misaligned bar times (e.g. 09:15:30 on 1m) | WP-CANDLE-2 | VERIFIED |
| R3 | CandleStore Interface & Pgx Store | `UpsertBatch` (conflict update), `Range` (indexed ascending), `Latest`, `LatestTime`, `CopyBatch` (temp table merge) | WP-CANDLE-3 | VERIFIED |
| R4 | High-throughput batch copy | `CopyBatch` of 100k rows completes < 5s locally and is idempotent | WP-CANDLE-3 | VERIFIED (3.87s) |
| R5 | ADR 0003: Numeric vs. Float | Document NUMERIC(12,4) storage vs float64 Python vectorization with centralized single-helper conversion | WP-CANDLE-0 | VERIFIED |

## CONSTRAINTS
| rule | source | effect on plan |
|---|---|---|
| Raw unadjusted data only | SS-003 Scope | `is_adjusted = FALSE` strictly enforced in SS-003; adjusted prices deferred to SS-006 |
| No application writes to 5m+ | SS-003 Scope | Application never writes 5m or higher timeframes; continuous aggregates deferred to SS-004 |
| IST Epoch alignment | SS-003 Architecture | All `bucket_time`s must align to `1970-01-01T00:00:00+05:30` and be stored in UTC |
| Single conversion helper | SS-003 Architecture | `NUMERIC(12,4)` to/from `float64` conversions isolated strictly in `numeric.go` |

## CAPABILITY MATRIX
| WP | needed capability | assigned skill/agent | fit (OK / ADHOC / GAP) | reason if generic over specialist |
|---|---|---|---|---|
| WP-CANDLE-0 | Architecture ADR & Identity | project-manager | OK | ADR 0003 author and boundary contract owner |
| WP-CANDLE-1 | Hypertables DDL Migration | pessimistic-minimalist-backend | OK | TimescaleDB hypertable DDL, 1-day chunks, index tuning |
| WP-CANDLE-2 | Timeframe Enum & Epoch Alignment | lazy-senior-engineer | OK | Pure Go temporal arithmetic and validation |
| WP-CANDLE-3 | Pgx CandleStore & Copy Engine | lazy-senior-engineer | OK | High-speed multi-row upsert, pgx.CopyFrom, index scan query |
| WP-CANDLE-4 | Testcontainers Quality & Benchmark Gate | verify-and-stop | OK | Adversarial empirical proof and throughput benchmarking |

## WAVES AND WORK PACKAGES
| wave | WP | agent-id | depends_on | write_scope | state (mirror) | synced |
|---|---|---|---|---|---|---|
| 0 | WP-CANDLE-0 | orchestrator | none | `docs/adr/0003-numeric-vs-float.md` | COMPLETED | yes |
| 1 | WP-CANDLE-1 | db-engineer | WP-CANDLE-0 | `db/migrations/0002_candles.*.sql` | COMPLETED | yes |
| 1 | WP-CANDLE-2 | backend-engineer | WP-CANDLE-0 | `api/internal/store/candles/candle*.go` | COMPLETED | yes |
| 2 | WP-CANDLE-3 | backend-engineer | WP-CANDLE-1, WP-CANDLE-2 | `api/internal/store/candles/store*.go` | COMPLETED | yes |
| 3 | WP-CANDLE-4 | qa-tester | WP-CANDLE-3 | `api/internal/store/candles/*test*.go` | COMPLETED | yes |

## CONTRACTS
| name | version | path | status (DRAFT / FROZEN) |
|---|---|---|---|
| metadata-schema | 1 | `.orchestrator/contracts/metadata-schema.md` | FROZEN |
| candle-store | 1 | `.orchestrator/contracts/candle-store.md` | FROZEN |

## RESOURCE REGISTRY
| resource (port, db, env key) | value or name | owner |
|---|---|---|
| TimescaleDB Hypertables | `candles_1s`, `candles_1m` | db/migrations/0002_candles.up.sql |
| Chunk Time Interval | 1 day (`INTERVAL '1 day'`) | TimescaleDB hypertable partition |
| Timezone Epoch Reference | `1970-01-01T00:00:00+05:30` (IST) | api/internal/store/candles/candle.go |

## DIRECTIVES
| id | target agent | directive (HALT / RESUME / CHANGE) | detail | issued |
|---|---|---|---|---|
| DIR-01 | all | RESUME | SS-002 completed and verified | 2026-11-18T15:30:00Z |
| DIR-02 | all | RESUME | SS-003 candle store and hypertables verified | 2026-11-18T16:00:00Z |

## ARBITRATION LOG
- 2026-11-18: Benchmarked `CopyBatch` vs `UpsertBatch`: single-day intraday batch upsert achieves ~18.5k bars/sec; bulk backfill using `CopyBatch` with temp table staging achieves ~25.8k bars/sec (copying 100k rows in < 4s). In-memory mock store delivers ~580k bars/sec for paper trading test doubles.
