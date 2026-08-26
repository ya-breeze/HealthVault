## Context

This is the backend prerequisite named in the idea-forge grilling decision for idea #12 (Android
Widget): "Login hardening must land before the bypass." The investigation attempt that preceded
this proposal (see the idea-forge plan file's Task 1 findings) confirmed there is genuinely no
rate limiting, lockout, or backoff anywhere reachable from HealthVault — not in `backend/pkg`, not
in the shared `kin-core` library its auth is built on — and identified the closest existing
templates to build the new endpoint from: `summaryHandler` (`backend/pkg/server/api.go`, the
route-ordering precedent) and `NutritionTargetHandler` (`backend/pkg/server/nutrition_target.go`,
the self-only / structured-422 precedent).

Two independent pieces of work are bundled into one change because they share a merge point (both
must land before the Cloudflare Bypass policy is safe to create) but touch unrelated code paths;
they get their own top-level design sections below rather than a shared narrative.

## Goals / Non-Goals

**Goals:**
- Give the Android Widget (and any other future thin client) one authenticated call that returns
  everything needed to draw "calories consumed / target, updated Xh ago."
- Make `POST /api/auth/login` survive being placed on the open internet without Access in front of
  it, using a mechanism simple enough to match this project's "no background job infrastructure,
  `ListenAndServe` is the only goroutine" constraint (`openspec/config.yaml`).

**Non-Goals:**
- Building the Android app, the Cloudflare Bypass policy, or the zone rate-limit rule — all out of
  this repo's scope (see proposal.md).
- Distributed/shared-state rate limiting. This project runs as a single backend process for a
  handful of users (`/data/CLAUDE.md`'s scale guidance); an in-memory, per-process limiter is
  correct here and would need revisiting only if the backend were ever horizontally scaled, which
  nothing in this project's deployment model does today.
- Computing Phase 4's `recommendation`. It ships as a nullable field, always null, in this change.

## Decisions

### 1. `GET /api/summary/today` response shape

Self-only, authenticated, no request body — same shape of decision as `NutritionTargetHandler`
(design.md precedent already established there). Response:

```json
{
  "date": "2026-08-26",
  "calories_consumed": 1450,
  "protein_grams_consumed": 80,
  "carbs_grams_consumed": 120,
  "fat_grams_consumed": 45,
  "meal_count": 3,
  "last_logged_at": "2026-08-26T14:32:00Z",
  "display_language": "en",
  "target": {
    "available": true,
    "calories": 2400,
    "protein_grams": 150,
    "carbs_grams": 250,
    "fat_grams": 90
  },
  "recommendation": null
}
```

When the target is unavailable, `target` is instead `{"available": false, "reason":
"missing_profile"}` (one of `nutrition-target`'s existing four reason codes:
`missing_profile`, `missing_measurements`, `missing_goal_weight`, `insufficient_activity_data`).
The endpoint itself always returns HTTP 200 when the caller is authenticated — an unavailable
target is a normal, expected state (a fresh user with no profile yet), not an error on *this*
endpoint. This is the concrete implementation of the plan's "never invent a target... show
consumed calories alone rather than a confidently wrong denominator": the widget can always render
`calories_consumed`, and separately decides whether to render a target/progress bar based on
`target.available`.

**Reuse, not reimplementation, of the target computation.** `nutrition_target.go`'s precondition
checks (`readUserProfile`, `latestPointValue` for weight/height/weight_goal, `resolveActivityTier`)
and `computeNutritionTarget` are extracted into a single function, e.g.
`computeUserNutritionTarget(storage, userID, now) (target nutritionTargetValues, unavailableReason
string, err error)`, called by both `NutritionTargetHandler` (which 422s on a non-empty
`unavailableReason`, unchanged behavior) and the new summary handler (which embeds it either way).
No behavioral change to `GET /api/users/me/nutrition-target` itself — its spec requirements are
untouched by this proposal.

**"Today" is `food-day-completeness`'s Local Day, not a new definition.** The caller's stored
`timezone` setting resolves via the existing `database.ResolveTimezone(settingsJSON)` (UTC
fallback for missing/invalid values, per that capability's existing "Local Day boundary"
requirement), and "today" is `database.LocalDate(time.Now(), loc)`. A second, backend-local
definition of "day" would be exactly the inconsistency `food-day-completeness`'s design.md flagged
as a known, deliberately-not-fixed gap elsewhere in the codebase (general `/api/data/{type}`
bucketing) — this endpoint has no reason to introduce a third one.

**Consumed totals sum `confirmed` meals only.** `FoodMeal.Calories`/`ProteinGrams`/`CarbsGrams`/
`FatGrams` are populated by `Aggregate()` only on confirm (see `models_food.go`); a `processing`,
`pending_review`, `pending_clarification`, or `failed` row's macro fields are zero or stale.
Summing across all statuses would silently show a partial or zero-inflated number whenever a meal
is still mid-recognition — worse than the omission it would be trying to avoid.

**`meal_count` is a raw row count, not an Eating Occasion count.** `food-day-completeness`'s
10-minute occasion-collapsing exists specifically to keep `usual_meals_per_day` comparisons honest
against a single sitting logged as 2-3 rows; reusing it here would silently couple the widget's
"how many times did I log today" number to that unrelated policy, and collapsing requires an extra
query this endpoint doesn't otherwise need. `meal_count` counts every `FoodMeal` row (any status)
whose Logged Day is today — a raw activity signal, matching "meal count" as named in the idea's
comment, not the day-completeness capability's derived metric.

**`last_logged_at` is the max `LoggedAt` among today's rows (any status), or `null` if none.** A
`processing` row still means "the user just took a photo" — that's activity worth reflecting in
"updated Xh ago" even before recognition finishes, matching the plan's requirement that a
`~60s follow-up` refresh after returning from the Custom Tab has something to observe.

**Named `last_logged_at`, not `last_updated`.** It tracks the most recent *logging* activity
(`LoggedAt`), not the most recent change to the response's own values. Confirming an
already-logged, still-`processing`/`pending_review` row later the same day changes
`calories_consumed`/the macro sums without moving `LoggedAt` (confirmation doesn't relog the meal),
so a field literally named "last updated" would understate its own freshness at that moment. This
is not a staleness bug in the response itself — every field on this endpoint, including the
consumed sums and the target, is computed fresh on every call, never cached — it is purely a naming
hazard: `last_updated` invites a client to treat it as "how fresh is this payload," when what it
actually means is "when did the user last log something." `last_logged_at` says that directly.

A new `database.TodaySummary(db *gorm.DB, userID uuid.UUID, loc *time.Location, now time.Time)
(TodaySummaryRow, error)` function in `food_completeness.go`'s package computes the UTC window
bounds for today the same way `DayRange` does for an arbitrary day (`fromDate.UTC()` /
`fromDate.AddDate(0,0,1).UTC()`, required because `FoodMeal.LoggedAt` is stored UTC-normalized and
SQLite compares it as text), then does one query for all of today's `FoodMeal` rows and folds them
into counts/sums/max in Go — no new SQL aggregation needed at this row volume. `now` is threaded in
explicitly (matching `computeUserNutritionTarget(storage, userID, now)` and how `LocalDate(t, loc)`
already takes an explicit `t` rather than calling `time.Now()` internally) so `SummaryTodayHandler`
computes `now := time.Now().UTC()` once — matching `NutritionTargetHandler`'s existing convention,
not bare `time.Now()` — and passes that same value into both `TodaySummary` and
`computeUserNutritionTarget` — keeping the reported `date` and the aggregated query window
consistent with each other at the exact local-midnight boundary — and so the boundary test in
`tasks.md` (today-vs-yesterday around local midnight) can use a fixed, deterministic `now` instead
of depending on wall-clock time. The `.UTC()` matters beyond convention: `computeUserNutritionTarget`
threads `now` into `latestPointValue`'s raw `Where(timeCol+" <= ?", now)` comparison against
`weights`/`heights`/`weight_goals` columns that are always stored UTC-normalized, and — per
`DayRange`'s own documented reasoning above — go-sqlite3 stores `time.Time` as TEXT preserving
whatever offset it's given, so SQLite compares it as text, not as an instant. A non-UTC `now` (Go's
`time.Now()` defaults to the process's Local location, and nothing in this project's Dockerfiles
pins `TZ=UTC`) would string-compare incorrectly against those UTC-offset rows, silently regressing
`GET /api/users/me/nutrition-target` itself (via the shared `computeUserNutritionTarget`) on any
non-UTC host — a behavior change Task 3.1 explicitly promises not to make.

### 2. Login attempt limiting

**In-memory, per-username, sliding window + exponential backoff.** A package-level, mutex-guarded
map keyed by the lowercased username. **Correction to an earlier assumption:** `FindUserByName`
(`backend/pkg/database/storage_impl.go`) does a plain `Where("username = ?", ...)` lookup with no
`COLLATE NOCASE`, and `kin-core`'s `User.Username` column has no case-insensitive collation either
— account lookup in this codebase is case-sensitive, so two accounts named e.g. `alice` and `Alice`
can coexist as distinct users. Lowercased keying is kept anyway, as defense-in-depth against
case-varied guessing against the *same* intended account, but this is an explicit, accepted
trade-off, not a "matches existing identity" guarantee: a failed-attempt run against `Alice` and
one against `alice` share one lockout bucket, so an attacker guessing one case variant of a
username can also lock out a different, legitimately-existing account spelled with different
casing. At this project's few-accounts-ever scale this is judged an acceptable cost for the
simplicity of one lookup rule, not a scenario worth adding real per-account case tracking for.

- **Window**: 15 minutes, trailing. Failed attempts (unknown username **or** wrong password —
  identical treatment, so lockout presence/absence can never be used as a username-existence
  oracle) increment a counter and record a timestamp.
- **Threshold**: 5 failed attempts within the window trips a lockout.
- **Attempt admission is atomic per username, not check-then-act — but admission reserves
  capacity, it does not record a failure.** Credential verification (`auth.VerifyPassword`, bcrypt)
  takes tens of milliseconds — long enough for many concurrent requests against the same username
  to all observe "not locked" before any of them finishes and records a failure, which would let an
  attacker fire a burst of parallel guesses per window instead of five. Closing that race by
  provisionally counting every *admitted* attempt as a failure was considered and rejected: doing so
  makes a burst of legitimate concurrent requests with the *correct* password (a double-tapped
  login button, a mobile client's retry-on-timeout, two devices signing in at once) trip a real,
  timed lockout — with escalating backoff — despite zero actual failures, which directly
  contradicts "5 **failed** attempts" below. Concurrency safety and failure counting are kept
  separate instead:
  - Each username tracks a failure count/timestamps (as below) **plus an in-flight counter** —
    the number of admitted attempts currently inside credential verification, not yet resolved.
  - `Login` calls a single mutex-guarded `admitAttempt(username) (allowed bool, retryAfter
    time.Duration)` **before** verifying credentials. Under the same lock held by
    `admitAttempt`/`recordFailure`/`recordSuccess`, `admitAttempt` (a) rejects immediately with the
    existing lockout's `retryAfter` if already locked, else (b) rejects with a fixed
    `retryAfter` of 1 second (see "429 response" below) if `failure count + in-flight count >= 5`
    (capacity is saturated by some mix of
    confirmed failures and attempts still being verified), else (c) increments the in-flight counter
    and returns `allowed = true`. Case (b) is a transient "try again shortly" signal, not a lockout:
    it sets no `lockedUntil`, advances no backoff level, and self-clears as soon as the in-flight
    attempts ahead of it resolve — it exists only to keep concurrent admission from exceeding 5
    outstanding attempts, mirroring the real threshold so an attacker still cannot get more than 5
    guesses verified per window no matter how many requests arrive at once.
  - Credential verification then proceeds outside the lock. Afterwards `Login` calls exactly one of:
    - `recordFailure(username)` on a 401 — decrements the in-flight counter, then advances the
      *confirmed* failure count/timestamp, tripping a real lockout (with backoff, per "Lockout
      duration" below) only now, if the confirmed count reaches the threshold.
    - `recordSuccess(username)` on success — decrements the in-flight counter and clears the
      failure count/backoff entirely, as already specified below.
  Because only a confirmed 401 ever advances the failure count or trips a lockout, a burst of
  concurrent *correct*-password requests can momentarily see case (b)'s short retry while capacity
  is full, but never trips a timed lockout and never blocks a login once the in-flight batch
  resolves. Because the in-flight counter is reserved at admission time under the same lock, an
  attacker still cannot get more than 5 guesses into verification per window regardless of
  concurrency — the 6th concurrent request sees capacity already reserved by attempts #1-#5, even
  if all five are still running their bcrypt compare.
- **Test seam for deterministic concurrency tests.** Real bcrypt is fast enough (tens of
  milliseconds) that simply launching N goroutines against `Login` and asserting some subset
  observed the capacity rejection would be scheduler-dependent: an early goroutine can finish
  verification and call `recordSuccess` — freeing its in-flight slot — before a later goroutine
  ever reaches `admitAttempt`, so a batch of 6 is not guaranteed to produce 5 admissions and 1
  rejection; it could produce 6 successive admit-then-resolve cycles with none overlapping.
  `Login` calls an unexported, test-only hook, `var afterAdmitHook func()`, immediately after a
  successful admission and before credential verification begins; it is `nil` in production (zero
  cost, never set outside `_test.go` files). A concurrency test sets it to block each admitted
  goroutine on a channel the test controls, so it can: admit exactly 5 goroutines and hold them
  there (in-flight counter pinned at 5, none resolved yet), *then* issue the 6th request and
  assert it deterministically observes the capacity rejection, *then* release the held goroutines
  to resolve normally. This replaces relying on wall-clock timing with an explicit barrier, so the
  test's pass/fail no longer depends on goroutine scheduling.
- **Lockout duration**: exponential backoff starting at 1 minute, doubling on each *subsequent*
  lockout for the same username (1m, 2m, 4m, 8m, 16m, capped at 30m), reset to 1m after a period
  with no failed attempts at all for 24h. This punishes sustained attack traffic increasingly
  harder while not permanently locking out a user who mistypes their password a few times after a
  long break.
- **Reset on success**: a successful login clears the username's failure counter and backoff
  level entirely.
- **Reset on lockout trip**: tripping a lockout also clears the trailing-window failure-timestamp
  list (but **not** the backoff level — that only resets via the success or 24h-quiet paths above).
  Each entry separately tracks a last-activity timestamp, updated on every failed attempt and on
  lockout trip, that is never cleared by the failure-list reset — this is what "expired" means for
  the sweep/ceiling eviction above (an entry with no activity for well past the 24h reset window),
  so clearing the failure list on lockout trip does not make an entry look prematurely stale or
  evictable. **A nonzero in-flight counter always makes an entry non-expired, regardless of
  `lastActivity`** — a brand-new username's very first admitted attempt has zero confirmed
  failures, no lockout, and a `lastActivity` that has never been set (since nothing updates it
  before a failure or lockout trip), so without this rule the entry would read as "expired" by the
  `lastActivity` criterion alone and be sweepable or evictable while its one in-flight attempt is
  still inside `auth.VerifyPassword`. Evicting it there would let the username be recreated with a
  fresh in-flight counter (defeating the 5-in-flight admission cap) and would leave the eventual
  `recordFailure`/`recordSuccess` call decrementing an in-flight counter on a recreated or absent
  entry. Without
  this, the sliding window's 15-minute lifetime would outlast most lockout durations (1m-30m), so
  the very next failed attempt after a lockout expires would see the window still holding the prior
  5 failures and re-trip immediately — contradicting the "5 more failed attempts" framing of
  escalation below. Clearing the count on trip means each escalation step genuinely requires a
  fresh batch of 5 failures after the previous lockout expires, while the backoff *level* still
  remembers the escalation history.
- **Scope of the lock**: blocks new calls to `POST /api/auth/login` for that username only, while
  locked. It does **not** touch `POST /api/auth/refresh`, `POST /api/auth/logout`, or
  `RequireAuth` — an already-issued access/refresh token pair keeps working normally through a
  lockout window. This resolves the open question Task 1 flagged (whether existing valid tokens
  keep working during a lockout): they do, because the attack this defends against is credential
  guessing at the login endpoint, not session validity, and locking out a legitimate user's
  already-authenticated app session as a side effect of someone else guessing their password would
  be a worse outcome than the lockout itself.
- **Unknown usernames run a dummy bcrypt compare, closing a timing side channel.** `Login`
  (`backend/pkg/server/auth.go`) currently returns 401 immediately when `FindUserByName` finds no
  match, skipping bcrypt entirely, while a known username with a wrong password pays the full bcrypt
  cost before its 401. That timing gap is a username-enumeration oracle regardless of this change —
  but this change is exactly what makes it reachable from the open internet (Access currently hides
  `/api/auth/login`; the Bypass policy this work unblocks does not), so closing it belongs here.
  `kin-core/auth` already ships `auth.DummyHash` and documents `VerifyPassword` as "always runs
  bcrypt even if hash is DummyHash, preventing timing oracles" for exactly this case — it is unused
  in HealthVault today. `Login` SHALL call `auth.VerifyPassword(req.Password, auth.DummyHash)` on the
  `FindUserByName` not-found path (discarding the result, since the user doesn't exist) before
  returning 401, so an unknown-username response and a known-username/wrong-password response take
  statistically indistinguishable time. Both paths SHALL still go through `admitAttempt` before
  verification and `recordFailure` after their 401 identically, per the existing "identical
  treatment" rule above.
- **No IP tracking.** The origin sits behind a Cloudflare Tunnel; trusting a client-supplied header
  for the real IP without first wiring up trusted-proxy header validation would be a spoofable
  input, and that wiring is out of scope here. Per-username is sufficient for the stated threat
  (unlimited-attempts credential guessing against one account) and requires no new trust
  assumption.
- **429 response**: `{"error": "too_many_attempts", "retry_after_seconds": <n>}` in all three
  rejection cases (real lockout, in-flight capacity, ceiling fail-closed), but `n` means something
  different in each because only one of them is actually a timed lockout:
  - **Real lockout** (`admitAttempt` finds the username already locked, before credential
    verification runs): `n` is the number of seconds remaining until `lockedUntil`, rounded up.
    The attempt whose confirmed failure *trips* a new lockout is never itself in this case: per
    the "Fifth failed attempt trips the lockout" scenario, that attempt has already committed to
    a 401 for its own failed credential check by the time `recordFailure` trips the lockout, so it
    returns 401, not 429. Only a later attempt — rejected by `admitAttempt` before verification
    even starts — sees this 429.
  - **In-flight capacity rejection** and **ceiling fail-closed rejection** (both defined below):
    neither sets a `lockedUntil` or advances backoff, so there is no lockout duration to report.
    `n` is instead the fixed constant **1 second** — long enough for an in-flight bcrypt compare
    (tens of milliseconds) to resolve and free capacity, short enough not to read as a real
    lockout. A client SHOULD treat this as "retry almost immediately," not as evidence of a
    lockout.
  No cookies are set in any of the three cases.
- **No background goroutine, but bounded map size.** Per `openspec/config.yaml`'s existing
  constraint ("no background job infrastructure... `ListenAndServe` is the only goroutine"), stale
  map entries (a username with no failures for well past the 24h reset window) are evicted lazily —
  but *not* only on next access to that same key, since the map is keyed by attacker-supplied
  username strings recorded before `FindUserByName` is even consulted: an attacker submitting a
  distinct, never-repeated username per request would create an entry that is never accessed again,
  so pure "evict on next access to that key" cleanup can never reclaim it, and the map would grow
  without bound on exactly the endpoint this change exists to protect from being placed on the open
  internet. Instead, `admitAttempt` sweeps a bounded number of the oldest expired entries (by last
  activity) on every call — cheap, no ticker, no per-key dependency, and `admitAttempt` is called on
  every login attempt (success, failure, or rejection alike), unlike `recordFailure` which now only
  runs after a confirmed 401 — and the map additionally
  enforces a hard size ceiling (an order of magnitude above this project's realistic account count)
  past which room is made for a new entry by evicting the oldest **expired** entry only — never an
  entry that is within an active lockout, within its trailing failure window, has a nonzero
  failure count, or has a nonzero in-flight counter (an attempt still inside credential
  verification). Eviction eligibility is therefore identical for the sweep and the ceiling: only
  "expired" (fully quiet, nothing to protect) entries are ever removed. **If the ceiling is reached
  and every entry is still live (none expired), the login attempt for the new username is rejected
  with 429 (the same `too_many_attempts` shape as an ordinary lockout, `retry_after_seconds` fixed
  at 1 second per the "429 response" bullet above, not tied to any lockout duration) instead of
  being admitted unrecorded.** A drop-and-admit policy here would
  reopen exactly the hole the ceiling exists to prevent: since the ceiling is only ever reached by
  live (non-expired) entries, an attacker can trivially keep it saturated by sending one failed
  attempt against a large batch of throwaway usernames often enough that none of them ever expire;
  once saturated, every *other* never-before-seen username — including real, previously-quiet
  accounts — would get a fresh, unrecorded, unlimited-attempts admission on every request, which is
  worse than having no ceiling at all. Failing closed at the ceiling means a flood large enough to
  saturate the map denies login attempts broadly (an availability cost, recoverable as soon as
  entries expire) rather than silently disabling the limiter for whichever username the flood
  hasn't already claimed. This still closes the original eviction side channel a naive
  "evict the least-recently-active entry" policy would have: an attacker still cannot flush a
  targeted, actively-protected username's failure count or lockout by flooding the map, because live
  entries are still never evicted — the difference is only what happens to the *new* request once
  no eviction is possible.
- **Constants, not env config.** Unlike ports/paths (`deployment-config`'s existing env-var
  contract), the threshold/window/backoff schedule are Go constants. This is deliberately
  inconsistent with "protection travels with the software for any self-hoster" only in the sense
  that a self-hoster can't retune it without a code change — an intentional simplicity trade-off;
  nothing about this threat model calls for per-deployment tuning, and adding config surface for
  numbers nobody has asked to change would be exactly the kind of complexity `/data/CLAUDE.md`
  says to avoid at this project's scale.

#### Rejected alternative: DB-backed lockout via `kin-core/authdb`

`authdb` already has a DB-backed token blacklist keyed by token string. Extending it (or adding a
sibling table) to also track login failures was considered, for durability across process
restarts. Rejected for this change: it would require a `kin-core` schema/version bump for a
capability only HealthVault currently needs (Task 1 found no rate-limiting precedent anywhere in
`kin-core`), and an in-memory lockout that resets on backend restart is an acceptable trade-off at
this project's scale — a restart is already a rare, operator-initiated event, not something an
attacker can trigger to reset their own lockout.

## Risks / Trade-offs

- **Restart clears all lockout state.** Accepted per the rejected-alternative note above.
- **A single Cloudflare Tunnel origin sees all traffic as coming through the tunnel**, so once
  `/api/*` gets its Bypass policy (out of this repo's scope), every login attempt reaching this
  limiter is already past Cloudflare's zone-wide edge. This limiter is the only per-account gate at
  that point; it is deliberately conservative (5 attempts) rather than tuned for convenience.
- **`daily-summary`'s two extra queries per widget refresh** (today's meals, if the target
  computation's own weight/height/goal/steps reads are counted separately) run at most once per
  30 minutes per the plan's WorkManager cadence, plus the ~60s post-Custom-Tab follow-up — trivial
  load at this project's scale.

## Migration Plan

No data migration. No breaking change to any existing endpoint or response shape.
