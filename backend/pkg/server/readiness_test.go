package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReadinessHandlerSQLiteReady(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}

	router := mux.NewRouter()
	registerReadinessRoute(router, db)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/internal/ready", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q; want 200", response.Code, response.Body.String())
	}
	if response.Body.Len() != 0 {
		t.Fatalf("readiness response exposed a body: %q", response.Body.String())
	}
}

func TestReadinessHandlerDatabaseFailureIsGenericUnavailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	for name, check := range map[string]readinessCheck{
		"query error":      sqliteReadinessCheck(db),
		"missing database": sqliteReadinessCheck(nil),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			readinessHandler(check, time.Second).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/internal/ready", nil))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d; want 503", response.Code)
			}
			if strings.TrimSpace(response.Body.String()) != "not ready" {
				t.Fatalf("body = %q; want generic error", response.Body.String())
			}
		})
	}
}

func TestReadinessHandlerIsBoundedByContextDeadline(t *testing.T) {
	const timeout = 20 * time.Millisecond
	check := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	response := httptest.NewRecorder()
	started := time.Now()
	readinessHandler(check, timeout).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/internal/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", response.Code)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("readiness check exceeded bounded timeout: %s", elapsed)
	}
}

func TestReadinessRouteDoesNotRequireAPIAuthentication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}

	router := mux.NewRouter()
	registerReadinessRoute(router, db)
	api := router.PathPrefix("/api").Subrouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	})
	api.HandleFunc("/private", func(http.ResponseWriter, *http.Request) {}).Methods(http.MethodGet)

	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/internal/ready", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("readiness status = %d; want 200 without credentials", ready.Code)
	}

	private := httptest.NewRecorder()
	router.ServeHTTP(private, httptest.NewRequest(http.MethodGet, "/api/private", nil))
	if private.Code != http.StatusUnauthorized {
		t.Fatalf("protected API status = %d; want 401 without credentials", private.Code)
	}
}
