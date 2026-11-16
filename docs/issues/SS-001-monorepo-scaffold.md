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
- Directory skeleton from `docs/issues/README.md` with a compiling stub in each language.
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
