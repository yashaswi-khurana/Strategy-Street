# SS-024 — Sector Composite Scoring, Leaderboard & Best-Strategy-Per-Stock

| | |
| --- | --- |
| Milestone | 3 — Orchestration |
| Depends on | SS-023 |
| Blocks | SS-032 |
| Size | ~2 days |
| Stack | Go, PostgreSQL |

## Goal

Answer the two product questions in the spec: *which sector does this strategy excel in?* and *which strategy is
best for this stock?* — with a defensible, documented score rather than a raw average.

## Scope

**In scope**
- `internal/analytics/scoring`:
  - `SectorScore = 0.35*norm(avg_win_rate) + 0.35*norm(median_sharpe) + 0.30*norm(profit_factor) − penalty(max_drawdown)`
    exactly as spec §8.1, with the normalisation method made explicit:
    - Robust min-max **winsorised at p5/p95** across the sectors of the same run (so one outlier stock can't
      own the scale), fallback to global stats when < 5 sectors.
    - `penalty(max_drawdown) = 0.5 * norm(max_drawdown_pct)` floored at 0, documented as a 0–50 point deduction.
  - Deterministic tie-breaks (score, then median sharpe, then sector id).
- Recompute job: on run finalisation compute and persist `backtest_sector_summary.composite_score` + rank;
  store the normalisation parameters used in a `scoring_meta` JSONB column for reproducibility.
- Endpoints:
  - `GET /api/v1/analytics/sectors/{strategy_id}?timeframe=&run_id=` → per-sector rows (score, rank, win rate,
    median sharpe, PF, net pnl, trade count, best stock) + run metadata.
  - `GET /api/v1/analytics/stocks/{symbol}/best-strategies?timeframe=&limit=` → ranked strategies with
    `composite_score`, backtest metrics and (joined from SS-028) live paper win rate when available.
- Index `(stock_id, timeframe, composite_score DESC)` for the per-stock lookup; a materialised view
  `mv_best_strategy_per_stock` refreshed on run finalisation.
- Explanation payload: `score_breakdown {win_rate_component, sharpe_component, pf_component, drawdown_penalty,
  normalisation}` so the UI can show *why* a sector ranked first.

**Out of scope**
- Cross-strategy ranking across timeframes, portfolio construction, ML scoring.
- Rendering heatmaps (SS-032).

## Small architecture

```text
per run (finalised):
  rows = SELECT sector_id, avg(win_rate), median_??  -- median not built-in
       FROM backtest_stock_metrics WHERE run_id=$1 GROUP BY sector_id
  → percentile_cont(0.5) within group for median sharpe
  → winsorise + min-max per component → weighted sum − penalty → rank
  → UPDATE backtest_sector_summary SET composite_score, rank, scoring_meta
  → REFRESH MATERIALIZED VIEW mv_best_strategy_per_stock
```

* Median needs `percentile_cont(0.5) WITHIN GROUP (ORDER BY sharpe_ratio)`; don't fake it with `avg`.
* Stocks with metric flags (`no_trades`, `no_losses`) are excluded from normalisation but still listed, marked
  `insufficient_data`, so a 1-trade fluke can't top the leaderboard (min 20 trades configurable).
* Scores are **per run** (a strategy version + timeframe + period). Never compare scores across runs — the API
  returns the run metadata alongside and the UI labels it.
* All computations live in SQL + a thin Go layer over the same expression so the job and endpoint can't diverge;
  the formula constants live in one Go file exposed by a `GET /analytics/scoring-config` endpoint.

## Tasks

- [ ] Scoring package + SQL (normalisation, penalty, ranking) with unit tests on synthetic metric sets.
- [ ] Persist score/rank/`scoring_meta` during finalisation; refresh the MV.
- [ ] Two analytics endpoints + OpenAPI + score-breakdown payload.
- [ ] Minimum-trade filter and `insufficient_data` marking.
- [ ] Property tests: monotonicity (better win rate ⇒ not-lower score), boundedness (0–100), determinism.
- [ ] ADR `docs/adr/0024-sector-scoring.md` documenting normalisation, penalty and the minimum-sample rule.

