# Strategy Street Issues & Milestones

This directory tracks platform issues and milestones.

## Milestones

- **Milestone 0: Foundations**
  - [SS-001: Monorepo Scaffold, Dev Environment, Config & Health](SS-001-monorepo-scaffold.md)
  - [SS-002: Migration Tool + Core Metadata Schema & Seeds](SS-002-metadata-schema.md)

- **Milestone 1: Data Layer & Market Streaming**
  - [SS-003: TimescaleDB Hypertables, Continuous Aggregates & Multi-Timeframe Candle Storage](SS-003-candle-hypertables.md)
  - [SS-004: Continuous Aggregates for 5m → 1Y + Retention/Compression](SS-004-continuous-aggregates.md)

## Repository Layout

- `api/` — Go 1.23 REST API and orchestrator
- `engine/` — Python 3.12 strategy and computation engine
- `web/` — Angular 18 frontend client
- `contracts/` — Shared schema contracts and protobuf definitions
- `db/migrations/` — Database migrations (managed via golang-migrate)
- `deploy/` — Docker Compose and local deployment manifests
- `docs/` — Architecture documentation, specifications, and issues
