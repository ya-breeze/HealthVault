package server

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func newTestLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]*loginLimiterEntry)}
}

// fiveFailures runs admitAttempt+recordFailure five times for username,
// simulating five confirmed-failed login attempts the way auth.go's Login
// handler drives the limiter on repeated 401s.
func fiveFailures(l *loginLimiter, username string) {
	for i := 0; i < loginLockoutThreshold; i++ {
		l.admitAttempt(username)
		l.recordFailure(username)
	}
}

// 2.1: the 5th failure trips a lockout, and a 6th attempt with correct
// credentials (i.e. one that never fails) is still rejected with 429 because
// admitAttempt itself refuses while locked.
func TestLoginLimiter_FifthFailureTripsLockout_SixthRejected(t *testing.T) {
	l := newTestLoginLimiter()
	fiveFailures(l, "alice")

	allowed, retryAfter := l.admitAttempt("alice")
	if allowed {
		t.Fatalf("6th attempt should be rejected after 5 confirmed failures")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected a positive retryAfter, got %v", retryAfter)
	}
}

// 2.3: a successful login clears the failure count and backoff level.
func TestLoginLimiter_SuccessClearsFailuresAndBackoff(t *testing.T) {
	l := newTestLoginLimiter()
	// Four failures: not enough to trip a lockout, but they populate the
	// trailing-window failure list and must be cleared by recordSuccess.
	for i := 0; i < loginLockoutThreshold-1; i++ {
		l.admitAttempt("alice")
		l.recordFailure("alice")
	}
	l.admitAttempt("alice")
	l.recordSuccess("alice")

	entry := l.entries[normalizeLoginUsername("alice")]
	if len(entry.failures) != 0 {
		t.Fatalf("expected failures cleared after success, got %d", len(entry.failures))
	}
	if entry.backoffLevel != 0 {
		t.Fatalf("expected backoff level reset to 0 after success, got %d", entry.backoffLevel)
	}
	if !entry.lockedUntil.IsZero() {
		t.Fatalf("expected no lockout after success, got lockedUntil=%v", entry.lockedUntil)
	}

	// A fresh admission after success must not be rejected.
	allowed, _ := l.admitAttempt("alice")
	if !allowed {
		t.Fatalf("expected admission to succeed after a clean success")
	}
}

// 2.4: backoff escalates 1m -> 2m across two consecutive lockouts (tripped
// close together in time, so the 24h quiet-reset does not engage), and
// separately resets back to 1m when the entry has been quiet for >24h.
func TestLoginLimiter_BackoffEscalatesAcrossConsecutiveLockouts(t *testing.T) {
	l := newTestLoginLimiter()

	tripStart1 := time.Now()
	fiveFailures(l, "alice")
	entry := l.entries[normalizeLoginUsername("alice")]
	if entry.backoffLevel != 1 {
		t.Fatalf("expected backoff level 1 after first trip, got %d", entry.backoffLevel)
	}
	first := entry.lockedUntil.Sub(tripStart1)
	if first < 50*time.Second || first > 70*time.Second {
		t.Fatalf("expected ~1m lockout after first trip, got %v", first)
	}

	// Simulate the first lockout having expired.
	entry.lockedUntil = time.Now().Add(-time.Second)

	tripStart2 := time.Now()
	fiveFailures(l, "alice")
	if entry.backoffLevel != 2 {
		t.Fatalf("expected backoff level 2 after second trip, got %d", entry.backoffLevel)
	}
	second := entry.lockedUntil.Sub(tripStart2)
	if second < 110*time.Second || second > 130*time.Second {
		t.Fatalf("expected ~2m lockout after second trip, got %v", second)
	}
}

func TestLoginLimiter_BackoffResetsAfter24hQuiet(t *testing.T) {
	l := newTestLoginLimiter()
	now := time.Now()

	// Simulate an entry that already tripped one lockout (backoffLevel=1,
	// so the next trip would normally use the 2m schedule entry) but has
	// since gone quiet for more than the 24h reset window. Four confirmed
	// failures are pre-seeded directly (bypassing recordFailure, which
	// would otherwise refresh lastActivity on every call) so that the 5th,
	// trip-triggering recordFailure call is the one that observes the
	// stale lastActivity.
	entry := &loginLimiterEntry{
		failures:     []time.Time{now, now, now, now},
		backoffLevel: 1,
		lastActivity: now.Add(-25 * time.Hour),
	}
	l.entries[normalizeLoginUsername("alice")] = entry

	tripStart := time.Now()
	l.recordFailure("alice")

	if entry.backoffLevel != 1 {
		t.Fatalf("expected backoff level to land back at 1 (reset to 0, then advanced by this trip), got %d", entry.backoffLevel)
	}
	got := entry.lockedUntil.Sub(tripStart)
	if got < 50*time.Second || got > 70*time.Second {
		t.Fatalf("expected the reset schedule's ~1m lockout, got %v", got)
	}
}

