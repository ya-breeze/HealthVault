package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/mcpserver"
	"github.com/ya-breeze/healthvault/pkg/usda"
	"github.com/ya-breeze/healthvault/pkg/vision"
	"github.com/ya-breeze/kin-core/cookies"
)

// requireBearerToken wraps h so that every request must carry
// "Authorization: Bearer <token>". If token is empty the handler responds 503
// (misconfigured) so the endpoint is never accidentally open.
func requireBearerToken(token string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.Error(w, "MCP endpoint not configured", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// Run starts the HealthVault HTTP server and blocks until ctx is cancelled or
// the listener returns a fatal error.
func Run(ctx context.Context, logger *slog.Logger, cfg *config.Config, storage database.Storage) error {
	jwtSecret := []byte(cfg.JWTSecret)
	cookieCfg := cookies.Config{Secure: cfg.CookieSecure}

	ah := &authHandlers{
		storage:   storage,
		db:        storage.DB(),
		jwtSecret: jwtSecret,
		cookieCfg: cookieCfg,
	}

	// USDA index is optional at startup: no import has necessarily run yet,
	// and the food search endpoint degrades to "unavailable" rather than
	// failing the whole server.
	usdaIndex, err := usda.Open(cfg.USDADBPath)
	if err != nil && !errors.Is(err, usda.ErrNoDatabase) {
		return fmt.Errorf("open usda index: %w", err)
	}
	defer usdaIndex.Close() //nolint:errcheck

	// The real OpenAI-backed vision.Client (task 4.1) is not wired up yet —
	// deferred until HCW_OPENAI_API_KEY is available to build and test
	// against. Every photo upload fails with vision.ErrNotConfigured until
	// then; manual entry, custom foods, and search need no vision access.
	fh := NewFoodHandlers(storage, usdaIndex, cfg.UploadsDir).
		WithVision(vision.Unconfigured{}, cfg.MaxUploadBytes, cfg.VisionTimeout)

	r := mux.NewRouter()

	// Webhook (unauthenticated) — implemented in Task 5
	r.HandleFunc("/webhook/{username}", webhookHandler(storage)).Methods("POST")

	// Auth
	r.HandleFunc("/api/auth/login", ah.Login).Methods("POST")
	r.HandleFunc("/api/auth/logout", ah.Logout).Methods("POST")
	r.HandleFunc("/api/auth/refresh", ah.Refresh).Methods("POST")

	// Protected API — data routes implemented in Task 6
	api := r.PathPrefix("/api").Subrouter()
	api.Use(RequireAuth(jwtSecret, cookieCfg, storage.DB()))
	api.HandleFunc("/users/me", meHandler(storage)).Methods("GET")
	// Note: /data/summary must be registered before /data/{type} to avoid
	// gorilla/mux routing "summary" as the {type} variable.
	api.HandleFunc("/data/summary", summaryHandler(storage)).Methods("GET")
	api.HandleFunc("/data/{type}", dataHandler(storage)).Methods("GET")
	api.HandleFunc("/data/{type}/{id}", DeleteRecordHandler(storage, fh.photos)).Methods("DELETE")
	api.HandleFunc("/import/health-connect", importHealthConnectHandler(storage)).Methods("POST")
	api.HandleFunc("/import/libra", importLibraHandler(storage)).Methods("POST")
	api.HandleFunc("/food/search", fh.Search).Methods("GET")
	api.HandleFunc("/food/custom", fh.CreateCustomFood).Methods("POST")
	api.HandleFunc("/food/custom", fh.ListCustomFoods).Methods("GET")
	api.HandleFunc("/food/custom/{id}", fh.UpdateCustomFood).Methods("PUT")
	api.HandleFunc("/food/custom/{id}", fh.DeleteCustomFood).Methods("DELETE")
	api.HandleFunc("/food/meals", fh.CreateMeal).Methods("POST")
	api.HandleFunc("/food/meals/manual", fh.CreateManualMeal).Methods("POST")
	api.HandleFunc("/food/meals/{id}/photo", fh.MealPhoto).Methods("GET")
	api.HandleFunc("/food/calibration-samples/{id}/photo", fh.CalibrationSamplePhoto).Methods("GET")

	// MCP — protected by a static bearer token (HCW_MCP_TOKEN).
	// If the token is empty the endpoint responds 503 so it is never accidentally open.
	mcpHandler := mcpserver.Handler(storage)
	r.PathPrefix("/mcp").Handler(requireBearerToken(cfg.MCPToken, mcpHandler))

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	logger.Info("listening", "port", cfg.Port)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return fmt.Errorf("server: %w", err)
	}
}
