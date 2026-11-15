# SS-001 — Monorepo Scaffold, Dev Environment, Config & Health

| | |
| --- | --- |
| Milestone | 0 — Foundations |
| Depends on | — |
| Blocks | everything |
| Size | ~2 days |
| Stack | Go 1.23, Python 3.12, Angular 18, Docker Compose |

## Goal

Give every later issue a place to land: a polyglot monorepo with one-command local boot, typed
configuration, structured logging, and a health endpoint. No business logic.

## Scope

**In scope**
- Directory skeleton from `docs/issues/README.md` §3 with a compiling stub in each language.
- `deploy/docker-compose.yml`: `timescaledb` (postgres 16 + timescale 2.x), `redis` 7, `engine` (Python), `api` (Go).
- `Makefile`: `make up`, `down`, `migrate`, `api`, `engine`, `web`, `test`, `lint`.
- Go: `internal/config` (env → typed struct, fail-fast validation), `internal/httpx` (JSON error envelope,
  request-id middleware, structured `slog` logger), `/healthz` + `/readyz`.
- Python: `pyproject.toml` (uv or pip-tools), package `engine`, `pytest` wired, `ruff` + `mypy` config.
- Angular: workspace + one placeholder route, `npm run build` green.
- CI: GitHub Actions running lint + unit tests per language (no DB integration yet).
- `.env.example` documenting every variable.

**Out of scope**
- Any domain table, any Upstox call, any strategy logic.
- Auth, TLS, production deployment manifests.

## Small architecture

```text
cmd/api/main.go
   ├── config.Load()        // env → Config, returns error on missing required
   ├── httpx.NewRouter()    // chi/gin router, middleware chain
   └── server.ListenAndServe on $API_ADDR (0.0.0.0:8080)
```

* `config.Config` is the **only** place env vars are read; every other package receives explicit values.
  Required: `DATABASE_URL`, `REDIS_URL`. Optional with defaults: `API_ADDR=0.0.0.0:8080`, `LOG_LEVEL=info`,
  `ENV=local`.
* Logging: `slog` JSON handler, `request_id` propagated through `context.Context` and echoed in every
  response header `X-Request-Id`.
* Error envelope shared by all future endpoints:

```json
{ "error": { "code": "validation_failed", "message": "…", "details": [ … ] }, "request_id": "…" }
```

* Compose healthchecks: Postgres `pg_isready`, Redis `redis-cli ping`; `api` depends_on both `service_healthy`.
* Timescale image must be pinned (e.g. `timescale/timescaledb:2.15.3-pg16`) — later issues depend on
  `time_bucket`, continuous aggregates and compression being available.

## Tasks

- [ ] Create directory skeleton + `.gitkeep`s; root `README.md` pointing at spec and `docs/issues`.
- [ ] Go module init, `internal/config`, `internal/httpx`, `cmd/api` with `/healthz`, `/readyz`.
- [ ] `docker-compose.yml` with timescaledb, redis, api, engine; named volume for pgdata.
- [ ] `db/migrations/` + `make migrate` using `golang-migrate` (empty `0001_init` is fine).
- [ ] Python package with `engine/__init__.py`, pytest smoke test, ruff/mypy config.
- [ ] Angular workspace with placeholder app; `web/proxy.conf.json` to `/api`.
- [ ] `.env.example`, `Makefile`, GitHub Actions workflow.

## Acceptance criteria

1. `cp .env.example .env && make up` brings up 4 healthy containers on a clean machine.
2. `curl localhost:8080/healthz` → `200 {"status":"ok"}`; `/readyz` fails (503) when Postgres is down.
3. A malformed `DATABASE_URL` makes the API exit non-zero with one clear log line.
4. `make test` runs Go + Python + Angular unit tests and all pass; `make lint` is clean.
5. Every log line is JSON and contains `request_id` for HTTP-triggered work.

## Test plan

- Go: table test for `config.Load` (missing/ok/partial), handler test for `/readyz` with a fake pinger.
- Python: trivial test to prove pytest + coverage wiring.
- CI: assert all three jobs run on push.

## AI prompt (copy-paste)

```text
You are bootstrapping a polyglot monorepo. Context files:
- algorithmic-trading-platform.md (the product spec; read §1 and §4.1 only)
- docs/issues/SS-001-monorepo-scaffold.md (this issue)

Build ONLY the foundation described in SS-001. Target layout (create it exactly):
api/ (Go 1.23: cmd/api, internal/config, internal/httpx), engine/ (Python 3.12 package `engine`),
web/ (Angular 18 workspace), db/migrations/, contracts/, deploy/docker-compose.yml, Makefile, .env.example.

Requirements:
1. Go: config.Load() reads DATABASE_URL, REDIS_URL, API_ADDR (default 0.0.0.0:8080), LOG_LEVEL, ENV and
   fails fast with a descriptive error. internal/httpx provides slog JSON logging, request-id middleware
   (X-Request-Id in/out), a JSON error envelope {"error":{"code","message","details"},"request_id"}, and
   a router exposing /healthz and /readyz (readyz pings Postgres and Redis, returns 503 when down).
2. docker-compose: timescale/timescaledb:2.15.3-pg16, redis:7, the Go api, the python engine, healthchecks,
   depends_on service_healthy, named pgdata volume. Bind API to 0.0.0.0.
3. Makefile targets: up, down, migrate (golang-migrate), api, engine, web, test, lint.
4. Python: pyproject with pytest, ruff, mypy; one smoke test. Angular: default app, builds successfully,
   dev proxy /api -> http://localhost:8080.
5. GitHub Actions: three jobs (go, python, node) running lint + tests.

Constraints: no domain logic, no Upstox code, no auth. Pin versions. Everything must build offline after
`make up` on a clean checkout.

Done when: `make up` yields healthy containers; `curl :8080/healthz` returns 200; `/readyz` returns 503 with
Postgres stopped; `make test` and `make lint` pass. Include the exact commands you ran to verify in the PR.
```

## Notes / risks

- Do not put `golang-migrate` in the API binary; run migrations as a separate job so pods stay read-only at boot.
- Choose `chi` or stdlib `net/http` over heavy frameworks — later WS work needs raw hijacking control.