// 2.5: username matching is case-insensitive.
func TestLoginLimiter_UsernameCaseInsensitive(t *testing.T) {
	l := newTestLoginLimiter()
	fiveFailures(l, "Alice")

	allowed, _ := l.admitAttempt("alice")
	if allowed {
		t.Fatalf("expected lockout on 'Alice' to block 'alice'")
	}
	allowed, _ = l.admitAttempt("ALICE")
	if allowed {
		t.Fatalf("expected lockout on 'Alice' to block 'ALICE'")
	}
}

// 2.7: a failure older than the trailing window does not count toward the
// threshold, and retry_after reflects the actual remaining lockout duration
// (not a fixed constant like the capacity-rejection path uses).
func TestLoginLimiter_AgedFailureExcludedFromThreshold(t *testing.T) {
	l := newTestLoginLimiter()
	now := time.Now()
	key := normalizeLoginUsername("alice")
	// 1 aged-out failure + 3 recent ones already on record.
	l.entries[key] = &loginLimiterEntry{
		failures: []time.Time{
			now.Add(-(loginFailureWindow + time.Minute)),
			now, now, now,
		},
		lastActivity: now,
	}

	// A 4th confirmed failure (on top of the 3 recent ones — the aged one
	// must not count) must NOT trip a lockout: only 4 failures are within
	// the window after this call.
	l.admitAttempt("alice")
	l.recordFailure("alice")

	entry := l.entries[key]
	if !entry.lockedUntil.IsZero() {
		t.Fatalf("expected no lockout yet (only 4 in-window failures), got lockedUntil=%v", entry.lockedUntil)
	}
	if len(entry.failures) != 4 {
		t.Fatalf("expected 4 in-window failures (aged one pruned), got %d", len(entry.failures))
	}

	// One more confirmed failure trips the lockout; retry_after must
	// reflect the real ~1m backoff duration, not the fixed 1s used for
	// capacity/ceiling rejections.
	tripStart := time.Now()
	l.admitAttempt("alice")
	l.recordFailure("alice")

	allowed, retryAfter := l.admitAttempt("alice")
	if allowed {
		t.Fatalf("expected lockout after 5th in-window failure")
	}
	elapsed := time.Since(tripStart)
	want := 1*time.Minute - elapsed
	if retryAfter < want-2*time.Second || retryAfter > want+2*time.Second {
		t.Fatalf("expected retryAfter close to remaining ~1m lockout (%v), got %v", want, retryAfter)
	}
}

// 2.8: tripping a lockout clears the failure count, so a single failure
// immediately after the lockout expires does not itself re-trigger a
// lockout.
func TestLoginLimiter_SingleFailureAfterExpiryDoesNotRelock(t *testing.T) {
	l := newTestLoginLimiter()
	fiveFailures(l, "alice")

	entry := l.entries[normalizeLoginUsername("alice")]
	if len(entry.failures) != 0 {
		t.Fatalf("expected failure count cleared on lockout trip, got %d", len(entry.failures))
	}

	// Simulate the lockout expiring.
	entry.lockedUntil = time.Now().Add(-time.Second)

	allowed, _ := l.admitAttempt("alice")
	if !allowed {
		t.Fatalf("expected admission to succeed once the lockout has expired")
	}
	l.recordFailure("alice")

	if entry.lockedUntil.After(time.Now()) {
		t.Fatalf("a single failure right after expiry must not re-trigger a lockout, got lockedUntil=%v", entry.lockedUntil)
	}
	if len(entry.failures) != 1 {
		t.Fatalf("expected exactly 1 failure recorded, got %d", len(entry.failures))
	}
}

