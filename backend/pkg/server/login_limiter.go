package server

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// afterAdmitHook is an unexported test seam. Login calls it immediately after
// a successful admission and before credential verification begins. nil in
// production (zero cost). Tests set it to make concurrent-admission overlap
// deterministic instead of relying on goroutine-scheduling timing — see
// design.md's "Test seam for deterministic concurrency tests" note.
var afterAdmitHook func()

const (
	// loginFailureWindow is the trailing window confirmed failures count
	// toward the lockout threshold.
	loginFailureWindow = 15 * time.Minute
	// loginLockoutThreshold is the number of confirmed failures (or
	// confirmed-failures-plus-in-flight-attempts) that trips a lockout /
	// saturates admission capacity.
	loginLockoutThreshold = 5
	// loginQuietReset is how long a username must have no activity before
	// its exponential backoff level resets back to the start of the
	// schedule.
	loginQuietReset = 24 * time.Hour
	// loginMapCeiling is the hard cap on tracked usernames, an order of
	// magnitude above this project's realistic account count.
	loginMapCeiling = 1000
	// loginSweepBatchSize bounds how many expired entries are reaped per
	// admitAttempt call, so a large backlog of expired entries can't turn
	// one call into an unbounded scan-and-delete.
	loginSweepBatchSize = 16
	// loginCapacityRetryAfter is the fixed retry-after used for the two
	// rejection cases that are not a timed lockout: in-flight capacity
	// saturation and ceiling fail-closed.
	loginCapacityRetryAfter = 1 * time.Second
)

// loginBackoffSchedule is the exponential backoff schedule applied on each
// subsequent lockout for the same username, capped at the last entry.
var loginBackoffSchedule = []time.Duration{
	1 * time.Minute,
	2 * time.Minute,
	4 * time.Minute,
	8 * time.Minute,
	16 * time.Minute,
	30 * time.Minute,
}

// loginLimiterEntry tracks one username's login-attempt state. All fields
// are guarded by loginLimiter.mu.
type loginLimiterEntry struct {
	// failures holds confirmed-failure timestamps within the trailing
	// window; pruned lazily.
	failures []time.Time
	// inFlight is the number of admitted attempts currently inside
	// credential verification, not yet resolved by recordFailure/recordSuccess.
	inFlight int
	// backoffLevel indexes loginBackoffSchedule for the *next* lockout to
	// be tripped for this username.
	backoffLevel int
	// lockedUntil is the time the current lockout (if any) expires.
	lockedUntil time.Time
	// lastActivity is updated on every confirmed failure and on lockout
	// trip. It is never cleared by the trailing-window reset. Together
	// with inFlight, it defines "expired" for sweep/ceiling eviction: an
	// entry is expired only if lastActivity is well past the quiet-reset
	// window and inFlight is zero.
	lastActivity time.Time
}

// isExpired reports whether the entry is eligible for sweep/ceiling
// eviction. A nonzero in-flight counter always makes an entry non-expired,
// regardless of lastActivity, so an attempt still inside credential
// verification is never evicted out from under itself.
func (e *loginLimiterEntry) isExpired(now time.Time) bool {
	return e.inFlight == 0 && now.Sub(e.lastActivity) > loginQuietReset
}

// pruneFailures drops failure timestamps older than the trailing window.
func (e *loginLimiterEntry) pruneFailures(now time.Time) {
	cutoff := now.Add(-loginFailureWindow)
	kept := e.failures[:0]
	for _, t := range e.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	e.failures = kept
}

// loginLimiter is a mutex-guarded, package-level login attempt limiter,
// keyed by lowercased username. See design.md's "Login attempt limiting"
// decision for the full rationale.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginLimiterEntry
}

var globalLoginLimiter = &loginLimiter{entries: make(map[string]*loginLimiterEntry)}

func normalizeLoginUsername(username string) string {
	return strings.ToLower(username)
}

// sweepLocked removes up to loginSweepBatchSize of the oldest expired
// entries. Caller must hold l.mu.
func (l *loginLimiter) sweepLocked(now time.Time) {
	type candidate struct {
		key          string
		lastActivity time.Time
	}
	var expired []candidate
	for k, e := range l.entries {
		if e.isExpired(now) {
			expired = append(expired, candidate{k, e.lastActivity})
		}
	}
	if len(expired) == 0 {
		return
	}
	sort.Slice(expired, func(i, j int) bool {
		return expired[i].lastActivity.Before(expired[j].lastActivity)
	})
	n := loginSweepBatchSize
	if n > len(expired) {
		n = len(expired)
	}
	for i := 0; i < n; i++ {
		delete(l.entries, expired[i].key)
	}
}

