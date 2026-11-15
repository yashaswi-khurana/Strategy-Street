# SS-021 — Strategy CRUD + Immutable Versioning API

| | |
| --- | --- |
| Milestone | 3 — Orchestration |
| Depends on | SS-002, SS-014 |
| Blocks | SS-022, SS-025, SS-031 |
| Size | ~2 days |
| Stack | Go, PostgreSQL, OpenAPI |

## Goal

"Add Strategy" needs a safe backend: validate the code, store it as an immutable version, and keep every
backtest and paper trade pinned to the exact version it ran with.

## Scope

**In scope**
- Tables `strategies`, `strategy_versions` (spec §5.1) + `strategy_version_validation` (diagnostics snapshot).
- Endpoints (contracts in `contracts/openapi.yaml`):
  - `POST /api/v1/strategies` `{name, description, python_code, parameters}` → validates (SS-014), creates
    strategy + version 1, returns `{strategy_id, version_id, version_number, diagnostics}`. `422` with
    diagnostics on validation failure (store the rejected attempt? no — reject without persisting code).
  - `POST /api/v1/strategies/{id}/versions` → new immutable version; `current_version` pointer moves; old
    versions untouched.
  - `GET /api/v1/strategies`, `GET /api/v1/strategies/{id}?include_versions=false`,
    `GET /api/v1/strategies/{id}/versions`, `GET /api/v1/strategy-versions/{vid}`.
  - `PATCH /api/v1/strategies/{id}` — metadata only (name/description); code changes require a new version.
- `POST /api/v1/strategies/validate` — lint-only endpoint (no persistence) for live editor feedback.
- `ast_hash` (SHA-256 of normalised AST dump) for dedupe/identical-code detection; `params` as JSONB with
  schema validation (numeric/string/bool only, max 32 keys).
- Template endpoint `GET /api/v1/strategies/template` returning SS-013's template constant.
- Ownership field `user_id` + auth middleware hook (placeholder user in dev).

**Out of scope**
- Triggering backtests (SS-022). Execution (SS-020). Editor UI (SS-031).

## Small architecture

```text
POST /strategies ─► auth ─► validate(params) ─► ast_validate(code) ──422 diagnostics──┐
                                                  │ ok                                 │
                                                  ├─ ast_hash(code)                  │
                                                  ├─ INSERT strategies (current_version=1)
                                                  ├─ INSERT strategy_versions (immutable)
                                                  └─ 201 {strategy_id, version_id}
```

Key invariants:
* `strategy_versions` is **append-only**: enforce with a DB rule/trigger rejecting `UPDATE`/`DELETE` (except
  `UPDATE` never at all) and no `updated_at` column.
* `strategy_versions.ast_hash` is unique per `strategy_id` — re-submitting identical code returns the existing
  version instead of a new one (idempotent edits).
* `parameters_json` is the *declared* defaults; per-run overrides are stored on the run, not here.
- Validation is performed by calling the Python validator through a small internal client interface
  (`ValidatorClient`) — Go never shells out to Python in production; a `local` implementation shells out for dev.

## Tasks

- [ ] Migrations: strategies, strategy_versions, validation snapshots; append-only trigger.
- [ ] Store layer + handlers with OpenAPI spec entries and generated Go types.
- [ ] `ValidatorClient` interface (gRPC to engine + local dev impl) and diagnostics passthrough.
- [ ] AST hashing + idempotent version creation.
- [ ] Errors: `409` duplicate name per user, `404`, `422` diagnostics, `400` bad params.
- [ ] Tests: happy path, validation failure, immutable-version enforcement, concurrent version creation.

## Acceptance criteria

1. Creating a strategy stores version 1 with `python_code`, `ast_hash`, `params`, and returns both ids.
2. Submitting identical code twice yields the same `version_id` (no duplicate version).
3. Editing code creates version 2; version 1's row is unchanged (verified by hash before/after) and
   `current_version=2`.
4. `UPDATE strategy_versions SET python_code='x'` fails at the database level (test asserts the trigger).
5. Invalid strategy (e.g. `import os`) returns `422` with at least one diagnostic containing
   `rule_id=IMPORT_FORBIDDEN`, `line`, `col` — and persists nothing.
6. `GET /strategies/{id}/versions` returns versions newest-first with hashes and creation timestamps.
7. Two concurrent `POST /versions` requests produce versions 2 and 3 with no gaps or duplicates.

## Test plan

- Handler tests with a fake validator client + real Postgres container.
- Concurrency test with `pgx` transactions and a unique constraint on `(strategy_id, version_number)`.
- Contract test: response bodies validate against `contracts/openapi.yaml` schemas.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §2.3, §3.1, §3.10 and §5.1 (strategies/strategy_versions),
docs/issues/SS-021-strategy-versions-api.md. Repo: Go 1.23 + chi + pgx, OpenAPI-first (contracts/openapi.yaml,
client generated), Python AST validator from SS-014, SDK template constant from SS-013, auth middleware hook.

Implement SS-021 only: strategy CRUD with immutable versioning.
1. Migrations for strategies (id, user_id, name unique per user, description, current_version, created_at) and
   strategy_versions (id, strategy_id, version_number unique per strategy, python_code TEXT, parameters_json
   JSONB, ast_hash, rules_version, created_at) plus a validation snapshot table. Make strategy_versions
   append-only with a trigger that rejects UPDATE and DELETE.
2. Endpoints: POST /api/v1/strategies (name, description, python_code, parameters) -> validate params then call
   the ValidatorClient (interface with gRPC impl + local shell-out dev impl) running SS-014; on failure return
   422 with the diagnostics array and persist nothing. On success create strategy + version 1 and return
   {strategy_id, version_id, version_number, diagnostics}. POST /strategies/{id}/versions creates a new
   immutable version and moves current_version. GET /strategies, GET /strategies/{id},
   GET /strategies/{id}/versions (newest first), GET /strategy-versions/{vid}. PATCH /strategies/{id} for
   name/description only. POST /strategies/validate for lint-only editor feedback with no persistence.
   GET /strategies/template returning the SDK template constant.
3. ast_hash = sha256 of a normalised ast.dump; re-submitting identical code returns the existing version
   instead of creating a duplicate. parameters_json validated as a flat map of numbers/strings/bools, max 32
   keys.
4. Errors: 409 duplicate name per user, 404 not found, 422 diagnostics, 400 bad parameters, all using the
   shared JSON error envelope. Update contracts/openapi.yaml in the same commit.

Constraints: never store code that failed validation; no execution of user code in Go; no backtest triggering
here; version numbers must be gap-free under concurrency.

Done when: version 1 is created with code/hash/params; identical resubmission is idempotent; editing creates v2
while v1 is byte-identical; direct SQL UPDATE on strategy_versions is rejected; `import os` returns 422 with
IMPORT_FORBIDDEN + line/col and persists nothing; two concurrent version creations yield v2 and v3 with no gaps;
responses validate against the OpenAPI schemas.
```

## Notes / risks

- Storing rejected code is tempting for debugging but creates a code-execution-adjacent audit surface; keep
  rejections out of the table and rely on logs.
- `ast_hash` must normalise (strip comments/docstrings/whitespace) or trivially-reformatted code will create
  duplicate versions.
