package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/auth"
	"github.com/ya-breeze/kin-core/authdb"
	"github.com/ya-breeze/kin-core/cookies"
	kinmodels "github.com/ya-breeze/kin-core/models"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func newLoginTestHandlers(t *testing.T) (*authHandlers, database.Storage) {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// go-sqlite3's ":memory:" DSN gives each physical connection its own,
	// independent in-memory database. Concurrent goroutines (2.13's
	// concurrent-login test) can otherwise land on a second connection that
	// never saw the migrated schema or seeded rows. Pin the pool to a
	// single connection so all queries share the same in-memory database.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	storage := database.NewStorage(db)
	return &authHandlers{
		storage:   storage,
		db:        db,
		jwtSecret: []byte("test-secret"),
		cookieCfg: cookies.Config{},
	}, storage
}

func createLoginTestUser(t *testing.T, storage database.Storage, username, password string) *kinmodels.User {
	t.Helper()
	familyID := uuid.New()
	if err := storage.DB().Create(&kinmodels.Family{ID: familyID, Name: "TestFamily"}).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := kinmodels.User{ID: uuid.New(), Username: username, PasswordHash: hash, FamilyID: familyID}
	if err := storage.DB().Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return &user
}

func doLogin(h *authHandlers, username, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password}) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	return rec
}

// 2.2: an unknown username accumulates failures identically to a known
// username with a wrong password — both trip a 429 lockout after 5 failed
// attempts.
func TestLogin_UnknownUsernameAccumulatesFailuresLikeKnownUser(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	knownWrongUser := "known-wrong-" + uuid.NewString()
	createLoginTestUser(t, storage, knownWrongUser, "correct-password")
	unknownUser := "unknown-" + uuid.NewString()

	for i := 0; i < loginLockoutThreshold; i++ {
		rec := doLogin(h, knownWrongUser, "wrong-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("known-username wrong-password attempt %d: expected 401, got %d", i, rec.Code)
		}
	}
	if rec := doLogin(h, knownWrongUser, "correct-password"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected known-wrong-password user to be locked out (429), got %d", rec.Code)
	}

	for i := 0; i < loginLockoutThreshold; i++ {
		rec := doLogin(h, unknownUser, "whatever")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unknown-username attempt %d: expected 401, got %d", i, rec.Code)
		}
	}
	if rec := doLogin(h, unknownUser, "whatever"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected unknown username to be locked out identically (429), got %d", rec.Code)
	}
}

// 2.6: Refresh, Logout, and a RequireAuth-protected request are unaffected
// by a concurrent lockout on the same username — none of them consult the
// login limiter.
func TestLogin_RefreshLogoutRequireAuthUnaffectedByConcurrentLockout(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	username := "victim-" + uuid.NewString()
	user := createLoginTestUser(t, storage, username, "correct-password")

	for i := 0; i < loginLockoutThreshold; i++ {
		doLogin(h, username, "wrong-password")
	}
	if rec := doLogin(h, username, "correct-password"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected the username to be locked out, got %d", rec.Code)
	}

	rt, err := authdb.CreateRefreshToken(storage.DB(), user.ID, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	refreshReq.AddCookie(&http.Cookie{Name: cookies.RefreshCookieName, Value: rt.Token})
	refreshRec := httptest.NewRecorder()
	h.Refresh(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusNoContent {
		t.Fatalf("expected Refresh to succeed despite the username's active lockout, got %d", refreshRec.Code)
	}

	// Checked before Logout runs (below), which blacklists whatever token it
	// is handed — GenerateAccessToken's claims have second-precision
	// timestamps, so a token minted moments later for the same user could
	// otherwise be byte-identical and get blacklisted along with it.
	accessToken, err := auth.GenerateAccessToken(user.ID, &user.FamilyID, h.jwtSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	protected := RequireAuth(h.jwtSecret, h.cookieCfg, storage.DB())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	protectedReq := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	protectedReq.AddCookie(&http.Cookie{Name: cookies.AccessCookieName, Value: accessToken})
	protectedRec := httptest.NewRecorder()
	protected.ServeHTTP(protectedRec, protectedReq)
	if protectedRec.Code != http.StatusOK {
		t.Fatalf("expected a RequireAuth-protected request to succeed despite the username's active lockout, got %d", protectedRec.Code)
	}

	logoutToken, err := auth.GenerateAccessToken(user.ID, &user.FamilyID, h.jwtSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: cookies.AccessCookieName, Value: logoutToken})
	logoutRec := httptest.NewRecorder()
	h.Logout(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected Logout to succeed despite the username's active lockout, got %d", logoutRec.Code)
	}
}

// 2.12: Login with an unknown username still runs a real bcrypt compare
// against auth.DummyHash, so an unknown-username 401 is not measurably
// faster than a wrong-password 401 for a known user.
func TestLogin_UnknownUsernameRunsDummyBcryptCompare(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	knownUser := "known-" + uuid.NewString()
	createLoginTestUser(t, storage, knownUser, "correct-password")
	unknownUser := "unknown-" + uuid.NewString()

	start := time.Now()
	rec := doLogin(h, knownUser, "wrong-password")
	knownDur := time.Since(start)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected known-wrong-password 401, got %d", rec.Code)
	}

	start = time.Now()
	rec = doLogin(h, unknownUser, "whatever-password")
	unknownDur := time.Since(start)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown-username 401, got %d", rec.Code)
	}

	if unknownDur < time.Millisecond {
		t.Fatalf("expected the unknown-username path to run a real bcrypt compare, got suspiciously fast %v", unknownDur)
	}
	if unknownDur < knownDur/3 || unknownDur > knownDur*3 {
		t.Fatalf("expected unknown-username 401 timing (%v) to be comparable to known-wrong-password 401 timing (%v), consistent with running the same dummy bcrypt compare", unknownDur, knownDur)
	}
}

