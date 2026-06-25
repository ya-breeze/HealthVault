package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/hcimport"
	"github.com/ya-breeze/healthvault/pkg/ingest"
)

func importHealthConnectHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Write zip to a temp file so we can pass it to archive/zip by path.
		tmp, err := os.CreateTemp("", "hc-import-*.zip")
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmp.Name())

		if _, err := io.Copy(tmp, file); err != nil {
			tmp.Close()
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		tmp.Close()

		// Extract health_connect_export.db from the zip.
		dbPath, cleanup, err := extractHCDB(tmp.Name())
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid archive: %s", err), http.StatusUnprocessableEntity)
			return
		}
		defer cleanup()

		payload, counts, err := hcimport.Read(dbPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("parse error: %s", err), http.StatusUnprocessableEntity)
			return
		}

		user, err := storage.FindUserByID(claims.UserID)
		if err != nil {
			http.Error(w, "user not found", http.StatusInternalServerError)
			return
		}

		if err := ingest.Process(storage.DB(), user.ID, user.FamilyID, uuid.New(), payload); err != nil {
			http.Error(w, fmt.Sprintf("ingest error: %s", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(counts) //nolint:errcheck
	}
}

// extractHCDB finds health_connect_export.db inside the zip at zipPath, extracts
// it to a temp file, and returns the temp path plus a cleanup function.
func extractHCDB(zipPath string) (string, func(), error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", nil, fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	const target = "health_connect_export.db"
	for _, f := range zr.File {
		if f.Name != target {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", nil, fmt.Errorf("open zip entry: %w", err)
		}
		defer rc.Close()

		tmp, err := os.CreateTemp("", "hc-db-*.db")
		if err != nil {
			return "", nil, fmt.Errorf("create temp: %w", err)
		}
		if _, err := io.Copy(tmp, rc); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return "", nil, fmt.Errorf("extract db: %w", err)
		}
		tmp.Close()

		return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
	}

	return "", nil, fmt.Errorf("%q not found in archive", target)
}
