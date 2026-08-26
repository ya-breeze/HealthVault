package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/ya-breeze/kin-core/auth"
	"github.com/ya-breeze/kin-core/authdb"
	"github.com/ya-breeze/kin-core/cookies"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

const (
	// maxLoginBodyBytes bounds the request body read before decoding, so an
	// oversized payload is rejected before any JSON parsing work.
	maxLoginBodyBytes = 4 * 1024
	// maxLoginUsernameLength bounds the username accepted for login. The
	// login limiter (login_limiter.go) retains a normalized copy of the
	// username for up to 24h and caps the number of tracked entries at
	// loginMapCeiling, but that count cap alone does not bound an entry's
	// size — without this check, an attacker submitting distinct oversized
	// usernames could still inflate the limiter's memory footprint well
	// beyond what the entry-count ceiling assumes.
	maxLoginUsernameLength = 254
)

type authHandlers struct {
	storage   database.Storage
	db        *gorm.DB
	jwtSecret []byte
	cookieCfg cookies.Config
}

// writeTooManyAttempts writes the 429 response shape shared by all three
// login-limiter rejection cases (real lockout, in-flight capacity, ceiling
// fail-closed). No cookies are set.
func writeTooManyAttempts(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"error":               "too_many_attempts",
		"retry_after_seconds": ceilRetryAfter(retryAfter),
	})
}

func (h *authHandlers) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(req.Username) > maxLoginUsernameLength {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	allowed, retryAfter := admitAttempt(req.Username)
	if !allowed {
		writeTooManyAttempts(w, retryAfter)
		return
	}
	if afterAdmitHook != nil {
		afterAdmitHook()
	}

	user, err := h.storage.FindUserByName(req.Username)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// An operational DB error (not "no such user") is not a
			// credential failure: release the reserved in-flight slot
			// without recording a failure, so a transient outage can't
			// contribute toward locking out a legitimate account.
			releaseAttempt(req.Username)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Run the same bcrypt cost as a wrong-password 401 so an
		// unknown-username response is not measurably faster (closes a
		// username-enumeration timing oracle, per design.md).
		auth.VerifyPassword(req.Password, auth.DummyHash)
		recordFailure(req.Username)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		recordFailure(req.Username)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	recordSuccess(req.Username)

	accessToken, err := auth.GenerateAccessToken(user.ID, &user.FamilyID, h.jwtSecret, 15*time.Minute)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rt, err := authdb.CreateRefreshToken(h.db, user.ID, 365*24*time.Hour)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cookies.SetAccessCookie(w, accessToken, 900, h.cookieCfg)
	cookies.SetRefreshCookie(w, rt.Token, 365*24*3600, h.cookieCfg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

func (h *authHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	tokenStr := cookies.GetAccessToken(r)
	if tokenStr != "" {
		// Parse to get expiry for the blacklist entry; ignore errors (best-effort).
		if claims, err := auth.ParseToken(tokenStr, h.jwtSecret); err == nil {
			expiresAt := claims.RegisteredClaims.ExpiresAt.Time
			authdb.BlacklistToken(h.db, tokenStr, expiresAt) //nolint:errcheck
		}
	}
	cookies.ClearAuthCookies(w, h.cookieCfg)
	w.WriteHeader(http.StatusNoContent)
}

func (h *authHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	rtToken := cookies.GetRefreshToken(r)
	if rtToken == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	newRT, err := authdb.RotateRefreshToken(h.db, rtToken, 365*24*time.Hour)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Load user to get FamilyID for the new access token.
	user, err := h.storage.FindUserByID(newRT.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	accessToken, err := auth.GenerateAccessToken(user.ID, &user.FamilyID, h.jwtSecret, 15*time.Minute)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cookies.SetAccessCookie(w, accessToken, 900, h.cookieCfg)
	cookies.SetRefreshCookie(w, newRT.Token, 365*24*3600, h.cookieCfg)
	w.WriteHeader(http.StatusNoContent)
}
