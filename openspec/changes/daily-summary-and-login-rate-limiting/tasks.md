## 1. Backend: login attempt limiting

- [ ] 1.1 Add `backend/pkg/server/login_limiter.go`: a mutex-guarded, package-level map keyed by
      lowercased username, tracking failed-attempt timestamps (15-minute trailing window), current
      backoff level, and lockout-expiry time, per design.md's "Login attempt limiting" decision
- [ ] 1.2 Implement `recordFailure(username string)`, `recordSuccess(username string)`, and
      `checkLocked(username string) (locked bool, retryAfter time.Duration)`. On every
      `recordFailure` call, sweep a bounded number of the oldest expired entries (no background
      goroutine, per `openspec/config.yaml`'s "no background job infrastructure" constraint) and
      enforce a hard map-size ceiling (evict the oldest entry to make room past the ceiling), per
      design.md's bounded-map-size note — a pure "evict only on next access to the same key" scheme
      does not reclaim entries created by a never-repeated attacker-supplied username
- [ ] 1.3 Implement the exponential backoff schedule: 1m/2m/4m/8m/16m/30m-cap, doubling per
      lockout since the last successful login, resetting to 1m after 24h with no failures.
      Tripping a lockout SHALL clear the username's trailing-window failure count (not its backoff
      level) per design.md's "Reset on lockout trip" note, so escalation requires a fresh 5
      failures after each lockout expires
- [ ] 1.4 Wire into `backend/pkg/server/auth.go`'s `Login`: check `checkLocked` before verifying
      credentials; on 401 (unknown username or wrong password) call `recordFailure`; on success
      call `recordSuccess`. A locked username SHALL be rejected with 429 even when credentials in
      the request are correct — do not verify credentials before the lock check
- [ ] 1.5 Return `{"error": "too_many_attempts", "retry_after_seconds": <n>}` with HTTP 429 and no
      cookies set when locked

## 2. Backend: login limiting unit tests

- [ ] 2.1 Unit-test the 5th-failure lockout trip, and that a 6th attempt with correct credentials
      is still rejected with 429
- [ ] 2.2 Unit-test that an unknown username accumulates failures identically to a known username
      with wrong password (same lockout behavior)
- [ ] 2.3 Unit-test that a successful login clears the failure count and backoff level
- [ ] 2.4 Unit-test backoff escalation across two consecutive lockouts (1m then 2m) and the 24h
      reset back to 1m
- [ ] 2.5 Unit-test that username matching is case-insensitive (lockout on `Alice` blocks `alice`)
- [ ] 2.6 Unit-test that `Refresh`, `Logout`, and a `RequireAuth`-protected request are unaffected
      by a concurrent lockout on the same username
- [ ] 2.7 Unit-test that a failed attempt older than 15 minutes does not count toward the
      5-attempt threshold (4 recent failures plus 1 that has aged out of the window does not trip
      a lockout), and that `retry_after_seconds` in a 429 response reflects the actual remaining
      lockout duration (rounded up), not a fixed constant
- [ ] 2.8 Unit-test that tripping a lockout clears the failure count so a single failed attempt
      immediately after the lockout expires does not itself re-trigger a lockout (distinguishing
      this from the escalation scenario in 2.4, which requires 5 fresh failures)

## 3. Backend: daily summary computation

- [ ] 3.1 Extract `nutrition_target.go`'s precondition checks and computation into a single
      reusable function (e.g. `computeUserNutritionTarget(storage, userID, now) (target
      nutritionTargetValues, unavailableReason string, err error)`), called by both the existing
      `NutritionTargetHandler` (422s on non-empty `unavailableReason`, no behavior change) and the
      new summary handler
- [ ] 3.2 Add `database.TodaySummary(db *gorm.DB, userID uuid.UUID, loc *time.Location, now
      time.Time) (TodaySummaryRow, error)` in the `food_completeness.go` package: compute today's
      Local Day via `database.LocalDate(now, loc)`, derive UTC window bounds the same way
      `DayRange` does for a single day, query all of today's `FoodMeal` rows for the user, and fold
      them into: sums of `calories`/`protein_grams`/`carbs_grams`/`fat_grams` restricted to
      `status = confirmed` rows, a total row count across all statuses, and the max `logged_at`
      across all statuses (or a zero/absent value when there are no rows). `now` is an explicit
      parameter (not `time.Now()` internally) so callers and tests can pin it, matching
      `computeUserNutritionTarget`'s pattern
- [ ] 3.3 Add `backend/pkg/server/summary_today.go`: `SummaryTodayHandler(storage)` —
      self-only (`ClaimsFromCtx`, no `?user=`), resolves the caller's timezone via
      `database.ResolveTimezone` (reusing `callerTimezoneAndThreshold`'s settings-read pattern or a
      shared helper), computes `now := time.Now()` once and passes that same value into both
      `database.TodaySummary` and `computeUserNutritionTarget` (so `date` and the aggregated
      window never disagree at a local-midnight boundary), reads `display_language` via the
      existing resolver, and assembles the response per design.md's shape (`date`,
      `calories_consumed`, `protein_grams_consumed`, `carbs_grams_consumed`, `fat_grams_consumed`,
      `meal_count`, `last_updated`, `display_language`, `target`, `recommendation: null`)
- [ ] 3.4 Register `GET /api/summary/today` in `backend/pkg/server/server.go`, behind the existing
      `api.Use(RequireAuth(...))` middleware, alongside `/users/me/nutrition-target`

## 4. Backend: daily summary unit tests

- [ ] 4.1 Unit-test `database.TodaySummary`: confirmed-only sums, all-status meal count, all-status
      max `logged_at`, and the zero-meals-today case
- [ ] 4.2 Unit-test that a `processing`/`pending_review`/`pending_clarification`/`failed` meal
      contributes to `meal_count` and `last_updated` but not to the consumed macro sums
- [ ] 4.3 Unit-test `SummaryTodayHandler`'s target embedding for each of: an available target, and
      each of the 4 `nutrition-target` unavailable reasons — asserting HTTP 200 in every case, not
      422
- [ ] 4.4 Unit-test that `GET /api/summary/today` ignores any `?user=` query parameter (self-only,
      matching `nutrition-target`'s existing test coverage pattern) and returns 401 with no valid
      token
- [ ] 4.5 Unit-test the Local Day boundary reuse: a meal logged just before/after local midnight in
      a non-UTC `timezone` setting lands in the correct day's summary (mirrors
      `food-day-completeness`'s existing timezone-boundary test cases), using a fixed `now` passed
      into `database.TodaySummary` rather than depending on wall-clock time

## 5. Documentation

- [ ] 5.1 New `docs/adr/ADR-008-<slug>.md` (`Status: Proposed`) recording the decision to
      implement login attempt limiting as an in-memory, per-process, HealthVault-local mechanism
      rather than extending `kin-core/authdb`'s token blacklist — the sliding-window +
      exponential-backoff design, the bounded-map-size/no-IP-tracking scoping, and the
      "restart clears lockout state" trade-off accepted in design.md's "Risks / Trade-offs"
      section. This is the first rate-limiting/lockout precedent anywhere in this codebase or
      `kin-core`, matching the ADR precedent set by ADR-005/ADR-006/ADR-007 for similarly-scoped
      decisions in prior changes

## 6. Verification

- [ ] 6.1 `make lint`
- [ ] 6.2 `make test`
- [ ] 6.3 `openspec validate daily-summary-and-login-rate-limiting --strict`