// evictOldestExpiredLocked removes a single oldest expired entry to make
// room at the ceiling. Returns true if an entry was evicted. Caller must
// hold l.mu. Never evicts a live entry (active lockout, nonzero trailing
// failure count, or nonzero in-flight counter) — those never satisfy
// isExpired.
func (l *loginLimiter) evictOldestExpiredLocked(now time.Time) bool {
	var oldestKey string
	var oldestActivity time.Time
	found := false
	for k, e := range l.entries {
		if !e.isExpired(now) {
			continue
		}
		if !found || e.lastActivity.Before(oldestActivity) {
			oldestKey = k
			oldestActivity = e.lastActivity
			found = true
		}
	}
	if found {
		delete(l.entries, oldestKey)
	}
	return found
}

// admitAttempt atomically reserves in-flight capacity for a login attempt
// against username. It rejects immediately with the existing lockout's
// retryAfter if already locked, else rejects with a fixed 1s retryAfter if
// confirmed-failure-count + in-flight-count is already saturated, else
// admits the attempt (incrementing the in-flight counter) and returns
// allowed = true.
func (l *loginLimiter) admitAttempt(username string) (allowed bool, retryAfter time.Duration) {
	now := time.Now()
	key := normalizeLoginUsername(username)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweepLocked(now)

	entry, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= loginMapCeiling {
			if !l.evictOldestExpiredLocked(now) {
				return false, loginCapacityRetryAfter
			}
		}
		entry = &loginLimiterEntry{}
		l.entries[key] = entry
	}

	if entry.lockedUntil.After(now) {
		return false, entry.lockedUntil.Sub(now)
	}

	entry.pruneFailures(now)
	if len(entry.failures)+entry.inFlight >= loginLockoutThreshold {
		return false, loginCapacityRetryAfter
	}

	entry.inFlight++
	return true, 0
}

// recordFailure resolves one admitted attempt as a confirmed failure:
// decrements the in-flight counter, then advances the confirmed failure
// count/timestamp, tripping a new lockout right there if the confirmed
// count reaches the threshold.
func (l *loginLimiter) recordFailure(username string) {
	now := time.Now()
	key := normalizeLoginUsername(username)

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok {
		return
	}
	if entry.inFlight > 0 {
		entry.inFlight--
	}

	prevActivity := entry.lastActivity
	entry.pruneFailures(now)
	entry.failures = append(entry.failures, now)
	entry.lastActivity = now

	if len(entry.failures) >= loginLockoutThreshold {
		l.tripLockoutLocked(entry, now, prevActivity)
	}
}

// tripLockoutLocked trips a new lockout for entry, applying and escalating
// the exponential backoff schedule, resetting it first if the username has
// had no activity at all for loginQuietReset. Clears the trailing-window
// failure list (not the backoff level) per design.md's "Reset on lockout
// trip" note. Caller must hold l.mu.
func (l *loginLimiter) tripLockoutLocked(entry *loginLimiterEntry, now, prevActivity time.Time) {
	if !prevActivity.IsZero() && now.Sub(prevActivity) > loginQuietReset {
		entry.backoffLevel = 0
	}
	idx := entry.backoffLevel
	if idx >= len(loginBackoffSchedule) {
		idx = len(loginBackoffSchedule) - 1
	}
	entry.lockedUntil = now.Add(loginBackoffSchedule[idx])
	if entry.backoffLevel < len(loginBackoffSchedule)-1 {
		entry.backoffLevel++
	}
	entry.failures = entry.failures[:0]
}

// recordSuccess resolves one admitted attempt as a success: decrements the
// in-flight counter, then clears the confirmed failure count and backoff
// level entirely.
func (l *loginLimiter) recordSuccess(username string) {
	key := normalizeLoginUsername(username)

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok {
		return
	}
	if entry.inFlight > 0 {
		entry.inFlight--
	}
	entry.failures = entry.failures[:0]
	entry.backoffLevel = 0
	entry.lockedUntil = time.Time{}
}

// ceilRetryAfter converts a duration to whole seconds, rounded up.
func ceilRetryAfter(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	secs := int(d / time.Second)
	if d%time.Second != 0 {
		secs++
	}
	return secs
}

func admitAttempt(username string) (allowed bool, retryAfter time.Duration) {
	return globalLoginLimiter.admitAttempt(username)
}

func recordFailure(username string) {
	globalLoginLimiter.recordFailure(username)
}

func recordSuccess(username string) {
	globalLoginLimiter.recordSuccess(username)
}
