# MASTER BOARD
<!-- owner: orchestrator. Workers read only. Status columns are mirrors of worker logs; logs are the truth for agent state. -->
- updated: 2026-11-21T12:15:00Z
- stale_after_minutes: 15

## OBJECTIVE
Milestone 1 / SS-006: Implement Corporate Actions & Split/Dividend Adjustment Pipeline across PostgreSQL (`corporate_actions`, `adjustment_factors`, `data_quality_issues`, stored function `adjusted_candles`) and Go (`api/internal/store/corporate`, `ca-recompute`, `ca-detect`) with cumulative backward adjustment, immutable raw candle hypertables, query-time lateral join adjustment, and automated overnight jump quality detection.

## REQUIREMENTS
| id | requirement | acceptance | covered_by | status |
|---|---|---|---|---|
| R1 | Migration 0006 Schema & Functions | `0006_corporate_actions.up.sql` creates tables and `adjusted_candles()`; `down.sql` cleanly rolls back | WP-CA-1 | VERIFIED |
| R2 | Go Package internal/store/corporate | Factor math unit tests (split 1:5, bonus 1:1, bonus 1:2, dividend, chains) pass against hand-computed values | WP-CA-2 | VERIFIED |
| R3 | Query-Time adjusted_candles Series Continuity | 1:5 split with pre-split close 2500 yields continuous 500 -> 500 series; volume strictly unchanged | WP-CA-2 | VERIFIED |
| R4 | CLI Binaries ca-recompute & ca-detect | `make ca-recompute --stock=RELIANCE` idempotent; `make ca-detect --stock=...` flags overnight jumps > ±35% and unverified actions into `data_quality_issues` | WP-CA-3 | VERIFIED |
| R5 | Historical Seeds & ADR 0006 | 6 real corporate actions seeded; `docs/adr/0006-adjustment-method.md` accepted | WP-CA-0, WP-CA-1 | VERIFIED |

## CONSTRAINTS
| rule | source | effect on plan |
|---|---|---|
| Backward Adjustment | SS-006 Goal | Cumulative backward adjustment scales historical prices strictly before `ex_date`; current quotes unaffected |
| Immutable Raw Candles | SS-006 Architecture | Raw hypertables (`candles_1s`, `candles_1m`) are never modified; adjustment applied at query time |
| Volume / OI Invariance | SS-006 Architecture | Volume and open interest are never multiplied or altered |
| Verified Gate Only | SS-006 Architecture | Unverified actions never adjust prices and are quarantined in `data_quality_issues` |
| Idempotent Recompute | SS-006 Architecture | Running `ca-recompute` $N$ times results in identical `adjustment_factors` rows |

## CAPABILITY MATRIX
| WP | needed capability | assigned skill/agent | fit (OK / ADHOC / GAP) | reason if generic over specialist |
|---|---|---|---|---|
| WP-CA-0 | Architecture ADR & Contract | project-manager | OK | ADR 0006 author and contract freeze |
| WP-CA-1 | Migration 0006 & Historical Seeds | pessimistic-minimalist-backend | OK | Schema definition, SQL lateral join, historical seeds |
| WP-CA-2 | Go Corporate Store & Factor Math | lazy-senior-engineer | OK | Pure factor math, pgx repository, integration tests |
| WP-CA-3 | CLI Binaries & Makefile Targets | ponytail | OK | ca-recompute, ca-detect, CLI flag handling |
| WP-CA-4 | Integration & Quality Proof | verify-and-stop | OK | Acceptance criteria 1-5 validation, linters |

## WAVES AND WORK PACKAGES
| wave | WP | agent-id | depends_on | write_scope | state (mirror) | synced |
|---|---|---|---|---|---|---|
| 0 | WP-CA-0 | orchestrator | none | `docs/adr/0006-adjustment-method.md`, `docs/issues/SS-006-corporate-actions.md`, `.orchestrator/contracts/corporate-actions.md` | COMPLETED | yes |
| 1 | WP-CA-1 | db-engineer | WP-CA-0 | `db/migrations/0006_corporate_actions.*.sql`, `db/seeds/05_corporate_actions.sql` | COMPLETED | yes |
| 2 | WP-CA-2 | backend-engineer | WP-CA-0, WP-CA-1 | `api/internal/store/corporate/**` | COMPLETED | yes |
| 3 | WP-CA-3 | backend-engineer | WP-CA-2 | `api/cmd/ca-recompute/**`, `api/cmd/ca-detect/**`, `Makefile` | COMPLETED | yes |
| 4 | WP-CA-4 | orchestrator | WP-CA-1, WP-CA-2, WP-CA-3 | `.orchestrator/master-board.md` | COMPLETED | yes |

## CONTRACTS
| name | version | path | status (DRAFT / FROZEN) |
|---|---|---|---|
| metadata-schema | 1 | `.orchestrator/contracts/metadata-schema.md` | FROZEN |
| candle-store | 1 | `.orchestrator/contracts/candle-store.md` | FROZEN |
| candle-aggregates | 1 | `.orchestrator/contracts/candle-aggregates.md` | FROZEN |
| session-filter | 1 | `.orchestrator/contracts/session-filter.md` | FROZEN |
| corporate-actions | 1 | `.orchestrator/contracts/corporate-actions.md` | FROZEN |

## RESOURCE REGISTRY
| resource (port, db, env key) | value or name | owner |
|---|---|---|
| Stored Function | `adjusted_candles(p_stock, p_tf, p_from, p_to)` | Migration 0006 |
| Tables | `corporate_actions`, `adjustment_factors`, `data_quality_issues` | Migration 0006 |
| CLI Binaries | `cmd/ca-recompute`, `cmd/ca-detect` | corporate store |

## DIRECTIVES
| id | target agent | directive (HALT / RESUME / CHANGE) | detail | issued |
|---|---|---|---|---|
| DIR-01 | all | RESUME | SS-003 candle store and hypertables verified | 2026-11-18T16:00:00Z |
| DIR-02 | all | RESUME | SS-004 continuous aggregates plan approved and verified | 2026-11-19T15:25:00Z |
| DIR-03 | all | RESUME | SS-005 session filter & missing-bar policy implemented and verified | 2026-11-20T21:40:00Z |
| DIR-04 | all | RESUME | SS-006 corporate actions & adjustment pipeline implemented and verified | 2026-11-21T12:15:00Z |

## ARBITRATION LOG
- 2026-11-21: Adopted backward adjustment with query-time SQL lateral join. Raw candles remain 100% immutable.
- 2026-11-21: Strictly enforced that volume and OI are never modified by corporate action factors.
- 2026-11-21: Unverified corporate actions are quarantined in `data_quality_issues` as `unverified_action` and do not alter adjusted prices.
- 2026-11-21: Overnight jumps beyond ±35% without verified actions are flagged idempotently into `data_quality_issues` as `unexplained_jump`.
