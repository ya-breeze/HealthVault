package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ya-breeze/kin-core/authdb"
	"github.com/ya-breeze/kin-core/cookies"
	"gorm.io/gorm"
)

// cfAuthorizationCookie is the cookie Cloudflare Access itself sets in the
// browser; CFAccess falls back to it only when the Cf-Access-Jwt-Assertion
// header is absent (nginx forwards that header unmodified, since it has no
// underscore for nginx to drop).
const cfAuthorizationCookie = "CF_Authorization"

// parseCFAccessEmailMap parses "email:username,..." — the same shape
// database.SeedUsers's spec uses — into a lookup map, lower-casing and
// trimming each email so a later lookup against the assertion's own email
// claim (also lower-cased) is a plain map read.
func parseCFAccessEmailMap(spec string) (map[string]string, error) {
	m := map[string]string{}
	if spec == "" {
		return m, nil
	}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid cf-access email map entry %q (want email:username)", entry)
		}
		email := strings.ToLower(strings.TrimSpace(parts[0]))
		username := strings.TrimSpace(parts[1])
		if email == "" || username == "" {
			return nil, fmt.Errorf("invalid cf-access email map entry %q (want email:username)", entry)
		}
		m[email] = username
	}
	return m, nil
}

func writeUnknownIdentity(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{"error": "unknown_identity"}) //nolint:errcheck
}

// CFAccess exchanges a verified Cloudflare Access Assertion for the same
// kind of session Login mints, so every downstream handler, RequireAuth, the
// blacklist and the refresh rotation keep working untouched.
//
// It 404s while the feature is unconfigured (team domain, AUD or the email
// map unset) so a deployment with no Cloudflare in front — the WIP stack,
// the e2e target, `make run-backend` — reports the feature as absent rather
// than as a failed sign-in.
func (h *authHandlers) CFAccess(w http.ResponseWriter, r *http.Request) {
	if h.cfVerifier == nil || len(h.cfEmailMap) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	token := r.Header.Get("Cf-Access-Jwt-Assertion")
	if token == "" {
		if c, err := r.Cookie(cfAuthorizationCookie); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	identity, err := h.cfVerifier.Verify(r.Context(), token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	username, ok := h.cfEmailMap[strings.ToLower(identity.Email)]
	if !ok {
		writeUnknownIdentity(w)
		return
	}

	user, err := h.storage.FindUserByName(username)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeUnknownIdentity(w)
		return
	}

	// No login limiter here, unlike Login: there is no guessable secret for
	// an attacker to spread attempts over, since the whole gate is an RSA
	// signature check against Cloudflare's published keys rather than a
	// bcrypt compare against a stored secret — the thing the limiter exists
	// to protect.
	accessToken, err := generateAccessToken(user.ID, &user.FamilyID, h.jwtSecret, 15*time.Minute)
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
