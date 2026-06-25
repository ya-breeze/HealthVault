package server

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/auth"
	"github.com/ya-breeze/kin-core/cookies"
	kinmw "github.com/ya-breeze/kin-core/middleware"
	"gorm.io/gorm"
)

type contextKey string

const claimsKey contextKey = "claims"

// ClaimsContextKey is the exported form of claimsKey, used in tests to inject claims directly.
const ClaimsContextKey = claimsKey

// RequireAuth returns a middleware that validates the kin_access JWT cookie.
// It checks the DB blacklist via the provided *gorm.DB.
func RequireAuth(jwtSecret []byte, cookieCfg cookies.Config, db *gorm.DB) func(http.Handler) http.Handler {
	cfg := kinmw.Config{JWTSecret: jwtSecret, CookieCfg: cookieCfg, DB: db}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := kinmw.ValidateRequest(r, cfg)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromCtx extracts the *auth.Claims set by RequireAuth from the request context.
func ClaimsFromCtx(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

// FamilyIDFromCtx returns the FamilyID from the JWT claims, or uuid.Nil if absent.
func FamilyIDFromCtx(r *http.Request) uuid.UUID {
	c := ClaimsFromCtx(r)
	if c == nil {
		return uuid.Nil
	}
	if c.FamilyID == nil {
		return uuid.Nil
	}
	return *c.FamilyID
}
