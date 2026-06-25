package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ya-breeze/kin-core/auth"
	"github.com/ya-breeze/kin-core/authdb"
	"github.com/ya-breeze/kin-core/cookies"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

type authHandlers struct {
	storage   database.Storage
	db        *gorm.DB
	jwtSecret []byte
	cookieCfg cookies.Config
}

func (h *authHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, err := h.storage.FindUserByName(req.Username)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
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
