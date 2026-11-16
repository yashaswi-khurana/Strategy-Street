# Strategy Street Issues & Milestones

This directory tracks platform issues and milestones.

## Milestones

- **Milestone 0: Foundations**
  - [SS-001: Monorepo Scaffold, Dev Environment, Config & Health](SS-001-monorepo-scaffold.md)

## Repository Layout

- `api/` — Go 1.23 REST API and orchestrator
- `engine/` — Python 3.12 strategy and computation engine
- `web/` — Angular 18 frontend client
- `contracts/` — Shared schema contracts and protobuf definitions
- `db/migrations/` — Database migrations (managed via golang-migrate)
- `deploy/` — Docker Compose and local deployment manifests
- `docs/` — Architecture documentation, specifications, and issues
