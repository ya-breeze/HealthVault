package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/auth"
	"github.com/ya-breeze/kin-core/cookies"
)

// responseCookie returns the named cookie a handler set, or fails the test.
func responseCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) string {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	t.Fatalf("no %s cookie on the response", name)
	return ""
}

// accessCookie returns the kin_access cookie a handler set, or fails the test.
func accessCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return responseCookie(t, rec, cookies.AccessCookieName)
}

// Two tokens for one user inside the same second must differ. The library's
// GenerateAccessToken makes them identical — claims of {UserID, FamilyID,
// IssuedAt, ExpiresAt} at one-second resolution, signed deterministically —
// which is what lets a blacklist entry for one session revoke another.
func TestGenerateAccessToken_TokensMintedInTheSameSecondDiffer(t *testing.T) {
	userID, familyID := uuid.New(), uuid.New()
	secret := []byte("test-secret")

	first, err := generateAccessToken(userID, &familyID, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("generateAccessToken (first): %v", err)
	}
	second, err := generateAccessToken(userID, &familyID, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("generateAccessToken (second): %v", err)
	}

	if first == second {
		t.Fatal("two tokens minted for the same user in the same second are identical; " +
			"a blacklist keyed on the token string cannot then tell two sessions apart")
	}

	// Distinct, but still the same session identity and still valid.
	for name, token := range map[string]string{"first": first, "second": second} {
		claims, err := auth.ParseToken(token, secret)
		if err != nil {
			t.Fatalf("ParseToken (%s): %v", name, err)
		}
		if claims.UserID != userID {
			t.Errorf("%s token: UserID = %v, want %v", name, claims.UserID, userID)
		}
		if claims.FamilyID == nil || *claims.FamilyID != familyID {
			t.Errorf("%s token: FamilyID = %v, want %v", name, claims.FamilyID, familyID)
		}
		if claims.RegisteredClaims.ID == "" {
			t.Errorf("%s token: no jti; uniqueness would depend on the clock again", name)
		}
	}
}

// The user-facing case: log out, then log straight back in. The second login's
// token must not be the one logout revoked, or the new session 401s on every
// request until the wall clock ticks into the next second — and cannot refresh
// its way out, because Refresh mints from the same inputs.
func TestLogin_AfterLogoutInTheSameSecondIsNotRevoked(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	username := "relogin-" + uuid.NewString()
	createLoginTestUser(t, storage, username, "correct-password")

	first := doLogin(h, username, "correct-password")
	if first.Code != http.StatusOK {
		t.Fatalf("first login: expected 200, got %d", first.Code)
	}
	firstToken := accessCookie(t, first)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: cookies.AccessCookieName, Value: firstToken})
	logoutRec := httptest.NewRecorder()
	h.Logout(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", logoutRec.Code)
	}

	// No sleep: landing in the same second as the logout is the whole point.
	second := doLogin(h, username, "correct-password")
	if second.Code != http.StatusOK {
		t.Fatalf("second login: expected 200, got %d", second.Code)
	}
	secondToken := accessCookie(t, second)

	if secondToken == firstToken {
		t.Fatal("the re-login reissued the token logout just blacklisted")
	}

	// The check RequireAuth performs, against the same blacklist logout wrote.
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.AddCookie(&http.Cookie{Name: cookies.AccessCookieName, Value: secondToken})
	rec := httptest.NewRecorder()
	authorized := false
	RequireAuth(h.jwtSecret, h.cookieCfg, h.db)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { authorized = true }),
	).ServeHTTP(rec, req)

	if !authorized || rec.Code == http.StatusUnauthorized {
		t.Fatalf("the new session was rejected as revoked (status %d); "+
			"logging out and back in inside one second must not kill the new session", rec.Code)
	}

	// The revocation itself must still work: the logged-out token stays dead.
	revokedReq := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	revokedReq.AddCookie(&http.Cookie{Name: cookies.AccessCookieName, Value: firstToken})
	revokedRec := httptest.NewRecorder()
	stillAccepted := false
	RequireAuth(h.jwtSecret, h.cookieCfg, h.db)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { stillAccepted = true }),
	).ServeHTTP(revokedRec, revokedReq)

	if stillAccepted || revokedRec.Code != http.StatusUnauthorized {
		t.Fatalf("the logged-out token was still accepted (status %d)", revokedRec.Code)
	}
}
