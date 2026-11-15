# SS-034 — Observability, Load Tests, Demo Dataset & Runbook

| | |
| --- | --- |
| Milestone | 6 — Hardening |
| Depends on | all |
| Blocks | release |
| Size | ~3 days |
| Stack | Prometheus, Grafana, k6, GitHub Actions |

## Goal

Make the platform operable: you can see whether it's healthy, prove it survives a market-open burst, demo it
without a live Upstox account, and hand a new engineer a runbook that answers "what do I do when X breaks?".

## Scope

**In scope**
- Metrics: one `/metrics` per service (Go api/streamer/backfiller/paperd, Python worker) with a documented
  catalogue: feed connectivity/reconnects, tick lag p50/p95, bars closed vs expected per session, backfill queue
  depth + 2 RPS utilisation, candle write latency, job queue depth/failures, paper fills/skips/timeouts, risk
  breaches, WS clients/dropped frames, DB pool saturation.
- Dashboards (Grafana JSON committed): "Ingestion", "Backtests", "Paper trading", "Platform health" — each with
  one row of golden signals and alert thresholds annotated.
- Alerts (Prometheus rules): token expired, feed disconnected > 2 min in session, bars-missing > 5% of expected
  for the last 15 min, backfill stalled, job failure rate > 10%, paper determinism replay failure, DB disk > 80%.
- Load tests (k6 + Go bench): 50 concurrent chart clients on one symbol + 20 active paper deployments + a
  500-stock backtest run; publish p95 latency and resource numbers, with a pass/fail budget in CI (nightly).
- Demo dataset generator: `make seed-demo` → 20 stocks × 1 year of synthetic 1m bars (session-aware, realistic
  OHLCV with regime changes), 3 sample strategies, 1 completed backtest run, 2 paper deployments with history.
  Must be deterministic (seeded) and fast (< 2 min).
- Runbook `docs/runbook.md`: token re-auth at 03:30 IST, feed outage, backfill stall, Postgres disk full,
  Timescale compression job failure, migration rollback, paper divergence response (freeze → replay → diff),
  kill-switch incident, on-call escalation.
- Ops hygiene: DB backup/restore drill script, `make doctor` (checks ports, versions, token status, migrations
  pending), structured log retention, and a health page in the UI (`/status`) showing subsystem states.

**Out of scope**
- Multi-region deployment, HA Postgres, real-money execution.

## Small architecture

```text
services ──/metrics──► Prometheus ──rules──► Alertmanager ──► Slack
                                     └──► Grafana (committed dashboards)

make doctor        → config + token + migrations + ports + feed handshake (read-only checks)
make seed-demo     → deterministic generator → candles + strategies + runs + deployments
k6 scenarios       → ws_fanout, paper_load, backtest_burst   (budgets enforced)
```

* Every alert must link to a runbook anchor; an alert without a runbook entry fails review.
* The demo generator writes through the same repositories/bulk paths as production (no back-door SQL) so it also
  serves as an integration test.
* Budgets are explicit numbers, e.g. chart WS fan-out p95 < 50 ms, backtest job throughput ≥ 20 stocks/min/worker,
  paper bar-close → ledger write p95 < 400 ms, API p95 < 150 ms at 50 rps.

## Tasks

- [ ] Metrics catalogue + `/metrics` endpoints + a doc table of every metric.
- [ ] Grafana dashboards (JSON) + Prometheus alert rules with runbook links.
- [ ] `make doctor` and `/status` UI page.
- [ ] `make seed-demo` deterministic generator + sample strategies/backtest/deployments.
- [ ] k6 scenarios (WS fan-out, paper load, backtest burst) with CI budgets.
- [ ] `docs/runbook.md` with all incident playbooks + backup/restore drill.
- [ ] Final docs pass: README quickstart, architecture diagram, ADR index.

## Acceptance criteria

1. `make doctor` on a fresh machine prints a pass/fail table covering config, migrations, token status, ports, and
   Redis/DB connectivity, exiting non-zero on any failure.
