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
  "last_updated": "2026-08-26T14:32:00Z",
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

**`last_updated` is the max `LoggedAt` among today's rows (any status), or `null` if none.** A
`processing` row still means "the user just took a photo" — that's activity worth reflecting in
"updated Xh ago" even before recognition finishes, matching the plan's requirement that a
`~60s follow-up` refresh after returning from the Custom Tab has something to observe.

A new `database.TodaySummary(db *gorm.DB, userID uuid.UUID, loc *time.Location) (TodaySummaryRow,
error)` function in `food_completeness.go`'s package computes the UTC window bounds for today the
same way `DayRange` does for an arbitrary day (`fromDate.UTC()` / `fromDate.AddDate(0,0,1).UTC()`,
required because `FoodMeal.LoggedAt` is stored UTC-normalized and SQLite compares it as text), then
does one query for all of today's `FoodMeal` rows and folds them into counts/sums/max in Go — no
new SQL aggregation needed at this row volume.

### 2. Login attempt limiting

**In-memory, per-username, sliding window + exponential backoff.** A package-level, mutex-guarded
map keyed by the lowercased username (matching how `FindUserByName` already resolves accounts —
usernames aren't case-sensitive identity in this codebase's existing lookup, so limiter state must
key the same way or a case-varied login attempt would bypass its own lockout):

- **Window**: 15 minutes, trailing. Failed attempts (unknown username **or** wrong password —
  identical treatment, so lockout presence/absence can never be used as a username-existence
  oracle) increment a counter and record a timestamp.
- **Threshold**: 5 failed attempts within the window trips a lockout.
- **Lockout duration**: exponential backoff starting at 1 minute, doubling on each *subsequent*
  lockout for the same username (1m, 2m, 4m, 8m, 16m, capped at 30m), reset to 1m after a period
  with no failed attempts at all for 24h. This punishes sustained attack traffic increasingly
  harder while not permanently locking out a user who mistypes their password a few times after a
  long break.
- **Reset on success**: a successful login clears the username's failure counter and backoff
  level entirely.
- **Scope of the lock**: blocks new calls to `POST /api/auth/login` for that username only, while
  locked. It does **not** touch `POST /api/auth/refresh`, `POST /api/auth/logout`, or
  `RequireAuth` — an already-issued access/refresh token pair keeps working normally through a
  lockout window. This resolves the open question Task 1 flagged (whether existing valid tokens
  keep working during a lockout): they do, because the attack this defends against is credential
  guessing at the login endpoint, not session validity, and locking out a legitimate user's
  already-authenticated app session as a side effect of someone else guessing their password would
  be a worse outcome than the lockout itself.
- **No IP tracking.** The origin sits behind a Cloudflare Tunnel; trusting a client-supplied header
  for the real IP without first wiring up trusted-proxy header validation would be a spoofable
  input, and that wiring is out of scope here. Per-username is sufficient for the stated threat
  (unlimited-attempts credential guessing against one account) and requires no new trust
  assumption.
- **429 response**: `{"error": "too_many_attempts", "retry_after_seconds": <n>}`, `n` rounded up to
  the nearest second remaining in the lockout. No cookies are set.
- **No background goroutine.** Per `openspec/config.yaml`'s existing constraint ("no background job
  infrastructure... `ListenAndServe` is the only goroutine"), stale map entries (a username with no
  failures for well past the 24h reset window) are evicted lazily on next access to that key, not
  via a ticker. At this project's user count (a handful of accounts, ever) the map never grows
  large enough for unbounded growth to matter even without eviction, but lazy cleanup costs nothing
  extra to add.
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
