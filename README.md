# Strategy Street

Algorithmic Trading, Multi-Timeframe Market Data Warehousing, Cross-Stock Backtesting, and Deterministic Paper-Trading Platform.

## Documentation & Architecture

- **Product Specification**: [algorithmic-trading-platform.md](algorithmic-trading-platform.md)
- **Issues & Roadmap**: [docs/issues/](docs/issues/README.md)
  - [SS-001: Monorepo Scaffold, Dev Environment, Config & Health](docs/issues/SS-001-monorepo-scaffold.md)

## Repository Structure

```
├── api/             # Go 1.23 REST API and orchestrator
│   ├── cmd/api/     # Service entrypoint
│   └── internal/    # Internal packages (config, httpx)
├── engine/          # Python 3.12 strategy engine package
│   ├── engine/      # Python source files
│   └── tests/       # Pytest unit tests
├── web/             # Angular 18 frontend application
├── db/              # Database schema migrations
│   └── migrations/  # golang-migrate SQL files
├── contracts/       # Shared interfaces, DTOs, schemas
├── deploy/          # Docker Compose configurations
├── Makefile         # Monorepo command orchestration
└── .env.example     # Environment variable documentation
```

## Quick Start

1. **Copy environment configuration:**
   ```bash
   cp .env.example .env
   ```

2. **Start the local stack:**
   ```bash
   make up
   ```

3. **Verify service health:**
   ```bash
   curl http://localhost:8080/healthz
   curl http://localhost:8080/readyz
   ```

4. **Run migrations:**
   ```bash
   make migrate
   ```

5. **Run tests and linters:**
   ```bash
   make test
   make lint
   ```
