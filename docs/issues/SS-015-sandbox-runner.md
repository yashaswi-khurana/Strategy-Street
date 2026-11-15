# SS-015 — Sandboxed Ephemeral Strategy Runner

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-014 |
| Blocks | SS-020, SS-022 |
| Size | ~2.5 days |
| Stack | nsjail or Docker + gVisor, Python 3.12 |

## Goal

Run untrusted user Python with hard CPU/memory/time/network/FS limits so a pasted script can never damage the
host, and make failures look like data (a structured error) rather than a hung process.

## Scope

**In scope**
- `engine/sandbox/runner.py`:
  - `run_strategy(source: str, job: JobPayload, limits: Limits) -> Result` — writes code to a temp dir,
    launches an isolated process, feeds input, collects JSON result.
  - Backends: `local` (subprocess with rlimits + `seccomp` where available, dev only), `docker`
    (network none, read-only rootfs, `--memory 512m`, `--cpus 1`, `--pids-limit 64`, tmpfs for scratch),
    `nsjail` (production default: `--disable_clone_newnet`, `--rlimit_as`, `--time_limit 30`,
    `--seccomp_string`).
  - Hard limits: 5 s CPU per stock backtest, 512 MB RSS, 30 s wall, 64 PIDs, no network, no write outside
    `/tmp`.
  - Determinism: `PYTHONHASHSEED=0`, `numpy.random.seed(0)`, `random.seed(0)`, fixed TZ (`Asia/Kolkata`),
    no `time.time()` dependence exposed through the SDK.
  - Structured outcome: `{status: ok|timeout|crashed|invalid, stdout_tail, error{type,message,traceback_sanitised},
    result_json, usage{cpu_ms, max_rss_kb}}` — never raises on user failure.
  - Artifact capture: the produced signals/trades JSON is size-capped (e.g. 20 MB) and schema-validated.
- `engine/sandbox/protocol.py` — the stdin/stdout JSON envelope between parent and sandboxed child (a tiny
  `__main__` shim inside the sandbox; the parent never imports user code).
- Timeouts kill the whole process group (no orphaned children).

**Out of scope**
- Job queues (SS-020), actual backtest math (SS-018/019).

## Small architecture

```text
runner.run(source, job)
  ├─ write /tmp/job-<uuid>/{strategy.py, payload.json}   (0700)
  ├─ spawn backend (docker run --rm --network none --memory 512m --cpus 1 --pids-limit 64 …)
  │     └─ child: read payload → import strategy via importlib → call SDK entry →
  │               write result.json (or error) → exit
  ├─ wait with wall-clock cap → on timeout: SIGKILL process group, status=timeout
  └─ parse result.json, validate schema + size, return Result
```

* The parent **never** `import`s user code — isolation is process-level plus OS-level controls; the in-process
  `local` backend exists only for fast tests and must be impossible to enable in production
  (`ENV=prod` + `ALLOW_LOCAL_SANDBOX=true` → refuse to boot).
* The child runs as an unprivileged UID with a read-only rootfs and no DNS; a self-test job asserting
  `socket.socket()` fails is part of CI.
* Limits are a struct with defaults from config, overridable per job (SS-020 passes them).
* Resource accounting is reported back so backtest jobs can be billed/limited per user.

## Tasks

- [ ] `protocol.py` + child shim (`python -m engine.sandbox.child`).
- [ ] Backends: `local` (rlimit-based), `docker`, `nsjail`; a `--backend` flag and config wiring.
- [ ] Timeout/kill logic with process-group handling and orphan cleanup.
- [ ] Result/error normalisation + schema validation + traceback sanitisation (strip host paths).
- [ ] Dockerfile for the sandbox image + `deploy/sandbox/` config; `make sandbox-image`.
- [ ] Tests: infinite loop, memory bomb, `os.system` attempt, `socket` attempt, file write outside tmp,
      huge output, crash, syntax error, valid strategy → all produce the right `status`.
- [ ] ADR `docs/adr/0015-sandbox-isolation.md` (nsjail vs gVisor decision + threat model).

## Acceptance criteria

1. `while True: pass` returns `status=timeout` in ≤ 6 s CPU (not a hang) and leaves no orphan processes.
2. Allocating 2 GB returns `status=crashed` with an OOM indication, host memory unaffected.
3. `socket.socket()` and `open("/etc/passwd")` inside the sandbox fail and surface as a clean error, proving
   network/FS isolation in the chosen backend.
4. A valid strategy returns `status=ok` with schema-valid JSON and `usage.cpu_ms` populated.
5. 1 000 bytes of abusive stdout does not corrupt the JSON envelope (stdout is captured separately from the
   result channel).
6. With `ENV=prod` and `ALLOW_LOCAL_SANDBOX=true`, the engine refuses to start.

## Test plan

- Adversarial pytest matrix (one test per row of the table above), tagged `sandbox` (skipped if Docker absent).
- Leak test: run 50 jobs sequentially, assert no zombie processes and stable RSS.
- Timing test asserting the CPU limit, not just wall clock.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.1 (Ephemeral Container / Worker Sandboxing: gVisor/nsjail,
seccomp, 5s CPU, 512MB), docs/issues/SS-015-sandbox-runner.md. Repo has engine/sdk (SS-013) and
engine/sandbox/ast_validator (SS-014).

Implement SS-015 only: engine/sandbox/runner.py, protocol.py and a child shim.
1. Parent API: run_strategy(source: str, job: dict, limits: Limits) -> Result. The parent must NEVER import or
   exec user code; it writes strategy.py + payload.json to a 0700 temp dir and spawns an isolated child.
2. Backends: `local` (subprocess with RLIMIT_CPU/RLIMIT_AS and process-group kill, dev/test only, refused when
   ENV=prod), `docker` (--rm --network none --read-only --memory 512m --cpus 1 --pids-limit 64 --tmpfs /tmp),
   and `nsjail` (production default with --disable_clone_newnet, rlimits, time_limit, seccomp policy).
   Limits: 5s CPU, 512MB RSS, 30s wall, 64 pids, no network, no writes outside /tmp.
3. Determinism: PYTHONHASHSEED=0, seeded random/numpy, TZ=Asia/Kolkata, fixed locale.
4. Result contract (never raises on user failure): {status: ok|timeout|crashed|invalid,
   result_json, error{type,message,traceback_sanitised}, stdout_tail (<=8KB), usage{cpu_ms,max_rss_kb}}.
   Sanitise tracebacks to remove host paths. Cap result_json at 20MB and schema-validate it.
5. Kill the entire process group on timeout so no orphans survive; report both CPU and wall time.
6. Ship a Dockerfile for the sandbox image, `make sandbox-image`, and a config-driven backend selector.

Constraints: no job queue, no backtest math, no DB access from the sandbox; child runs as unprivileged UID with
read-only rootfs; stdout must never be mixed with the result channel.

Done when this adversarial matrix passes: infinite loop -> timeout <=6s with no orphans; 2GB allocation ->
crashed/OOM with host unaffected; socket.socket() -> clean failure; open("/etc/passwd") -> denied; valid
strategy -> ok with usage populated; 1KB of noisy stdout -> envelope still parses; syntax error -> invalid;
prod boot with local backend -> refuses to start. 50 sequential jobs leak neither processes nor memory.
Document the threat model in ADR 0015.
```

## Notes / risks

- If nsjail isn't available in the deployment environment, gVisor (`runsc`) is the fallback; the interface must
  not change.
- pandas import time (~0.4 s) eats into the 5 s CPU budget — allow a pre-warmed process pool in SS-020 rather
  than loosening limits.
