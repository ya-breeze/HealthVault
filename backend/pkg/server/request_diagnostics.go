package server

import (
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type diagnosticResponse struct {
	http.ResponseWriter
	code int
}

func (w *diagnosticResponse) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *diagnosticResponse) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (w *diagnosticResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Correlate API requests without logging literal URLs, queries, headers or response bodies.
func RequestDiagnostics(logger *slog.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
			if err != nil || id == uuid.Nil {
				id = uuid.New()
			}
			w.Header().Set("X-Request-ID", id.String())
			start := time.Now()
			out := &diagnosticResponse{ResponseWriter: w}
			next.ServeHTTP(out, r)
			route := "unmatched"
			if current := mux.CurrentRoute(r); current != nil {
				if tmpl, err := current.GetPathTemplate(); err == nil {
					route = tmpl
				}
			}
			code := out.code
			if code == 0 {
				code = 200
			}
			logger.Info("api request", "request_id", id.String(), "method", r.Method, "route", route, "status", code, "duration_ms", time.Since(start).Milliseconds())
		})
	}
}
