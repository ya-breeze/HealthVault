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
	// loginCapacityRetryAfter is the fixed retry-after used for the
	// rejection cases that are not a timed lockout: in-flight capacity
	// saturation, the global verification cap, and ceiling fail-closed.
	loginCapacityRetryAfter = 1 * time.Second
	// loginVerifyConcurrency bounds how many credential verifications may
	// run at once across *all* usernames. The per-username limiter does not
	// bound total work: every unknown username runs a full-cost bcrypt
	// compare before its 401 (that is what closes the enumeration oracle),
	// so without this a cheap request from each of many distinct usernames
	// buys unbounded server CPU. Sized well above this project's realistic
	// concurrent-login count and well below loginMapCeiling — the latter
	// matters, because it is what keeps the ceiling's fail-closed path
	// unreachable: only an entry with an attempt in flight is inevictable,
	// and at most this many can be in flight at once.
	loginVerifyConcurrency = 8
)

// Eviction tiers, ranked by how much protection is lost by evicting the
// entry: lower is more disposable. The ceiling evicts the lowest-tier entry
// available, oldest-first within a tier, so a flood of throwaway usernames
// displaces its own junk long before it reaches anything real. The top two
// tiers are never evicted at all.
//
// Ranking by tier rather than by age alone is not a refinement, it is the
// fix for a login denial-of-service: with age as the only criterion, one
// failed attempt against each of loginMapCeiling throwaway usernames filled
// the map with entries nothing could evict for loginQuietReset, and every
// username without an existing entry was then rejected before credential
// verification ran — 1000 cheap requests, 24 hours of nobody logging in.
//
// An active lockout stays inevictable, so what remains possible is far more
// expensive and far shorter-lived: an attacker must trip and keep tripping
// loginMapCeiling separate lockouts (five confirmed failures each, every one
// of them paying full bcrypt cost through loginVerifyConcurrency) to force
// the fail-closed path, and it lasts only until those lockouts expire.
// Sacrificing a lockout instead would hand back exactly the targeted
// credential-stuffing protection this limiter exists to provide.
const (
	// evictTierExpired: quiet past the reset window — nothing left to protect.
	evictTierExpired = iota
	// evictTierIdle: no lockout and no failures in the window; only the
	// backoff level survives, and only until loginQuietReset would clear it.
	evictTierIdle
	// evictTierFailures: partial progress toward a lockout. Evicting one
	// costs an attacker's own accumulated failures as readily as a real
	// user's, and neither has protection to lose yet.
	evictTierFailures
	// evictTierLocked: an active lockout. Never evicted — it is the
	// protection, and it expires on its own.
	evictTierLocked
	// evictTierPinned: an admitted attempt is still inside credential
	// verification. Never evicted: dropping the entry would discard the
	// decrement its resolver is about to make.
	evictTierPinned

	// evictTierProtected is the first inevictable tier: an entry at or above
	// it is kept even when that means rejecting a brand-new username.
	evictTierProtected = evictTierLocked
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
	// lastActivity is updated on every confirmed failure, on lockout trip,
	// and on a successful login. It is never cleared by the trailing-window
	// reset. Together with inFlight, it defines "expired" for sweep/ceiling
	// eviction: an entry is expired only if lastActivity is well past the
	// quiet-reset window and inFlight is zero.
	//
	// Success updates it for the same reason failure does: an account that
	// logs in normally has to hold its slot. While it did not, every
	// successful login left an entry that was immediately expired and swept
	// on the next admitAttempt, so no legitimate account was ever present
	// in the map — which is what let a flood of throwaway usernames deny
	// login to *everyone* rather than only to usernames not seen before.
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

// evictionTier ranks the entry for ceiling eviction; see the tier constants
// for what each means and why the ranking exists. Caller must hold l.mu
// (pruneFailures mutates the entry).
func (e *loginLimiterEntry) evictionTier(now time.Time) int {
	if e.inFlight > 0 {
		return evictTierPinned
	}
	if e.lockedUntil.After(now) {
		return evictTierLocked
	}
	e.pruneFailures(now)
	if len(e.failures) > 0 {
		return evictTierFailures
	}
	if now.Sub(e.lastActivity) > loginQuietReset {
		return evictTierExpired
	}
	return evictTierIdle
}

// evictOneLocked removes a single most-disposable entry to make room at the
// ceiling: the lowest eviction tier present, and within that tier the one
// idle longest. Returns false when every entry is at or above
// evictTierProtected, which is the fail-closed path. Caller must hold l.mu.
func (l *loginLimiter) evictOneLocked(now time.Time) bool {
	var bestKey string
	var bestActivity time.Time
	bestTier := evictTierProtected
	found := false
	for k, e := range l.entries {
		tier := e.evictionTier(now)
		if tier >= evictTierProtected {
			continue
		}
		if !found || tier < bestTier || (tier == bestTier && e.lastActivity.Before(bestActivity)) {
			bestKey = k
			bestTier = tier
			bestActivity = e.lastActivity
			found = true
		}
	}
	if found {
		delete(l.entries, bestKey)
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
			if !l.evictOneLocked(now) {
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
	entry.failures = entry.failures[:0]
	entry.backoffLevel = 0
	entry.lockedUntil = time.Time{}
	entry.lastActivity = now
}

// releaseAttempt resolves one admitted attempt as neither a confirmed
// failure nor a success: it only decrements the in-flight counter. Used when
// credential verification could not run at all (e.g. an operational DB
// error), so a transient outage never counts toward a username's lockout
// threshold or clears its existing failure/backoff state.
func (l *loginLimiter) releaseAttempt(username string) {
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

func releaseAttempt(username string) {
	globalLoginLimiter.releaseAttempt(username)
}

// loginVerifySlots bounds concurrent credential verifications process-wide.
// A buffered channel rather than a counter because the bound must be
// acquired without blocking: an attempt that cannot get a slot is rejected
// with the standard 429 shape, not queued behind bcrypt work that an
// attacker chose the volume of.
var loginVerifySlots = make(chan struct{}, loginVerifyConcurrency)

// acquireVerifySlot reserves one of the process-wide verification slots,
// reporting false immediately if all of them are taken.
func acquireVerifySlot() bool {
	select {
	case loginVerifySlots <- struct{}{}:
		return true
	default:
		return false
	}
}

// releaseVerifySlot returns a slot taken by acquireVerifySlot. Only ever
// called after a successful acquire, so the receive cannot block.
func releaseVerifySlot() {
	<-loginVerifySlots
}
