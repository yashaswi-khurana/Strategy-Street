# MASTER BOARD
<!-- owner: orchestrator. Workers read only. Status columns are mirrors of worker logs; logs are the truth for agent state. -->
- updated: 2026-11-20T21:40:00Z
- stale_after_minutes: 15

## OBJECTIVE
Milestone 0 / SS-005: Implement Market Session Filter & Missing-Bar Policy across Go (`internal/marketcal`) and Python (`engine/marketcal.py`) with half-open `[09:15, 15:30)` IST boundaries, deterministic wall-clock-free `NextBarClose`, read-time `CarryForward` gap filling, and schema honest `is_filled` migration.

## REQUIREMENTS
| id | requirement | acceptance | covered_by | status |
|---|---|---|---|---|
| R1 | Migration 0005_is_filled | `candles_1s` and `candles_1m` get `is_filled BOOLEAN NOT NULL DEFAULT FALSE`; continuous aggregates pass `bool_and(is_filled)` | WP-SESSION-1 | VERIFIED |
| R2 | Go Package internal/marketcal | `Session()`, `IsOpen()`, `TradingDays()`, `NextBarClose()`, and `BarsPerSession()` pass all boundary checks | WP-SESSION-2 | VERIFIED |
| R3 | Read-Time FillPolicy Decorator | `FilledRange` decorates intra-session gaps under `CarryForward` with previous close, vol=0, `is_filled=true`; zero DB writes | WP-SESSION-2 | VERIFIED |
| R4 | Python Mirror engine/marketcal.py | 1:1 identical semantics and signatures in Python | WP-SESSION-3 | VERIFIED |
| R5 | Cross-Language Fixtures & ADR 0005 | Shared `engine/testdata/sessions.json` verified byte-identical in pytest and Go tests; ADR 0005 accepted | WP-SESSION-0, WP-SESSION-3 | VERIFIED |

## CONSTRAINTS
| rule | source | effect on plan |
|---|---|---|
| Half-open `[09:15, 15:30)` IST | SS-005 Goal | 15:30:00 tick belongs to NO bar; never create a phantom last-second bar |
| Wall-Clock Independence | SS-005 Architecture | `NextBarClose` is pure with respect to wall-clock seconds for deterministic paper trading |
| Read-Time Only Gap Filling | SS-005 Architecture | Filled bars are never persisted to database; `ValidateBar` rejects `is_filled=true` |
| 1:1 Go/Python Parity | SS-005 Goal | Shared JSON fixture file ensures identical behavior across both engines |

## CAPABILITY MATRIX
| WP | needed capability | assigned skill/agent | fit (OK / ADHOC / GAP) | reason if generic over specialist |
|---|---|---|---|---|
| WP-SESSION-0 | Architecture ADR & Contract | project-manager | OK | ADR 0005 author and contract freeze |
| WP-SESSION-1 | Migration 0005 & Store Update | pessimistic-minimalist-backend | OK | Schema alter, continuous aggregates, candle store |
| WP-SESSION-2 | Go Marketcal Implementation | lazy-senior-engineer | OK | Calendar loader, Session/IsOpen, FilledRange decorator |
| WP-SESSION-3 | Python Marketcal Mirror | lazy-senior-engineer | OK | Python engine mirror, shared fixture tests |
| WP-SESSION-4 | Integration & Quality Proof | verify-and-stop | OK | Unit tests, pytest cross-language validation, linters |

## WAVES AND WORK PACKAGES
| wave | WP | agent-id | depends_on | write_scope | state (mirror) | synced |
|---|---|---|---|---|---|---|
| 0 | WP-SESSION-0 | orchestrator | none | `docs/adr/0005-session-boundaries.md`, `.orchestrator/contracts/session-filter.md` | COMPLETED | yes |
| 1 | WP-SESSION-1 | db-engineer | WP-SESSION-0 | `db/migrations/0005_is_filled.*.sql`, `api/internal/store/candles/**` | COMPLETED | yes |
| 2 | WP-SESSION-2 | backend-engineer | WP-SESSION-0, WP-SESSION-1 | `api/internal/marketcal/**`, `engine/testdata/sessions.json` | COMPLETED | yes |
| 3 | WP-SESSION-3 | backend-engineer | WP-SESSION-2 | `engine/engine/marketcal.py`, `engine/marketcal.py`, `engine/tests/test_marketcal.py` | COMPLETED | yes |
| 4 | WP-SESSION-4 | orchestrator | WP-SESSION-1, WP-SESSION-2, WP-SESSION-3 | `.orchestrator/master-board.md` | COMPLETED | yes |

## CONTRACTS
| name | version | path | status (DRAFT / FROZEN) |
|---|---|---|---|
| metadata-schema | 1 | `.orchestrator/contracts/metadata-schema.md` | FROZEN |
| candle-store | 1 | `.orchestrator/contracts/candle-store.md` | FROZEN |
| candle-aggregates | 1 | `.orchestrator/contracts/candle-aggregates.md` | FROZEN |
| session-filter | 1 | `.orchestrator/contracts/session-filter.md` | FROZEN |

## RESOURCE REGISTRY
| resource (port, db, env key) | value or name | owner |
|---|---|---|
| Regular Session Hours | `09:15:00` - `15:30:00` Asia/Kolkata | ADR 0005 / marketcal |
| Schema Column Flag | `is_filled BOOLEAN NOT NULL DEFAULT FALSE` | Migration 0005 |
| Shared Cross-Language Fixture | `engine/testdata/sessions.json` | marketcal / testdata |

## DIRECTIVES
| id | target agent | directive (HALT / RESUME / CHANGE) | detail | issued |
|---|---|---|---|---|
| DIR-01 | all | RESUME | SS-003 candle store and hypertables verified | 2026-11-18T16:00:00Z |
| DIR-02 | all | RESUME | SS-004 continuous aggregates plan approved and verified | 2026-11-19T15:25:00Z |
| DIR-03 | all | RESUME | SS-005 session filter & missing-bar policy implemented and verified | 2026-11-20T21:40:00Z |

## ARBITRATION LOG
- 2026-11-20: Enforced half-open `[09:15, 15:30)` IST session boundaries. A tick at `15:30:00` belongs to NO bar.
- 2026-11-20: Forward-filled missing bars are decorated strictly at query/read-time with `is_filled=true` and `volume=0`. `ValidateBar()` actively forbids application writes with `is_filled=true` to guarantee zero phantom storage.
