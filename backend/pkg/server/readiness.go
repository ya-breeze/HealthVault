package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

const readinessTimeout = 2 * time.Second

type readinessCheck func(context.Context) error

// sqliteReadinessCheck confirms that the application's user table can be read.
// It intentionally returns no row data and never exposes a database error.
func sqliteReadinessCheck(db *gorm.DB) readinessCheck {
	return func(ctx context.Context) error {
		if db == nil {
			return errors.New("database unavailable")
		}

		var count int64
		return db.WithContext(ctx).Raw("SELECT COUNT(*) FROM users").Scan(&count).Error
	}
}

func readinessHandler(check readinessCheck, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check == nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := check(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// registerReadinessRoute adds an internal-only readiness route outside the
// authenticated /api subrouter. Nginx restricts external access to this path.
func registerReadinessRoute(router *mux.Router, db *gorm.DB) {
	router.Handle("/internal/ready", readinessHandler(sqliteReadinessCheck(db), readinessTimeout)).Methods(http.MethodGet)
}
