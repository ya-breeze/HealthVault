# ADR-009: In-Memory, Per-Process Login Attempt Limiting

## Status
Accepted

## Context and Problem Statement

`POST /api/auth/login` has never had any rate limiting, lockout, or backoff — not in
`backend/pkg`, not in the shared `kin-core` library HealthVault's auth is built on. This was
tolerable while Cloudflare Access sat in front of every route, but the Android Widget (idea #12)
needs a Bypass policy on `/api/*` so a home-screen widget can call the API without an interactive
Access login, which would place `/api/auth/login` directly on the open internet. Login hardening
must land before that Bypass policy is safe to create. `kin-core/authdb` already has a DB-backed
token blacklist keyed by token string; should login-failure tracking live there too, or be
scoped locally to HealthVault?

## Decision Drivers

- This is the first rate-limiting/lockout mechanism anywhere in this codebase or in `kin-core` —
  there is no existing precedent to extend.
- The backend has no background job infrastructure — `ListenAndServe` is the only goroutine —
  ruling out a ticker-based cleanup process.
- This project runs as a single backend process for a handful of users (`/data/CLAUDE.md`'s scale
  guidance); distributed or shared-state rate limiting is not a real requirement here.
- Extending `kin-core/authdb` would require a schema/version bump to a shared library for a
  capability only HealthVault currently needs.
- A restart is a rare, operator-initiated event, not something an attacker can trigger on demand
  to reset their own lockout.

## Considered Options

- **DB-backed lockout via `kin-core/authdb`** (extend the existing token-blacklist table, or add a
  sibling table, to also track login failures) — durable across process restarts, but forces a
  `kin-core` schema/version bump for a capability nothing else in `kin-core` needs, and adds a
  cross-repo dependency to ship a single project's threat mitigation.
- **In-memory, per-process, HealthVault-local map** — a mutex-guarded, package-level map keyed by
  lowercased username, tracking a sliding-window failure count, an in-flight counter, backoff
  level, lockout expiry, and last activity; swept lazily (bounded, on every `admitAttempt` call)
  and capped at a hard size ceiling instead of run by a background goroutine. Loses lockout state
  on restart, but requires no new schema, no `kin-core` change, and fits the existing
  single-goroutine constraint.

## Decision Outcome

Chosen: **in-memory, per-process, HealthVault-local login attempt limiting**, implemented as a
sliding-window failure count (15-minute trailing window, 5-failure threshold) with exponential
backoff on lockout (1m/2m/4m/8m/16m, capped at 30m, doubling per lockout since the last success,
resetting to 1m after 24h with no failures). Concurrency safety and failure counting are kept
separate: `admitAttempt` atomically reserves an in-flight slot before credential verification
runs, and only a confirmed 401 (via `recordFailure`) ever advances the failure count or trips a
lockout — so a burst of concurrent *correct*-password requests can never trip a real lockout.
Because there is no background goroutine, stale entries are reclaimed lazily: `admitAttempt`
sweeps a bounded number of the oldest expired entries on every call, and a hard map-size ceiling
evicts the oldest expired entry to make room for a new one — never a live entry (active lockout,
nonzero failure count, or nonzero in-flight counter) — failing closed (429) if every entry is
still live when the ceiling is hit.

### Consequences

- **Restart clears all lockout state.** Accepted: a restart is already a rare, operator-initiated
  event, not an attacker-triggerable reset.
- **No IP tracking, per-username only.** The origin sits behind a Cloudflare Tunnel; trusting a
  client-supplied header for the real IP without trusted-proxy validation wiring (out of scope
  here) would be a spoofable input. Per-username is sufficient for the stated threat (unlimited
  credential guessing against one account).
- **Bounded map size, not unlimited growth.** An attacker submitting a distinct, never-repeated
  username per request cannot grow the map without bound, because the ceiling forces eviction of
  the oldest expired entry — or, if none are expired, fails the new request closed — rather than
  admitting it unrecorded.
- **No per-deployment tuning.** Threshold, window, and backoff schedule are Go constants, not env
  config — consistent with this project's scale guidance against complexity nothing has asked for.
- This is the precedent future rate-limiting/lockout decisions in this codebase or `kin-core`
  should reference, matching ADR-005/ADR-006/ADR-007's role for their own scoped decisions.
