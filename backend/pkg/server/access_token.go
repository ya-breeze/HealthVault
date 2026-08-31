package server

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ya-breeze/kin-core/auth"
)

// generateAccessToken mints a signed access token that is unique to this call,
// which auth.GenerateAccessToken is not.
//
// The library signs claims of {UserID, FamilyID, IssuedAt, ExpiresAt} with no
// jti and timestamps at one-second resolution. HS256 is deterministic, so two
// tokens minted for one user inside the same second are byte-identical — and
// RequireAuth's blacklist (middleware.go) revokes by the token *string*. Logout
// therefore blacklists a string that the next login reproduces exactly, and
// Refresh reproduces it again, so the new session 401s on every request and
// cannot refresh its way out. It recovers only when the wall clock ticks into
// the next second. That is a real user-facing hole for anyone who logs out and
// straight back in, and it is what made mobile-nav's submit-bar test fail
// whenever it followed the logout test into the same second.
//
// A random jti makes every issued token distinct, so a blacklist entry revokes
// exactly the one session it was created for. Nothing else changes: jti lives
// in jwt.RegisteredClaims, which auth.Claims already embeds and ParseToken
// already parses, so validation is untouched.
//
// The flaw is really kin-core's, and every project on that library shares it.
// Minting here keeps the fix in the repository that found it rather than
// pinning this one to an unreleased library version.
func generateAccessToken(userID uuid.UUID, familyID *uuid.UUID, secret []byte, duration time.Duration) (string, error) {
	now := time.Now()
	claims := auth.Claims{
		UserID:   userID,
		FamilyID: familyID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}
