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
evicts one entry to make room for a new one, **graded by what that entry is protecting** — expired
first, then idle, then partial failure progress, oldest-first within each — failing closed (429)
only when every entry holds an active lockout or an attempt still inside credential verification.

Alongside it, a **process-wide cap on concurrent credential verifications**. The per-username
limiter bounds one account's attempts and nothing else: every unknown username runs a full-cost
bcrypt compare before its 401 (which is what closes the enumeration oracle), so cheap requests
across distinct usernames bought unbounded CPU. The two answer different attacks — per-username
lockout answers targeted credential stuffing, the global cap answers volume — and neither
substitutes for the other.

> **Correction (review of the implementing PR).** As first written, eviction ranked entries by age
> alone and `recordSuccess` never updated `lastActivity`. Together those made the ceiling a login
> denial-of-service: a successful login left an entry that was immediately expired and swept, so no
> real account was ever in the map, and one failed attempt against each of 1000 throwaway usernames
> filled it with entries nothing could evict for 24 hours — after which every username without an
> entry was refused before verification. Roughly 1000 cheap, trickle-able requests to stop everyone
> logging in. The reasoning that picked fail-closed over admitting-unrecorded weighed it only
> against losing the limiter for untouched accounts, and never against nobody logging in at all.

### Consequences

- **Restart clears all lockout state.** Accepted: a restart is already a rare, operator-initiated
  event, not an attacker-triggerable reset.
- **No IP tracking, per-username only.** The origin sits behind a Cloudflare Tunnel; trusting a
  client-supplied header for the real IP without trusted-proxy validation wiring (out of scope
  here) would be a spoofable input. Per-username is sufficient for the stated threat (unlimited
  credential guessing against one account).
- **Bounded map size, not unlimited growth.** An attacker submitting a distinct, never-repeated
  username per request cannot grow the map without bound: the ceiling forces eviction of the most
  disposable entry rather than admitting the attempt unrecorded.
- **An active lockout is never evicted, and the fail-closed path survives because of that.**
  Sacrificing a lockout to admit a stranger would hand back precisely the targeted-guessing
  protection this exists to provide. So fail-closed remains reachable — but only by tripping and
  re-tripping 1000 separate lockouts, five full-cost verifications each, throttled by the global
  cap, and only until those lockouts expire on their own. That is a different order of attack from
  the 1000 cheap requests the correction above describes.
- **The global cap can reject a legitimate login under load.** It returns the same 429 shape with a
  1-second retry hint, and at 8 concurrent verifications against a handful of users the bound is
  reached only under an attack or a thundering herd, both of which are the cases it exists for.
- **No per-deployment tuning.** Threshold, window, and backoff schedule are Go constants, not env
  config — consistent with this project's scale guidance against complexity nothing has asked for.
- This is the precedent future rate-limiting/lockout decisions in this codebase or `kin-core`
  should reference, matching ADR-005/ADR-006/ADR-007's role for their own scoped decisions.