// 2.9: flooding the map with distinct, never-repeated throwaway usernames
// past the ceiling must not evict a different, live username's active
// lockout or nonzero failure count.
func TestLoginLimiter_FloodDoesNotEvictLiveEntry(t *testing.T) {
	l := newTestLoginLimiter()

	// A real, actively-locked-out username.
	fiveFailures(l, "victim")
	victimEntry := l.entries[normalizeLoginUsername("victim")]
	if victimEntry.lockedUntil.IsZero() {
		t.Fatalf("expected 'victim' to be locked out")
	}

	// Flood past the ceiling with throwaway usernames.
	for i := 0; i < loginMapCeiling+50; i++ {
		l.admitAttempt(fmt.Sprintf("flood-%d", i))
	}

	got := l.entries[normalizeLoginUsername("victim")]
	if got == nil {
		t.Fatalf("expected 'victim' entry to survive the flood")
	}
	if got.lockedUntil.IsZero() {
		t.Fatalf("expected 'victim' to remain locked out after the flood")
	}
	if len(l.entries) > loginMapCeiling {
		t.Fatalf("expected the map to respect the ceiling, got %d entries", len(l.entries))
	}
}

// 2.10: once the map is saturated with live (non-expired) entries,
// admitAttempt for a brand-new username fails closed (rejected, not
// silently admitted-unrecorded).
func TestLoginLimiter_CeilingFailsClosedWhenAllEntriesLive(t *testing.T) {
	l := newTestLoginLimiter()

	// Saturate the map with live entries: each has a nonzero in-flight
	// counter (admitted, never resolved), so none of them are expired.
	for i := 0; i < loginMapCeiling; i++ {
		l.admitAttempt(fmt.Sprintf("live-%d", i))
	}
	if len(l.entries) != loginMapCeiling {
		t.Fatalf("expected map to be exactly at the ceiling, got %d", len(l.entries))
	}

	allowed, retryAfter := l.admitAttempt("brand-new-victim")
	if allowed {
		t.Fatalf("expected the new username to be rejected (fail closed) when the ceiling is saturated with live entries")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected a positive retryAfter for the fail-closed rejection")
	}
	if len(l.entries) != loginMapCeiling {
		t.Fatalf("expected the ceiling rejection to not grow the map, got %d entries", len(l.entries))
	}
}

// 2.11: admitAttempt is safe under concurrency — firing many goroutines'
// admitAttempt calls for the same never-locked-before username concurrently
// admits at most 5 before later callers see the in-flight capacity
// rejection. This exercises the atomic reservation, not a simulated slow
// credential-verification step.
func TestLoginLimiter_ConcurrentAdmitAttemptCapsAtFive(t *testing.T) {
	l := newTestLoginLimiter()

	const n = 50
	results := make([]bool, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			allowed, _ := l.admitAttempt("concurrent-user")
			results[i] = allowed
		}(i)
	}
	wg.Wait()

	admitted := 0
	for _, ok := range results {
		if ok {
			admitted++
		}
	}
	if admitted != loginLockoutThreshold {
		t.Fatalf("expected exactly %d admitted attempts out of %d concurrent callers, got %d", loginLockoutThreshold, n, admitted)
	}
}

// 2.14: an entry with a pending in-flight attempt is never evicted by the
// sweep or by the ceiling, even though it has zero confirmed failures, no
// lockout, and an unset lastActivity (a brand-new username's very first
// admitted attempt).
func TestLoginLimiter_InFlightEntryNeverEvicted(t *testing.T) {
	t.Run("sweep", func(t *testing.T) {
		l := newTestLoginLimiter()
		allowed, _ := l.admitAttempt("brand-new")
		if !allowed {
			t.Fatalf("expected the first attempt to be admitted")
		}

		// Populate a batch of clearly-expired entries and drive the sweep
		// via further admitAttempt calls.
		for i := 0; i < loginSweepBatchSize+5; i++ {
			key := fmt.Sprintf("expired-%d", i)
			l.entries[normalizeLoginUsername(key)] = &loginLimiterEntry{
				lastActivity: time.Now().Add(-(loginQuietReset + time.Hour)),
			}
		}
		l.admitAttempt("someone-else")

		entry, ok := l.entries[normalizeLoginUsername("brand-new")]
		if !ok {
			t.Fatalf("expected the in-flight entry to survive the sweep")
		}
		if entry.inFlight != 1 {
			t.Fatalf("expected inFlight=1 to be preserved, got %d", entry.inFlight)
		}

		// The pending attempt must still resolve against the same entry.
		l.recordSuccess("brand-new")
		if entry.inFlight != 0 {
			t.Fatalf("expected recordSuccess to resolve the same entry's in-flight counter, got %d", entry.inFlight)
		}
	})

	t.Run("ceiling", func(t *testing.T) {
		l := newTestLoginLimiter()
		allowed, _ := l.admitAttempt("brand-new")
		if !allowed {
			t.Fatalf("expected the first attempt to be admitted")
		}

		// Saturate the map with expired entries, forcing ceiling eviction
		// on every subsequent new username.
		for i := 0; i < loginMapCeiling; i++ {
			key := fmt.Sprintf("expired-%d", i)
			l.entries[normalizeLoginUsername(key)] = &loginLimiterEntry{
				lastActivity: time.Now().Add(-(loginQuietReset + time.Hour)),
			}
		}
		l.admitAttempt("yet-another-new-user")

		entry, ok := l.entries[normalizeLoginUsername("brand-new")]
		if !ok {
			t.Fatalf("expected the in-flight entry to survive ceiling eviction")
		}

		l.recordFailure("brand-new")
		if entry.inFlight != 0 {
			t.Fatalf("expected recordFailure to resolve the same entry's in-flight counter, got %d", entry.inFlight)
		}
		if len(entry.failures) != 1 {
			t.Fatalf("expected the confirmed failure to land on the same entry, got %d failures", len(entry.failures))
		}
	})
}