## Acceptance criteria

1. Given a synthetic run with 5 sectors, the sector with the best combination ranks first, and the score matches
   a hand-computed value to 2 dp.
2. `score_breakdown` components sum (with the penalty subtracted) to the reported `composite_score`.
3. A sector containing a single 1-trade stock (below the 20-trade floor) is marked `insufficient_data` and cannot
   rank above a sector with ≥ 20 trades per stock.
4. Cross-strategy: `GET /analytics/stocks/TCS/best-strategies?timeframe=5m` returns strategies ordered by
   `composite_score` desc with a live win rate field present when a paper deployment exists.
5. Recomputing scores for the same run twice yields identical values and `scoring_meta` (determinism).
6. All scores are within [0, 100] across 1 000 randomly generated metric sets (property test).

## Test plan

- Golden synthetic run (fixture SQL) with expected ranks and scores.
- Property tests for bounds/monotonicity/determinism.
- Query plan check that the best-strategies endpoint uses the composite index.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §8.1 and §8.2 (sector composite score formula, best strategy per
stock), docs/issues/SS-024-sector-analytics.md. Repo: Go api + pgx; SS-023 provides backtest_stock_metrics and
backtest_sector_summary (already holding avg_win_rate_pct, avg_return_pct, median_sharpe, best_stock_id);
SS-028 will later expose paper win rates.

Implement SS-024 only: sector scoring and analytics endpoints.
1. Formula: SectorScore = 0.35*norm(avg_win_rate) + 0.35*norm(median_sharpe) + 0.30*norm(profit_factor)
   - 0.5*norm(max_drawdown_pct), clamped to [0,100]. Normalisation is winsorised min-max at p5/p95 across the
   sectors of the same run (fallback to global stats when fewer than 5 sectors). Median sharpe must use
   percentile_cont(0.5) WITHIN GROUP — never avg.
2. Persist composite_score, rank and a scoring_meta JSONB (normalisation params, constants, min-trade floor) on
   backtest_sector_summary during run finalisation; deterministic tie-breaks (score, median sharpe, sector id).
   Expose GET /analytics/scoring-config with the constants.
3. Minimum-sample rule: stocks with fewer than 20 trades (configurable) or flags no_trades/no_losses are
   excluded from normalisation and surfaced as insufficient_data; such sectors cannot rank above a
   statistically adequate one.
4. Endpoints: GET /api/v1/analytics/sectors/{strategy_id}?timeframe=&run_id= returning per-sector score, rank,
   metrics, best stock and a score_breakdown {win_rate_component, sharpe_component, pf_component,
   drawdown_penalty, normalisation}; GET /api/v1/analytics/stocks/{symbol}/best-strategies?timeframe=&limit=
   ordered by composite_score desc, including live paper win rate when a deployment exists. Add index
   (stock_id, timeframe, composite_score DESC) and a materialized view refreshed on finalisation.
5. Update contracts/openapi.yaml; document everything in ADR 0024.

Constraints: never compare scores across different runs (return run metadata and document); keep formula
constants in one place shared by the job and the endpoint; no recomputation on read.

Done when: a synthetic 5-sector run ranks correctly with the score matching a hand computation to 2dp; the
breakdown sums to the composite score; a 1-trade fluke sector is marked insufficient_data; best-strategies
ordering and the paper win rate join work; recomputation is deterministic; 1000 random metric sets stay within
[0,100]. Include EXPLAIN for the best-strategies query.
```

## Notes / risks

- Min-max normalisation makes scores relative, so "84.2" means nothing outside its run — label it in the API and
  UI, or users will compare numbers across periods.
- Winsorising is essential: without it, one stock with Sharpe 40 flattens every other sector's component.