2. `make seed-demo` twice produces identical row counts and identical metrics for the seeded backtest (seeded RNG).
3. Grafana dashboards load from the repo with no manual edits and every panel has data after seeding.
4. Firing a synthetic feed disconnect triggers the feed alert within 2 minutes and the alert description links to
   the correct runbook section.
5. k6 WS fan-out with 50 concurrent clients holds p95 < 50 ms and 0 dropped frames; the paper-load scenario keeps
   bar-close → ledger p95 < 400 ms; both are enforced as nightly CI budgets.
6. A full backtest of 20 stocks × 1 year of 1m data completes in the demo environment and the resulting metrics
   reproduce on a second run (determinism).
7. The backup/restore drill restores a database into a fresh container with candle counts and a paper ledger
   matching exactly.
8. `docs/runbook.md` covers all 7 listed incidents, each with symptoms → diagnosis commands → remediation →
   verification.

## Test plan

- CI nightly: k6 scenarios + a determinism replay of the demo backtest.
- Alert rule unit tests (`promtool test rules`).
- Restore drill scripted end-to-end.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md (whole spec; this issue operationalises it),
docs/issues/SS-034-observability-runbook.md. Repo: Go services (api, streamer, backfiller, paperd), Python engine,
Angular web, TimescaleDB, Redis, docker-compose; observability conventions: prometheus client + slog JSON logs.

Implement SS-034 only: operability.
1. Add /metrics to every service and document a metric catalogue (feed connectivity/reconnects, tick lag p50/p95,
   bars closed vs expected per session, backfill queue depth and 2 RPS utilisation, candle write latency, job queue
   depth/failures, paper fills/skips/timeouts, risk breaches, WS clients/dropped frames, DB pool saturation).
2. Commit Grafana dashboard JSON (Ingestion, Backtests, Paper trading, Platform health) and Prometheus alert rules
   (token expired, feed down >2min in session, >5% missing bars in the last 15min, backfill stalled, job failure
   rate >10%, determinism replay failure, DB disk >80%). Every alert description must link to a runbook anchor; add
   promtool unit tests.
3. `make doctor`: read-only pass/fail table for config, migrations pending, Upstox token status, ports, Redis/DB
   connectivity; non-zero exit on failure. Add a /status UI page showing subsystem health.
4. `make seed-demo`: deterministic (seeded) generator producing 20 stocks x 1 year of session-aware synthetic 1m
   bars through the normal repositories, 3 sample strategies, 1 completed backtest run and 2 paper deployments with
   history; completes in under 2 minutes and is reproducible run-to-run.
5. k6 scenarios ws_fanout (50 concurrent chart clients), paper_load (20 deployments), backtest_burst (500-stock run
   with a stub engine) with explicit CI budgets: WS p95 <50ms and zero drops, paper bar-close->ledger p95 <400ms,
   API p95 <150ms at 50rps, backtest >=20 stocks/min/worker. Wire into nightly CI.
6. docs/runbook.md with playbooks for: 03:30 IST token re-auth, feed outage, backfill stall, Postgres disk full,
   compression job failure, migration rollback, paper determinism divergence (freeze -> replay -> diff), kill-switch
   incident, escalation. Include a scripted backup/restore drill.
7. Final docs pass: README quickstart, architecture diagram, ADR index.

Constraints: no multi-region/HA work; demo generation must use production code paths; no alert without a runbook
entry; no PII/credentials in dashboards or logs.

Done when: `make doctor` gives a clean pass/fail table and exits non-zero on failure; seed-demo is reproducible and
fast; dashboards load from the repo with data; a synthetic feed disconnect fires the alert in <2min with the right
runbook link; k6 budgets are met and enforced nightly; the demo backtest reproduces metric-for-metric; the
restore drill matches candle counts and paper ledger exactly; all 7 playbooks exist with commands. Include the k6
summary and the alert test output.
```

## Notes / risks

- Observability added last always becomes observability added never — if any earlier issue ships metrics ad hoc,
  reconcile them into this catalogue rather than duplicating.
- The determinism replay alert is the most valuable one; treat it as paging, because it invalidates every paper
  PnL number on the dashboard.
