package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/ingest"
	"gorm.io/gorm"
)

// parseAndIngest handles the common import scaffold: auth → multipart → parse → user lookup → ingest → JSON response.
//
// parseFn receives the uploaded file and returns the payload for ingest, counts to JSON-encode on
// success, an HTTP status code to use on error (ignored when err == nil), and the error itself.
// For 5xx status codes the actual error is logged but only "server error" is sent to the client.
func parseAndIngest(
	w http.ResponseWriter,
	r *http.Request,
	storage database.Storage,
	prefix string,
	parseFn func(multipart.File, *multipart.FileHeader) (*ingest.PayloadJSON, any, int, error),
) {
	start := time.Now()

	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	slog.Info(prefix+": request received", "user_id", claims.UserID)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		slog.Error(prefix+": parse multipart form", "err", err, "user_id", claims.UserID)
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Error(prefix+": missing file field", "err", err, "user_id", claims.UserID)
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()
	slog.Info(prefix+": file received", "filename", header.Filename, "size", header.Size)

	t1 := time.Now()
	payload, counts, errStatus, parseErr := parseFn(file, header)
	if parseErr != nil {
		slog.Error(prefix+": parse failed", "err", parseErr, "status", errStatus, "user_id", claims.UserID)
		if errStatus >= http.StatusInternalServerError {
			http.Error(w, "server error", errStatus)
		} else {
			http.Error(w, parseErr.Error(), errStatus)
		}
		return
	}
	slog.Info(prefix+": parse complete", "duration", time.Since(t1))

	user, dbErr := storage.FindUserByID(claims.UserID)
	if dbErr != nil {
		slog.Error(prefix+": find user", "err", dbErr, "user_id", claims.UserID)
		if errors.Is(dbErr, gorm.ErrRecordNotFound) {
			http.Error(w, "user not found", http.StatusNotFound)
		} else {
			http.Error(w, "server error", http.StatusInternalServerError)
		}
		return
	}

	t2 := time.Now()
	payloadID := uuid.New()
	if err = storage.DB().Transaction(func(tx *gorm.DB) error {
		return ingest.Process(tx, user.ID, user.FamilyID, payloadID, payload)
	}); err != nil {
		slog.Error(prefix+": ingest failed", "err", err, "user", user.Username, "duration", time.Since(t2))
		http.Error(w, fmt.Sprintf("ingest error: %s", err), http.StatusInternalServerError)
		return
	}
	slog.Info(prefix+": done", "total_duration", time.Since(start), "user", user.Username)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(counts) //nolint:errcheck
}
