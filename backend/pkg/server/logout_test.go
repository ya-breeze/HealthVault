package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/cookies"
)

// doRefresh posts the given refresh token to Refresh and returns the recorder,
// so callers can read both the status and the rotated cookie it set.
func doRefresh(h *authHandlers, refreshToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: cookies.RefreshCookieName, Value: refreshToken})
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)
	return rec
}

// Logout must end the session, not just the next 15 minutes of it.
//
// Clearing the cookies is a client-side gesture: anyone who kept a copy of the
// refresh token value can still POST /api/auth/refresh with it. The refresh
// token's TTL is a year (Login), so without an explicit revocation the session
// logout claims to have ended outlives the logout by that year.
func TestLogout_RevokesTheRefreshToken(t *testing.T) {
	h, storage := newLoginTestHandlers(t)
	username := "logout-" + uuid.NewString()
	createLoginTestUser(t, storage, username, "correct-password")

	login := doLogin(h, username, "correct-password")
	if login.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", login.Code)
	}
	accessToken := responseCookie(t, login, cookies.AccessCookieName)

	// Refreshing once first serves two purposes: it proves the token works
	// before logout, so a 401 afterwards can only be the revocation; and it
	// rotates the value, so the test logs out with the token a real browser
	// would be holding rather than the one login issued.
	refreshed := doRefresh(h, responseCookie(t, login, cookies.RefreshCookieName))
	if refreshed.Code != http.StatusNoContent {
		t.Fatalf("refresh before logout: expected 204, got %d", refreshed.Code)
	}
	refreshToken := responseCookie(t, refreshed, cookies.RefreshCookieName)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: cookies.AccessCookieName, Value: accessToken})
	logoutReq.AddCookie(&http.Cookie{Name: cookies.RefreshCookieName, Value: refreshToken})
	logoutRec := httptest.NewRecorder()
	h.Logout(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", logoutRec.Code)
	}

	if code := doRefresh(h, refreshToken).Code; code != http.StatusUnauthorized {
		t.Fatalf("the refresh token survived logout (status %d); "+
			"anyone holding that cookie value can mint a new access token for another year", code)
	}
}