// Review finding 1 (regression): a flood of *resolved* failures — one
// confirmed failed attempt each for ceiling-many throwaway usernames — must
// not deny admission to a username the limiter has never seen. This is the
// shape age-only eviction could not evict: every entry has a zero in-flight
// counter but recent activity, so for loginQuietReset nothing was evictable
// and every new username was rejected before credential verification ran.
func TestLoginLimiter_ResolvedFailureFloodDoesNotDenyNewUsernames(t *testing.T) {
	l := newTestLoginLimiter()

	for i := 0; i < loginMapCeiling; i++ {
		u := fmt.Sprintf("junk-%d", i)
		l.admitAttempt(u)
		l.recordFailure(u)
	}
	if len(l.entries) != loginMapCeiling {
		t.Fatalf("expected the flood to fill the map to the ceiling, got %d entries", len(l.entries))
	}

	allowed, retryAfter := l.admitAttempt("brand-new")
	if !allowed {
		t.Fatalf("a never-seen username must still be admitted after a resolved-failure flood (retryAfter %v)", retryAfter)
	}
	if len(l.entries) > loginMapCeiling {
		t.Fatalf("expected the ceiling to hold, got %d entries", len(l.entries))
	}
}

// Review finding 1: the flood displaces its own junk before it touches an
// active lockout — evictTierFailures sorts below evictTierLocked.
func TestLoginLimiter_LockoutOutlivesResolvedFailureFlood(t *testing.T) {
	l := newTestLoginLimiter()

	fiveFailures(l, "victim")
	if l.entries[normalizeLoginUsername("victim")].lockedUntil.IsZero() {
		t.Fatalf("expected 'victim' to be locked out")
	}

	for i := 0; i < loginMapCeiling+50; i++ {
		u := fmt.Sprintf("junk-%d", i)
		l.admitAttempt(u)
		l.recordFailure(u)
	}

	got := l.entries[normalizeLoginUsername("victim")]
	if got == nil {
		t.Fatalf("expected 'victim' to survive a resolved-failure flood")
	}
	if got.lockedUntil.IsZero() {
		t.Fatalf("expected 'victim' to remain locked out after the flood")
	}
}

// Review finding 2: a successful login records activity, so a legitimate
// account holds its map slot instead of being swept on the next admission.
// While it did not, no real account was ever present in the map, which is
// what let the flood above deny login to everyone rather than only to
// usernames not seen before.
func TestLoginLimiter_SuccessfulLoginHoldsItsSlot(t *testing.T) {
	l := newTestLoginLimiter()

	l.admitAttempt("alice")
	l.recordSuccess("alice")

	entry := l.entries[normalizeLoginUsername("alice")]
	if entry == nil {
		t.Fatalf("expected an entry for 'alice' after a successful login")
	}
	if entry.lastActivity.IsZero() {
		t.Fatalf("expected recordSuccess to record activity")
	}
	if entry.isExpired(time.Now()) {
		t.Fatalf("a just-succeeded entry must not count as expired")
	}

	// Drive a sweep via another username's admission and confirm it survives.
	l.admitAttempt("someone-else")
	if l.entries[normalizeLoginUsername("alice")] == nil {
		t.Fatalf("expected 'alice' to survive the sweep after a successful login")
	}
}