// 2.13: concurrent correct-password login requests never trip a real
// lockout. Uses the afterAdmitHook test seam (1.5) to make the overlap
// deterministic: 5 concurrent, correct-password Login calls are held right
// after admission (in-flight counter pinned at 5), a 6th concurrent call for
// the same username is issued only once all 5 are confirmed held (so it
// deterministically observes the transient in-flight capacity rejection,
// not a real lockout), then the 5 held calls are released to succeed.
func TestLogin_ConcurrentCorrectPasswordNeverTripsLockout(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	username := "concurrent-ok-" + uuid.NewString()
	createLoginTestUser(t, storage, username, "goodpass")

	admitted := make(chan struct{})
	release := make(chan struct{})
	orig := afterAdmitHook
	afterAdmitHook = func() {
		admitted <- struct{}{}
		<-release
	}
	defer func() { afterAdmitHook = orig }()

	results := make(chan int, 5)
	for i := 0; i < loginLockoutThreshold; i++ {
		go func() {
			results <- doLogin(h, username, "goodpass").Code
		}()
	}

	for i := 0; i < loginLockoutThreshold; i++ {
		<-admitted
	}

	sixthRec := doLogin(h, username, "goodpass")
	if sixthRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected the 6th concurrent login to hit the in-flight capacity rejection (429), got %d", sixthRec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(sixthRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if body["retry_after_seconds"] != float64(1) {
		t.Fatalf("expected the capacity rejection's fixed 1s retry_after_seconds (not a real lockout's backoff), got %v", body["retry_after_seconds"])
	}

	close(release)

	for i := 0; i < loginLockoutThreshold; i++ {
		if code := <-results; code != http.StatusOK {
			t.Fatalf("expected a held goroutine to succeed once released, got %d", code)
		}
	}

	// Restore the hook before this call: the receiving loop above has
	// already taken its 5 signals and stopped listening, so a hook still
	// installed here would send to "admitted" with no reader left and hang.
	afterAdmitHook = orig

	finalRec := doLogin(h, username, "goodpass")
	if finalRec.Code != http.StatusOK {
		t.Fatalf("expected a follow-up login to succeed — no real lockout should ever have been tripped, got %d", finalRec.Code)
	}
}

// Review finding 5: every 429 carries the standard Retry-After header, not
// just the JSON body. The widget refreshes unattended, and HTTP clients and
// proxies back off on the header without knowing this endpoint's body shape.
func TestLogin_LockoutSetsRetryAfterHeader(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	createLoginTestUser(t, storage, "header-user", "correct-password")

	for i := 0; i < loginLockoutThreshold; i++ {
		doLogin(h, "header-user", "wrong-password")
	}

	rec := doLogin(h, "header-user", "correct-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once locked out, got %d", rec.Code)
	}
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatalf("expected a Retry-After header on the 429")
	}
	secs, err := strconv.Atoi(retryAfter)
	if err != nil || secs <= 0 {
		t.Fatalf("expected Retry-After to be a positive whole number of seconds, got %q", retryAfter)
	}

	var body struct {
		RetryAfterSeconds int `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if body.RetryAfterSeconds != secs {
		t.Fatalf("header and body disagree: header %d, body %d", secs, body.RetryAfterSeconds)
	}
}

// Review finding 4: the per-username limiter bounds one account's attempts,
// not the process's total bcrypt work — every unknown username runs a
// full-cost compare before its 401. Attempts beyond loginVerifyConcurrency
// are rejected with the standard 429 shape before verification begins, and
// the reservation each one took is released so no phantom in-flight attempts
// are left behind.
func TestLogin_GlobalVerifyCapRejectsExcessConcurrency(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	createLoginTestUser(t, storage, "cap-user", "correct-password")

	admitted := make(chan struct{})
	release := make(chan struct{})
	orig := afterAdmitHook
	afterAdmitHook = func() {
		admitted <- struct{}{}
		<-release
	}
	defer func() { afterAdmitHook = orig }()

	// Hold every verification slot open, each under its own username so the
	// per-username in-flight cap is never what does the rejecting.
	var wg sync.WaitGroup
	for i := 0; i < loginVerifyConcurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			doLogin(h, fmt.Sprintf("holder-%d", i), "wrong-password")
		}(i)
	}
	for i := 0; i < loginVerifyConcurrency; i++ {
		<-admitted
	}

	rec := doLogin(h, "cap-user", "correct-password")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once every verification slot is held, got %d", rec.Code)
	}
	var body struct {
		Error             string `json:"error"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if body.Error != "too_many_attempts" || body.RetryAfterSeconds <= 0 {
		t.Fatalf("expected the standard 429 shape, got %+v", body)
	}
	if entry := globalLoginLimiter.entries[normalizeLoginUsername("cap-user")]; entry != nil && entry.inFlight != 0 {
		t.Fatalf("expected the rejected attempt to release its reservation, in-flight is %d", entry.inFlight)
	}

	close(release)
	wg.Wait()
	afterAdmitHook = orig

	// Slots are released, so a correct password gets through as normal.
	if rec := doLogin(h, "cap-user", "correct-password"); rec.Code != http.StatusOK {
		t.Fatalf("expected a normal login to succeed once the slots are freed, got %d", rec.Code)
	}
}
