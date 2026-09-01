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

// Logout revokes a refresh token that reaches it — which, in a browser, none
// does.
//
// Read the name literally: this pins the handler's logic, not the product's
// behaviour. kin-core's SetRefreshCookie scopes the cookie to
// Path=/api/auth/refresh, so a browser never sends kin_refresh to
// /api/auth/logout; cookies.GetRefreshToken returns empty there and the
// revocation below never runs in the real flow. Verified against a deployed
// stack: log in, log out, replay the refresh token — still 204, still mints a
// new session.
//
// The test passes anyway because httptest.NewRequest + AddCookie attaches
// whatever cookie a test names, ignoring path scoping. That gap is exactly why
// this comment exists: without it, a green test reads as proof of a revocation
// the product does not perform.
//
// The behaviour is deliberate, not an oversight to fix here. Scoping a
// year-long credential to one endpoint is why it is hard to steal; widening
// the path to make logout work would put it on every request instead. See
// idea-forge#181. This test stays as the guard for the day that changes, and
// for any non-browser caller that does send the cookie.
func TestLogout_RevokesARefreshTokenItActuallyReceives(t *testing.T) {
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
