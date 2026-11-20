# MASTER BOARD
<!-- owner: orchestrator. Workers read only. Status columns are mirrors of worker logs; logs are the truth for agent state. -->
- updated: 2026-11-19T15:25:00Z
- stale_after_minutes: 15

## OBJECTIVE
Milestone 1 / SS-004: Implement TimescaleDB continuous aggregates (5m -> 1Y) with unified 09:15 IST bucket origin, refresh policies, chunk compression, raw retention, Go relation resolver, synthetic 1m test seeder, and `make tf-verify` verification tool.

## REQUIREMENTS
| id | requirement | acceptance | covered_by | status |
|---|---|---|---|---|
| R1 | Migration 0003_continuous_aggregates | 11 continuous aggregates + `v_candle_gaps` created `WITH NO DATA`; clean rollback | WP-CAGG-1 | VERIFIED |
| R2 | Migration 0004_policies | Refresh policies (1m end-offset), compression policies (7d, 30d, 90d, 180d), retention (7d, 30d); clean rollback | WP-CAGG-2 | VERIFIED |
| R3 | Go Timeframe Resolver & Range | All 13 enum values resolve to relation names; `Align()` aligns to 09:15 IST anchor; writes forbidden on 5m+ | WP-CAGG-3 | VERIFIED |
| R4 | ADR 0004 & Contracts | `docs/adr/0004-bucket-origins.md` and `candle-aggregates.md` contract frozen | WP-CAGG-0 | VERIFIED |
| R5 | Synthetic 1m Candle Seeder | `make seed-candles` populates ~3 months of 1m data for NSE market sessions | WP-CAGG-4 | VERIFIED |
| R6 | `make tf-verify` Verification Script | Direct comparison of continuous aggregates against from-scratch `time_bucket` SQL; byte-identical match | WP-CAGG-5 | VERIFIED |
| R7 | Integration & Quality Proof | Visibility test after refresh, 15:30 boundary check, empty input check, policy job presence in `timescaledb_information.jobs` | WP-CAGG-6 | VERIFIED |

## CONSTRAINTS
| rule | source | effect on plan |
|---|---|---|
| No application writes to 5m+ | SS-004 Goal | Application only writes 1s and 1m; 5m+ maintained solely by TimescaleDB |
| Unified 09:15 IST Origin | ADR 0004 | Origin `2015-01-01 09:15:00+05:30` mandatory for 1h/1d alignment & hierarchical cagg compatibility |
| Hierarchical Cagg Dependency | TimescaleDB 2.15 | `candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y` depend on `candles_1d` and must share origin |
| 1m End Offset on Refresh | ADR 0004 | Refresh policies use `end_offset => INTERVAL '1 minute'` to prevent locking live open candle |
| Permanent Daily+ Data | Platform Spec §5.2 | No retention policy on `candles_1d` or higher timeframes |

## CAPABILITY MATRIX
| WP | needed capability | assigned skill/agent | fit (OK / ADHOC / GAP) | reason if generic over specialist |
|---|---|---|---|---|
| WP-CAGG-0 | Architecture ADR & Contract | project-manager | OK | ADR 0004 author and contract freeze |
| WP-CAGG-1 | Continuous Aggregates DDL | pessimistic-minimalist-backend | OK | TimescaleDB continuous aggregates DDL, rollups, indexes |
| WP-CAGG-2 | Policies DDL Migration | pessimistic-minimalist-backend | OK | Refresh, compression, retention policies in SQL |
| WP-CAGG-3 | Go Relation Resolver & Align | lazy-senior-engineer | OK | Timeframe enum, Align() arithmetic, Range support |
| WP-CAGG-4 | Synthetic 1m Seeder | lazy-senior-engineer | OK | High-speed synthetic bar generation with CopyBatch |
| WP-CAGG-5 | tf-verify Verification Tool | qa-tester | OK | Row-by-row verification against raw computation |
| WP-CAGG-6 | Integration & Verification Gate | verify-and-stop | OK | Adversarial integration tests and acceptance proofs |

## WAVES AND WORK PACKAGES
| wave | WP | agent-id | depends_on | write_scope | state (mirror) | synced |
|---|---|---|---|---|---|---|
| 0 | WP-CAGG-0 | orchestrator | none | `docs/adr/0004-bucket-origins.md`, `.orchestrator/contracts/candle-aggregates.md` | COMPLETED | yes |
| 1 | WP-CAGG-1 | db-engineer | WP-CAGG-0 | `db/migrations/0003_continuous_aggregates.*.sql` | COMPLETED | yes |
| 1 | WP-CAGG-3 | backend-engineer | WP-CAGG-0 | `api/internal/store/candles/**` | COMPLETED | yes |
| 2 | WP-CAGG-2 | db-engineer | WP-CAGG-1 | `db/migrations/0004_policies.*.sql` | COMPLETED | yes |
| 2 | WP-CAGG-4 | backend-engineer | WP-CAGG-1, WP-CAGG-3 | `api/cmd/seed-candles/**`, `Makefile` | COMPLETED | yes |
| 3 | WP-CAGG-5 | qa-tester | WP-CAGG-2, WP-CAGG-4 | `api/cmd/tf-verify/**`, `Makefile` | COMPLETED | yes |
| 4 | WP-CAGG-6 | orchestrator | WP-CAGG-3, WP-CAGG-5 | `api/internal/store/candles/cagg_test.go`, `.orchestrator/master-board.md` | COMPLETED | yes |

## CONTRACTS
| name | version | path | status (DRAFT / FROZEN) |
|---|---|---|---|
| metadata-schema | 1 | `.orchestrator/contracts/metadata-schema.md` | FROZEN |
| candle-store | 1 | `.orchestrator/contracts/candle-store.md` | FROZEN |
| candle-aggregates | 1 | `.orchestrator/contracts/candle-aggregates.md` | FROZEN |

## RESOURCE REGISTRY
| resource (port, db, env key) | value or name | owner |
|---|---|---|
| Unified Origin Constant | `2015-01-01 09:15:00+05:30` (UTC: `2015-01-01 03:45:00Z`) | ADR 0004 / Migration 0003 |
| Intraday Continuous Aggregates | `candles_5m`, `candles_10m`, `candles_15m`, `candles_30m`, `candles_1h`, `candles_3h`, `candles_1d` | Migration 0003 |
| Macro Continuous Aggregates | `candles_15d`, `candles_1mo`, `candles_6mo`, `candles_1y` | Migration 0003 |
| Gap Detection View | `v_candle_gaps` | Migration 0003 |
| TimescaleDB Jobs Registry | `timescaledb_information.jobs` | Migration 0004 |

## DIRECTIVES
| id | target agent | directive (HALT / RESUME / CHANGE) | detail | issued |
|---|---|---|---|---|
| DIR-01 | all | RESUME | SS-003 candle store and hypertables verified | 2026-11-18T16:00:00Z |
| DIR-02 | all | RESUME | SS-004 continuous aggregates plan approved, beginning implementation | 2026-11-19T15:25:00Z |

## ARBITRATION LOG
- 2026-11-19: Selected `2015-01-01 09:15:00+05:30` as the platform-wide unified origin. Anchors 1h bars to 09:15, 10:15... IST, keeps 1d session bars within 09:15-15:30 IST, and enables hierarchical caggs (`candles_15d`, `candles_1mo`, etc.) to build directly on `candles_1d` without origin mismatch errors.
