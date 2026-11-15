# SS-007 — Upstox OAuth2 Daily Token Lifecycle Service

| | |
| --- | --- |
| Milestone | 1 — Data layer |
| Depends on | SS-001 |
| Blocks | SS-008, SS-010 |
| Size | ~1.5 days |
| Stack | Go, PostgreSQL (token store), Upstox OAuth2 |

## Goal

Every Upstox call needs a valid access token that dies daily at ~03:30 IST. Own that lifecycle in one
service so no other package ever handles credentials.

## Scope

**In scope**
- `internal/upstox/auth`:
  - `URL()` → login URL with `client_id` + `redirect_uri`.
  - `Exchange(ctx, code) (Token, error)` → hits `POST /v2/login/authorization/token`.
  - Thread-safe `TokenSource` with `Token(ctx) (string, error)`; refreshes from DB, never from memory-only.
  - `Status() Status` → `valid | expiring | expired | missing` (used by `/readyz` and the UI banner).
- Persistence: `upstox_tokens` (access_token encrypted at rest, issued_at, expires_at, user_id).
- Scheduler: at **03:15 IST** mark stale and notify; a manual "reconnect" endpoint
  `GET /api/v1/admin/upstox/login` → redirect, `GET /api/v1/admin/upstox/callback?code=…` → exchange + store.
- Middleware `RequireUpstoxToken` returning `503 upstox_auth_required` with the login URL in `details`.
- Token health metric + alert log if `expiring` for > 1 hour and no action.

**Out of scope**
- Any market-data call (SS-008/SS-010).
- Multi-tenant broker accounts — single account, clearly documented.

## Small architecture

```text
Angular  --GET /admin/upstox/login--> 302 to api.upstox.com/v2/login/authorization/dialog
                                                │  (user logs in, approves)
        <--GET /admin/upstox/callback?code=…----┘
              │
              ├─ auth.Exchange(code)  → POST /v2/login/authorization/token (code, client_id, client_secret)
              ├─ upsert upstox_tokens(encrypted)
              └─ 302 back to /settings with ?status=connected
```

* `TokenSource.Token(ctx)` returns the stored token when `now < expires_at - 5m`; otherwise returns a
  typed `ErrTokenExpired` (never silently calls Upstox).
* Encryption: AES-256-GCM with key from `TOKEN_ENC_KEY`; the plaintext token never appears in logs — add a
  `redact` helper and a test asserting no token substring is logged.
* One goroutine owns refresh checks; all readers use `RLock`.
* Expiry stored in UTC; the UI shows IST. Daily expiry boundary is data, not hardcoded — derive from the
  JWT `exp` claim in the token response and store it.

## Tasks

- [ ] Migration `upstox_tokens` + repository.
- [ ] `URL`, `Exchange`, `TokenSource`, `Status` with tests against a `httptest` fake Upstox.
- [ ] Admin login/callback endpoints + redirect handling; `RequireUpstoxToken` middleware.
- [ ] 03:15 IST scheduler job + health log; `/readyz` surfaces `upstox: expired` as a warning (not a 503).
- [ ] Secrets wiring in `.env.example` (`UPSTOX_CLIENT_ID`, `UPSTOX_CLIENT_SECRET`, `UPSTOX_REDIRECT_URI`, `TOKEN_ENC_KEY`).
- [ ] ADR `docs/adr/0007-token-storage.md`.

## Acceptance criteria

1. With no token, any guarded endpoint returns `503` with `details.login_url` populated.
2. `GET /admin/upstox/callback?code=bad` returns `400` and does not store anything.
3. A stored token with `expires_at` in the past makes `Status()` return `expired` and `Token()` return
   `ErrTokenExpired`.
4. The access token string never appears in application logs (grep-based test on a captured log buffer).
5. Calling `Exchange` twice with the same code stores at most one active row (upsert, not duplicate).

## Test plan

- `httptest` fake Upstox server for success, 401, 500, timeout.
- Encryption round-trip test; corruption test returns a clean error.
- Concurrency test: 100 goroutines calling `Token()` with `-race`.

## AI prompt (copy-paste)

```text
Context: algorithmic-trading-platform.md §3.9 (Upstox token lifecycle),
docs/issues/SS-007-upstox-oauth.md. Repo: Go 1.23 + chi + pgx, config package reading env,
slog JSON logging, /healthz and /readyz exist, golang-migrate wired.

Implement SS-007 only: a self-contained Upstox OAuth2 lifecycle service in internal/upstox/auth.
Provide URL() building the /v2/login/authorization/dialog link, Exchange(ctx, code) calling
POST https://api.upstox.com/v2/login/authorization/token with form fields (code, client_id, client_secret,
redirect_uri, grant_type=authorization_code), a concurrency-safe TokenSource exposing Token(ctx) and
Status() (valid/expiring/expired/missing) where Token returns a typed ErrTokenExpired rather than calling
Upstox when stale, and a RequireUpstoxToken middleware returning 503 with the login URL in details.

Persist tokens in a new upstox_tokens table (access_token encrypted with AES-256-GCM using TOKEN_ENC_KEY,
issued_at, expires_at from the JWT exp claim, user_id) with an upsert so re-auth never duplicates rows.
Expose GET /api/v1/admin/upstox/login (302 to Upstox) and GET /api/v1/admin/upstox/callback?code=... which
exchanges, stores, and redirects to the frontend with a status flag. Add a scheduler that marks the token
stale at 03:15 IST and logs a health warning, and reflect upstox status in /readyz as a non-fatal warning.

Constraints: no market-data calls in this issue; credentials only via env; never log the token (add a redact
helper plus a test that greps a captured log buffer); expiry stored in UTC, displayed in IST.

Done when: guarded endpoint returns 503 with login_url when no token; bad code returns 400 and stores nothing;
expired token yields ErrTokenExpired; duplicate code exchange keeps a single row; -race test with 100
concurrent Token() calls passes; no token appears in logs.
```

## Notes / risks

- Upstox has no refresh token for this flow — the daily flow genuinely needs a human or an automated
  browser flow. Document the operational requirement in the runbook (SS-034) instead of pretending it's
  hands-free.
