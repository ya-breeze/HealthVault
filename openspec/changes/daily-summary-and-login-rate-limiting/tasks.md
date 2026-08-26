## 1. Backend: login attempt limiting

- [x] 1.1 Add `backend/pkg/server/login_limiter.go`: a mutex-guarded, package-level map keyed by
      lowercased username, tracking **confirmed** failed-attempt timestamps (15-minute trailing
      window), an in-flight counter (admitted attempts currently inside credential verification,
      not yet resolved), current backoff level, lockout-expiry time, and a last-activity timestamp
      (updated on every confirmed failed attempt and on lockout trip, never cleared by the
      trailing-window reset — this, together with a nonzero in-flight counter, is what "expired"
      below is defined against: an entry is expired only if `lastActivity` is well past the 24h
      reset window **and** its in-flight counter is zero), per design.md's "Login attempt limiting"
      decision
- [x] 1.2 Implement `admitAttempt(username string) (allowed bool, retryAfter time.Duration)`,
      `recordFailure(username string)`, and `recordSuccess(username string)`. `admitAttempt` SHALL
      run under the map's single mutex and, atomically: reject with the existing lockout's
      `retryAfter` if already locked; else reject with a fixed `retryAfter` of 1 second (not tied
      to any lockout — see design.md's "429 response" bullet) if `confirmed
      failure count + in-flight count >= 5` (capacity saturated, per design.md's "Attempt admission
      is atomic per username" note — this rejection SHALL NOT set a lockout, advance the backoff
      level, or touch the confirmed failure count, since none of the in-flight attempts have failed
      yet); else increment the in-flight counter and return `allowed = true`. This closes the race
      where credential verification (bcrypt) is slow enough for many concurrent callers to all
      observe "not locked" before any of them resolves, without conflating "admitted" with
      "failed" — a burst of concurrent *correct*-password requests may transiently see the capacity
      rejection, but never trips a real lockout, because only `recordFailure` (below, called after a
      confirmed 401) ever advances the confirmed failure count or backoff. `recordFailure` SHALL run
      under the same mutex and: decrement the in-flight counter, then advance the confirmed failure
      count/timestamp in the trailing window, tripping a new lockout right there if the confirmed
      count reaches the threshold. `recordSuccess` SHALL run under the same mutex and: decrement the
      in-flight counter, then clear the confirmed failure count and backoff level entirely (per the
      "Reset on success" note). On every `admitAttempt` call (the highest-frequency entry point —
      called for every login attempt, not just confirmed failures), sweep a bounded number of the
      oldest expired entries (no background goroutine, per `openspec/config.yaml`'s "no background
      job infrastructure" constraint) and enforce a hard map-size ceiling: past the ceiling, evict
      the oldest **expired** entry to make room for the new one (same expiry criterion as the sweep
      — never an entry with an active lockout, a nonzero trailing-window failure count, a nonzero
      in-flight counter, or recent activity — so an entry with an attempt still inside credential
      verification is never evicted regardless of how stale its last-activity timestamp looks, which
      matters most for a brand-new username's very first admitted attempt, whose last-activity has
      not been set yet); if every entry is still live when the ceiling is hit, `admitAttempt` SHALL reject
      the new username's attempt with the standard 429 shape (fail closed) rather than admitting it
      unrecorded. Per design.md's bounded-map-size note, a pure "evict only on next access to the
      same key" scheme does not reclaim entries created by a never-repeated attacker-supplied
      username; evicting by recency alone (rather than expiry) would let an attacker flush a
      targeted username's protection by flooding the map with throwaway usernames until the ceiling
      forces it out; and admitting-unrecorded at the ceiling would let an attacker who keeps the map
      saturated with live throwaway entries permanently disable the limiter for every other
      never-before-seen username, including real accounts
- [x] 1.3 Implement the exponential backoff schedule: 1m/2m/4m/8m/16m/30m-cap, doubling per
      lockout since the last successful login, resetting to 1m after 24h with no failures.
      Tripping a lockout SHALL clear the username's trailing-window failure count (not its backoff
      level) per design.md's "Reset on lockout trip" note, so escalation requires a fresh 5
      failures after each lockout expires
- [x] 1.4 Wire into `backend/pkg/server/auth.go`'s `Login`: call `admitAttempt` before verifying
      credentials; if `allowed = false`, reject with 429 immediately, no cookies set, without
      touching `FindUserByName`/`auth.VerifyPassword` at all. If `allowed = true`, verify
      credentials: on unknown username, call `auth.VerifyPassword(req.Password, auth.DummyHash)`
      (discard the result) before returning 401, so the response timing is indistinguishable from a
      known-username/wrong-password 401 (per design.md's "Unknown usernames run a dummy bcrypt
      compare" note — `kin-core/auth.DummyHash` already exists for this); on a 401 call
      `recordFailure`; on success call `recordSuccess`. A locked username SHALL be rejected with 429
      even when credentials in the request are correct — do not verify credentials before the
      `admitAttempt` check
- [x] 1.5 Add an unexported test seam, `var afterAdmitHook func()`, called by `Login` immediately
      after a successful admission and before credential verification begins; `nil` in production.
      Per design.md's "Test seam for deterministic concurrency tests" note, this lets 2.13 hold a
      known number of admitted goroutines open (in-flight counter pinned) before issuing the next
      request, instead of relying on goroutine-scheduling timing to produce an overlap
- [x] 1.6 Return `{"error": "too_many_attempts", "retry_after_seconds": <n>}` with HTTP 429 and no
      cookies set when locked (`n` = seconds remaining until `lockedUntil`, rounded up), when
      rejected by the ceiling fail-closed path, or when rejected by `admitAttempt`'s in-flight
      capacity check (`n` = fixed at 1 in both of the latter two cases, per design.md's "429
      response" bullet — neither sets a `lockedUntil`)

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
- [ ] 2.9 Unit-test that flooding the map with distinct, never-repeated throwaway usernames past
      the hard size ceiling does not evict a different username's active lockout or nonzero
      failure count — the flood's own entries (or, if none are evictable, the newest flood entry)
      are dropped/evicted instead, per the "never evict a live entry" rule in 1.2
- [ ] 2.10 Unit-test the fail-closed ceiling path: once the map is saturated with live (non-expired)
      entries, `admitAttempt` for a brand-new username SHALL return `allowed = false` with a 429
      shape, not `allowed = true` unrecorded — i.e. the new username's own attempt is also
      rate-limited under sustained flood, not silently let through
- [ ] 2.11 Unit-test that `admitAttempt` is safe under concurrency: firing many goroutines'
      `admitAttempt` calls for the same never-locked-before username concurrently admits at most 5
      before the 6th (and later) see the in-flight capacity rejection — i.e. the in-flight count is
      reserved atomically at admission, not after a simulated slow credential-verification step
- [ ] 2.12 Unit-test that `Login` with an unknown username still calls `auth.VerifyPassword` against
      `auth.DummyHash` (e.g. by asserting comparable timing, or by asserting the call happens via a
      test seam) so an unknown-username 401 is not measurably faster than a wrong-password 401
- [ ] 2.13 Unit-test that concurrent *correct*-password login requests never trip a real lockout,
      using the 1.5 `afterAdmitHook` test seam to make the overlap deterministic instead of relying
      on goroutine-scheduling timing: set the hook to signal a channel and then block each admitted
      goroutine on a second, test-controlled channel; launch exactly 5 concurrent `Login` calls for
      the same never-locked-before username, all with the correct password; wait for all 5 signals
      (confirming all 5 are admitted and held, in-flight counter pinned at 5, none resolved yet);
      only then issue a 6th `Login` call for that username (also correct password) and, since
      capacity is already pinned at 5, assert it deterministically observes the transient in-flight
      capacity rejection (carries no `lockedUntil` and no backoff, and is not evidence of a failed
      attempt), not a real lockout; then release the held channel so the 5 held goroutines proceed
      to real credential verification and `recordSuccess`; assert all 5 held calls succeed, the 6th
      does not, none of the 6 calls ever tripped a real lockout, and that a subsequent `Login` call
      for that username made after the batch finishes SHALL succeed rather than being rejected with
      429 — i.e. `recordSuccess` clears state fully and no confirmed failure was ever recorded,
      distinguishing this from 2.11's all-guesses-fail scenario. The 6th call is expected to observe
      the capacity rejection (this is the expected, correct behavior per the "Concurrent attempts
      cannot exceed the threshold" spec scenario applied to a mix of in-flight slots, not a bug to
      work around) — issuing it only after the barrier confirms all 5 slots are held is what makes
      that outcome deterministic rather than a race between the 6th request's arrival and the first
      5 resolving
- [ ] 2.14 Unit-test that an entry with an in-flight attempt is never evicted by the sweep or the
      ceiling while that attempt's credential verification is still pending: admit a brand-new
      username's very first attempt (so it has zero confirmed failures, no lockout, and an unset
      last-activity timestamp) without resolving it, then drive the sweep and separately saturate
      the map to the ceiling with other expired entries, and assert in both cases the in-flight
      entry survives and its eventual `recordFailure`/`recordSuccess` call resolves against the
      same entry rather than panicking, creating a new entry, or silently losing the decrement

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
      shared helper), computes `now := time.Now().UTC()` once (matching `NutritionTargetHandler`'s
      existing convention — bare `time.Now()` would silently regress `latestPointValue`'s SQLite
      text comparison on a non-UTC host, per design.md) and passes that same value into both
      `database.TodaySummary` and `computeUserNutritionTarget` (so `date` and the aggregated
      window never disagree at a local-midnight boundary), reads `display_language` via the
      existing resolver, and assembles the response per design.md's shape (`date`,
      `calories_consumed`, `protein_grams_consumed`, `carbs_grams_consumed`, `fat_grams_consumed`,
      `meal_count`, `last_logged_at`, `display_language`, `target`, `recommendation: null`)
- [ ] 3.4 Register `GET /api/summary/today` in `backend/pkg/server/server.go`, behind the existing
      `api.Use(RequireAuth(...))` middleware, alongside `/users/me/nutrition-target`

## 4. Backend: daily summary unit tests

- [ ] 4.1 Unit-test `database.TodaySummary`: confirmed-only sums, all-status meal count, all-status
      max `logged_at`, and the zero-meals-today case
- [ ] 4.2 Unit-test that a `processing`/`pending_review`/`pending_clarification`/`failed` meal
      contributes to `meal_count` and `last_logged_at` but not to the consumed macro sums
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
