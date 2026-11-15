# SS-023 — Backtest Results Persistence & Query API

| | |
| --- | --- |
| Milestone | 3 — Orchestration |
| Depends on | SS-022 |
| Blocks | SS-024, SS-032 |
| Size | ~2 days |
| Stack | Go, PostgreSQL |

## Goal

Store every job's metrics so the UI can answer "which sector did this strategy win in?" and "which strategy
wins on TCS?" without recomputing anything.

## Scope

**In scope**
- Tables per spec §5.3: `backtest_stock_metrics` (all metrics columns + `params_json`, `rules_version`,
  `cost_config_version`, `total_trades`, `win_rate_pct`, `net_pnl`, `return_pct`, `profit_factor`,
  `sharpe_ratio`, `sortino_ratio`, `calmar_ratio`, `max_drawdown_pct`, `expectancy`, `cagr`,
  `avg_holding_period`) and `backtest_sector_summary`.
- Artifact storage: `trades_uri`/`equity_uri` pointing at blob storage (local FS in dev, S3-compatible in prod)
  with size caps; `backtest_stock_metrics` never stores raw trades.
- Endpoints:
  - `GET /api/v1/backtests/{run_id}/results?sort=&order=&limit=&cursor=` (paginated, stable cursor).
  - `GET /api/v1/backtests/{run_id}/results/{symbol}` → metrics + presigned artifact URLs.
  - `GET /api/v1/backtests/{run_id}/summary` → totals, distribution stats, best/worst stocks.
  - `GET /api/v1/backtests/compare?run_ids=a,b,c` → side-by-side metric comparison for A/B of strategy
    versions.
- Idempotent write path `PUT`-style: `UpsertStockMetric(runID, stockID, metrics)` so worker retries never
  duplicate rows (unique `(backtest_run_id, stock_id)`).
- Indexes: `(backtest_run_id, sector_id)`, `(stock_id, timeframe, composite_score DESC)` placeholder for SS-024,
  `(backtest_run_id, net_pnl DESC)`.
- Result finalisation: when a run completes, a `finalise` step computes `backtest_sector_summary` rows (raw
  aggregates; scoring formula belongs to SS-024) and freezes the run.

**Out of scope**
- Composite sector scoring/leaderboards (SS-024). Charts (SS-032).

## Small architecture

```text
job result → POST internal /runs/{id}/metrics (worker callback) → upsert backtest_stock_metrics
run completes → finalise() → aggregate per sector → backtest_sector_summary → run immutable

GET /results → keyset pagination on (metric_value, stock_id) with an index per sortable column
```

* Cursor pagination: `cursor = base64(last_sort_value|last_stock_id)`, so large runs page in bounded time.
  `OFFSET` is banned (documented) because 500-row pages 3-deep already degrade.
* Sector summary aggregates only simple things (avg/median/count/best stock); anything weighted is SS-024.
* Artifacts are written by the engine to blob storage and referenced by URI; presigned URLs are minted on read
  with a short TTL so no trade data goes through the API.
* Metrics upsert is transactional per stock; a run's counters in SS-022 only advance after a successful upsert
  (otherwise progress lies).

## Tasks

- [ ] Migrations: both tables, unique constraints, indexes.
- [ ] `UpsertStockMetric` + `FinaliseRun` (idempotent) in the store.
- [ ] Query endpoints with keyset pagination + sorting whitelist (no arbitrary column injection).
- [ ] Blob storage abstraction + presign; size cap + content-type validation.
- [ ] Compare endpoint.
- [ ] Contract/OpenAPI updates + tests (pagination stability under concurrent inserts).

## Acceptance criteria

1. Replaying the same job result 3× leaves exactly one row per `(run, stock)` with identical values.
2. `GET /results?sort=sharpe_ratio&order=desc&limit=50` returns stable pages: walking all cursors yields every
   row exactly once, even while new rows are inserted.
3. Sorting by an unknown column returns `400` (whitelist enforced).
4. `summary` returns counts matching the row count, plus best/worst stock by net PnL.
5. Presigned artifact URLs expire and are never served with an unbounded TTL; requesting a missing artifact
   returns `404`.
6. `compare` with 3 runs returns aligned rows keyed by symbol with a `missing` marker where a run has no data.
7. Finalising a completed run twice produces the same sector summary rows (idempotent).

## Test plan

- Pagination test inserting rows between page fetches (assert no duplicates/skips).
- Idempotency tests for upsert and finalise.
- SQL plan check that the sort queries hit their indexes.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §5.3 (backtest_stock_metrics, backtest_sector_summary), §8, §9,
docs/issues/SS-023-results-api.md. Repo: Go api with pgx, SS-022 gives backtest_runs + job results stream,
sectors/stocks from SS-002, a blob-storage abstraction may be stubbed.

Implement SS-023 only: persistence and query API for backtest results.
1. Migrations for backtest_stock_metrics (backtest_run_id, stock_id, sector_id, timeframe, params_json,
   rules_version, cost_config_version, total_trades, win_rate_pct, net_pnl, return_pct, profit_factor,
   sharpe_ratio, sortino_ratio, calmar_ratio, max_drawdown_pct, expectancy, cagr, avg_holding_period,
   trades_uri, equity_uri, created_at, UNIQUE(backtest_run_id, stock_id)) and backtest_sector_summary
   (avg_win_rate_pct, avg_return_pct, median_sharpe, best_stock_id, trade_count). Add indexes for
   (backtest_run_id, net_pnl DESC), (backtest_run_id, sharpe_ratio DESC), (backtest_run_id, sector_id) and
   (stock_id, timeframe).
2. Internal write path UpsertStockMetric (idempotent on the unique constraint) and FinaliseRun which aggregates
   simple per-sector stats once and freezes the run. Progress counters may only advance after a successful
   upsert.
3. Public endpoints: GET /api/v1/backtests/{run_id}/results with sort whitelist + keyset (cursor) pagination,
   GET /results/{symbol}, GET /summary (counts, best/worst by net PnL), GET /backtests/compare?run_ids=a,b,c.
   OFFSET pagination is forbidden — implement opaque cursors encoding (sort_value, stock_id).
4. Artifacts: store trades/equity blobs in a small Storage interface (local FS default, S3-compatible optional),
   return short-TTL presigned URLs, cap size and validate content type; never return raw trades inline.
5. Update contracts/openapi.yaml and add contract tests.

Constraints: no sector composite scoring or leaderboard logic here (SS-024); no recomputation of metrics; all
list endpoints must be index-backed and bounded.

Done when: triple-replaying a job result yields one row; full cursor walk returns every row exactly once even
while inserting concurrently; unknown sort column gives 400; summary counts reconcile; presigned URLs expire
and missing artifacts 404; compare aligns rows by symbol with `missing` markers; finalise is idempotent. Include
EXPLAIN output for the two hottest sort queries.
```

## Notes / risks

- Storing 500 rows per run × many runs is fine; storing trades per stock is not — artifacts must stay outside
  Postgres or the DB will bloat within weeks.
- Freeze semantics matter: results for a finished run must never silently change, or the leaderboard becomes
  non-reproducible.
