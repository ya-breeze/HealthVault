# Daily summary endpoint and login rate limiting

Idea: ya-breeze/idea-forge#12

## Why

Idea #12's Android widget needs one cheap, self-contained read to draw a home-screen calorie count —
not three or four separate authenticated calls per refresh. Nothing today aggregates "how am I doing
today": `GET /api/users/me/nutrition-target` computes the target alone, and consumed totals are not
exposed anywhere.

Separately, the widget's auth design requires a Cloudflare Access **Bypass** policy on `/api/*`, so a
background refresh can log in with username/password instead of an interactive Google flow. That
policy is zone config tracked outside this repo, and it cannot land until login can survive being
placed on the open internet. Today it cannot: there is no rate limiting, lockout, or backoff
anywhere in the backend or in the shared `kin-core` library. Access is currently the only thing
standing between `/api/auth/login` and unlimited attempts against all health data. This change is
that prerequisite — it does not add the Bypass policy.

## How

**`GET /api/summary/today`** is a new self-only endpoint (no `?user=` override, matching
`nutrition-target`'s precedent) returning in one response: today's consumed calories/protein/carbs/
fat summed from **confirmed meals only**, a raw count of today's logged meals in any status, the
timestamp of the most recent one, the caller's `display_language`, an embedded nutrition target *or*
a structured reason it is unavailable, and an always-null `recommendation` field reserved for Phase
4. "Today" reuses `food-day-completeness`'s existing Local Day boundary (the caller's stored
`timezone`, UTC fallback) rather than inventing a second definition of a day. The target is never
fabricated: when the profile is incomplete the response carries the same four structured reasons
`nutrition-target` already defines, so a client shows consumed calories alone rather than a
confidently wrong denominator.

**Login attempt limiting** is an in-process, per-username sliding-window counter with
exponential-backoff lockout on `POST /api/auth/login`, returning `429` with
`{"error": "too_many_attempts", "retry_after_seconds": n}` once tripped. Backoff runs
1m/2m/4m/8m/16m/30m-cap, doubling per lockout since the last successful login and resetting after
24h without failures. State is in-memory and process-local — acceptable at this project's
single-instance, few-user scale, and recorded as ADR-009; a DB-backed lockout via `kin-core/authdb`
was considered and rejected there.

Two details are worth stating because they are what the tests are mostly about. First, admission is
**atomic per username and separate from failure**: `admitAttempt` reserves an in-flight slot under
the map's single mutex, and only a confirmed 401 advances the failure count. bcrypt is slow enough
that many concurrent callers would otherwise all observe "not locked" before any resolved — so a
burst of concurrent *correct*-password requests may transiently hit a capacity rejection, but can
never trip a real lockout. An operational DB error (as opposed to "no such user") releases the
reserved slot without recording anything, so a transient outage cannot contribute toward locking out
a legitimate account. Second, the map is **bounded and fails closed**: sweeping happens on
`admitAttempt` rather than in a background goroutine, eviction targets only genuinely expired entries
(never one with an active lockout, a live failure window, or an in-flight attempt), and when every
entry is live at the ceiling a new username is rejected rather than admitted unrecorded. Evicting by
recency instead would let an attacker flush a targeted username's protection by flooding the map with
throwaway names; admitting-unrecorded would let them disable the limiter for every account they had
not yet touched.

Unknown usernames run `auth.VerifyPassword` against `auth.DummyHash` before their 401, so the
response is not measurably faster than a wrong-password 401 — that closes a username-enumeration
timing oracle. `Refresh`, `Logout`, and `RequireAuth` are untouched: a locked-out username's existing
tokens keep working.

**Out of scope:** the `healthvault-android` app itself, the path-scoped Access Bypass policy, and the
companion zone-level Cloudflare rate-limit rule — infra and other-repo work. Phase 4's recommendation
computation is modeled as nullable and always null here. `RequireAuth` gains no
`Authorization: Bearer` path: the pinned `kin-core` v0.1.0 validates the `access_token` cookie
exclusively, and the Android client is expected to keep a cookie jar. Adding real header support is a
follow-up decision for whoever starts `healthvault-android`, not a prerequisite for this change.

**Review correction — the ceiling was a login DoS.** As first written, an entry counted as
evictable only after 24h of quiet, and `recordSuccess` never touched `lastActivity`. Two
consequences compounded: a successful login left an entry that was *immediately* expired and swept,
so real accounts held no slot; and one failed login against each of 1000 throwaway usernames filled
the map with entries nothing could evict for a day, after which every username without an existing
entry was rejected with 429 before credential verification ran. 1000 cheap, trickle-able requests
denied login to everyone — on the endpoint this change exists to put on the open internet. Fail-closed
was chosen over admitting-unrecorded on the grounds that losing the limiter for untouched accounts
was worse; what that reasoning missed is that the alternative outcome is nobody logging in at all.

Eviction is now **graded by what the entry is protecting** rather than by age alone — expired, then
idle, then partial failure progress — oldest-first within a tier, while an active lockout and an
attempt still inside credential verification are never evicted at all. So a flood displaces its own
junk and stops there. Fail-closed still exists, but reaching it now means tripping and re-tripping
1000 separate lockouts at five full-cost verifications each, and it lasts only until those lockouts
expire. Sacrificing a lockout to admit a stranger was the alternative, and it would give back exactly
the targeted-guessing protection the limiter is for. The cost of those verifications is bounded by
the other half —

