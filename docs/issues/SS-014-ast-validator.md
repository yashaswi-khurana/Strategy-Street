# SS-014 — AST Validator & Look-Ahead Bias Linter

| | |
| --- | --- |
| Milestone | 2 — Python engine |
| Depends on | SS-013 |
| Blocks | SS-015, SS-021, SS-031 |
| Size | ~2 days |
| Stack | Python `ast`, `symtable` |

## Goal

Reject dangerous or dishonest strategy code **before** it ever executes, and return machine-readable
diagnostics the Angular editor can render inline.

## Scope

**In scope**
- `engine/sandbox/ast_validator.py`:
  - Import allowlist: `numpy`, `pandas`, `pandas_ta`, `math`, `statistics`, `scipy`, `sklearn`,
    `engine.sdk` — everything else is an error (including `os`, `sys`, `subprocess`, `socket`, `urllib`,
    `requests`, `shutil`, `importlib`, `pathlib`, `builtins`).
  - Forbidden names/calls: `open`, `eval`, `exec`, `compile`, `__import__`, `globals`, `locals`, `vars`,
    `getattr` with a non-literal name, dunder attribute access (`__class__`, `__globals__`, `__subclasses__`),
    `ctypes`, `pickle`, `marshal`, `input`.
  - Structural limits: no `while True` without a *provable* break, max AST depth 40, max 2 000 statements,
    no module-level side effects outside imports/class/`params`.
- Look-ahead linter:
  - Reject `.shift(-n)` with `n > 0`, `.iloc[i+1]`, negative slices, `[::-1]`, `df.tail(...)` as a source of
    signals, `.rolling(...).apply` that receives future rows, and `center=True` in rolling calls.
  - Require that `generate_signals` returns the same index length as the input.
- `engine/sandbox/rules.py`: one declarative rule table (id, severity, matcher, message, docs URL) — no
  scattered `if` chains, so rules are auditable and testable.
- Output: `list[Diagnostic]{rule_id, severity: error|warning, line, col, message, suggestion}` +
  `valid: bool`; JSON-schema'd and served by the API.
- CLI: `python -m engine.sandbox.ast_validator path/to/strategy.py --json`.

**Out of scope**
- Actually executing code (SS-015). Runtime enforcement (also SS-015) — this is static only.

## Small architecture

```text
source ──► parse (ast.parse, SyntaxError → diagnostic)
        ──► RuleEngine.run(tree)          # visitor per rule group: imports | names | structure | lookahead
        ──► {valid, diagnostics[], stats{nodes, depth, statements}}
```

* Each rule is a class with `visit_*` or a predicate over `(node, ctx)`; the engine runs all rules so a user
  sees every problem at once, not one-at-a-time.
* Bypass attempts the rules must cover in tests: `import os` inside a function, `__import__("os")`,
  `getattr(__builtins__, "eval")`, `eval` via lambda alias, `exec` in a conditional never taken,
  `while True: x = 1` (no break), `df["close"].shift(-1)` inside a helper function, `np.roll(close, -1)`.
* Warnings (non-blocking): `np.roll` with negative shift (suspicious but legitimate), fetching a full-history
  column then using `.max()` (subtle in vectorized mode), printing to stdout.
* Rules are versioned (`RULES_VERSION`) and the version is stored with each strategy version so a later rule
  tightening can be explained.

## Tasks

- [ ] `Rule`/`RuleEngine` framework + `Diagnostic` model + JSON schema.
- [ ] Import rules, forbidden-name/call rules, dunder/attribute-escape rules.
- [ ] Structural limits (depth, statements, `while True`, module-level side effects).
- [ ] Look-ahead rules (negative shift, negative slices, `center=True`, `np.roll` negative).
- [ ] CLI + `--json`; golden tests per rule; an adversarial test corpus directory
      (`engine/tests/ast/bypass/*.py`) each asserting the expected rule ID.
- [ ] ADR `docs/adr/0014-numeric-libraries-whitelist.md` (pandas_ta vs ta, sklearn inclusion).

## Acceptance criteria

1. The spec's default template validates with `valid=true`, zero errors.
2. `import os` at top level, inside a function, and via `__import__("os")` all yield
   `IMPORT_FORBIDDEN` with correct line/col.
3. `df["close"].shift(-1)` yields `LOOKAHEAD_SHIFT` (severity error); `.shift(1)` and `.shift(0)` are clean.
4. 40 adversarial fixtures produce their expected rule IDs — the full corpus passes in one pytest run.
5. Validation of a 5 000-line file completes in < 300 ms.
6. Diagnostics are stable JSON (schema-validated) and sorted by (line, col, rule_id).

## Test plan

- `pytest -k ast` over the adversarial corpus (each file has an `# EXPECT: RULE_ID` header).
- Property test: validator never raises on random syntax; always returns a `valid` boolean.
- Timing/perf test on a large generated file.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.1 (AST static analysis) and §3.6 (look-ahead bias prevention),
docs/issues/SS-014-ast-validator.md. Repo has engine/sdk from SS-013 (BaseStrategy, StrategyContractError).

Implement SS-014 only: engine/sandbox/ast_validator.py + rules.py. Static analysis ONLY — never execute the
analysed code.
1. Declarative rule framework: Rule(id, severity, description, matcher), RuleEngine that runs ALL rules and
   returns {valid: bool, diagnostics: [{rule_id, severity, line, col, message, suggestion}], stats}.
   Diagnostics sorted by (line, col, rule_id) and JSON-schema validated.
2. Import allowlist: numpy, pandas, pandas_ta, math, statistics, scipy, sklearn, engine.sdk. Everything else is
   IMPORT_FORBIDDEN, including imports nested inside functions and __import__("os") calls.
3. Forbidden calls/names: open, eval, exec, compile, __import__, globals, locals, vars, getattr with a
   non-literal argument, input, ctypes/pickle/marshal imports, dunder escapes (__class__, __globals__,
   __subclasses__, __bases__).
4. Structural limits: no provably-infinite `while True` (must contain a break), AST depth <= 40, <= 2000
   statements, no module-level side effects beyond imports/class definition/params assignment.
5. Look-ahead linter: error on .shift(-n) with n>0, negative integer .iloc indexing relative to the end for
   future rows, [::-1] reversal used before signals, rolling(center=True); warn on np.roll with negative shift
   and on printing. Reject strategies whose returned frame length differs from input.
6. Version the rule set (RULES_VERSION) and include it in output. Provide a CLI
   `python -m engine.sandbox.ast_validator file.py --json`.

Constraints: no execution, no imports of user modules, pure stdlib ast. Every rule must have a positive and a
negative test. Add an adversarial corpus under engine/tests/ast/bypass/ where each file declares `# EXPECT:
RULE_ID` and is run by pytest.

Done when: the spec template is valid with zero diagnostics; three different os-import bypasses all yield
IMPORT_FORBIDDEN with correct line/col; shift(-1) errors while shift(1)/shift(0) are clean; the 40-file corpus
passes; a 5000-line file validates in <300ms; JSON output matches the schema. Paste the corpus results table.
```

## Notes / risks

- Static analysis is necessary but not sufficient — SS-015 must still enforce at runtime. Say so explicitly in
  the UI copy so users don't over-trust the linter.
- Rules that are too strict will block legitimate pandas idioms; keep a documented escape hatch comment
  (`# noqa: LOOKAHEAD_SHIFT` with justification) that downgrades error→warning but is stored per version.