**a global cap on concurrent credential verifications.** The per-username limiter never bounded total
work: every unknown username runs a full-cost bcrypt compare before its 401 (which is what closes the
enumeration oracle), so cheap HTTP requests from distinct usernames bought unbounded server CPU. A
process-wide semaphore now bounds verifications in flight; over the bound returns the same 429 shape.
Per-username lockout answers targeted credential stuffing, the global cap answers volume, and neither
substitutes for the other. `Retry-After` accompanies every 429 so an unattended client — the widget
this is all for — can back off without parsing the body.

**Finishing note.** This change was implemented before OpenSpec was retired, so it originally lived
in `openspec/changes/daily-summary-and-login-rate-limiting/`. That tree was deleted from `main` in
#43 while this branch was in flight; the branch's copy went with it in the merge, and this file
replaces it. Two things surfaced in that merge: `releaseAttempt` had a method but no package-level
wrapper, which is what broke `make lint` and stalled the branch, and the ADR landed as a second
`ADR-008` because `ADR-008-bottom-clearance-as-a-css-token` had merged to `main` meanwhile — so it
is renumbered to ADR-009 here.

## Validation Commands

- `make lint`
- `make test`

### Task 1: Login attempt limiting

- [x] Add `backend/pkg/server/login_limiter.go`: mutex-guarded package-level map keyed by lowercased
      username, tracking confirmed failure timestamps in a 15-minute trailing window, an in-flight
      counter, backoff level, lockout expiry, and last activity
- [x] Implement `admitAttempt` / `recordFailure` / `recordSuccess` / `releaseAttempt` under that
      single mutex, with admission atomic and separate from confirmed failure
- [x] Implement the 1m/2m/4m/8m/16m/30m-cap backoff schedule, with a lockout trip clearing the
      failure count but not the backoff level, and a 24h idle reset
- [x] Sweep expired entries on `admitAttempt`, enforce the map-size ceiling by evicting only expired
      entries, and fail closed when every entry is live
- [x] Wire it into `Login` in `backend/pkg/server/auth.go`: reject before credential verification,
      run the dummy bcrypt compare on unknown usernames, release the slot on operational DB errors
- [x] Return `429` with `{"error": "too_many_attempts", "retry_after_seconds": n}` and no cookies
      for all three rejection paths
- [x] Add the unexported `afterAdmitHook` test seam so concurrency tests can pin in-flight attempts
- [x] Mark completed

### Task 2: Login limiting tests

- [x] Cover the fifth-failure lockout trip, rejection of a correct sixth attempt, and reset on success
- [x] Cover backoff escalation across two lockouts, the 24h reset, and case-insensitive matching
- [x] Cover that `Refresh`, `Logout`, and `RequireAuth` requests are unaffected by a lockout
- [x] Cover the trailing window: a failure older than 15 minutes does not count toward the threshold
- [x] Cover the bounded map: throwaway-username flooding cannot evict a live targeted entry, the
      ceiling fails closed, and an in-flight entry is never evicted
- [x] Cover concurrency: correct-password bursts never trip a real lockout, and the dummy compare
      runs for unknown usernames
- [x] Mark completed

### Task 3: Daily summary computation

- [x] Extract `nutrition_target.go`'s precondition checks and computation into a shared helper
- [x] Add `database.TodaySummary`: confirmed-only macro sums, all-status meal count and last-updated,
      over the Local Day boundary from `food_completeness.go`
- [x] Add `backend/pkg/server/summary_today.go` with the handler and response types, embedding either
      the target or its structured unavailability reason
- [x] Register `GET /api/summary/today` behind the existing auth middleware in `server.go`
- [x] Mark completed

### Task 4: Daily summary tests

- [x] Cover confirmed-only sums against non-confirmed meals in every other status
- [x] Cover target embedding for an available target and for each unavailability reason
- [x] Cover that `?user=` is ignored (self-only)
- [x] Cover the Local Day boundary either side of local midnight in a non-UTC timezone
- [x] Mark completed

### Task 5: Documentation and verification

- [x] Record the in-memory, per-process lockout decision as an ADR, `Accepted` on merge
- [x] Fold the review's corrections into it: graded eviction, and the global verification cap
- [x] Renumber it to ADR-009 after `ADR-008-bottom-clearance-as-a-css-token` merged to `main` first
- [x] Replace the retired `openspec/changes/` entry with this file
- [x] `make lint` and `make test` pass on the tree merged with `main`
- [x] Mark completed

### Task 6: Review corrections

- [x] Set `lastActivity` on success, so an account that logs in normally holds its slot
- [x] Replace age-only eviction with the graded tiers, oldest-first within a tier, never evicting an
      entry with an attempt in flight
- [x] Bound concurrent credential verifications process-wide, releasing the reserved in-flight slot
      when the bound rejects an attempt
- [x] Send `Retry-After` alongside every 429 body
- [x] Surface the lockout on the login page — retry time from the response, never "Invalid
      credentials"
- [x] Read the caller's settings once per `/api/summary/today` request instead of three times
- [x] Cover the resolved-failure flood: it must not deny a new username, and must not strip a lockout
      before its own junk
- [x] Cover the global cap: attempts beyond it are rejected with the 429 shape and release their slot
- [x] Mark completed
