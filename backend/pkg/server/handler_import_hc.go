package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/hcimport"
	"github.com/ya-breeze/healthvault/pkg/ingest"
	"gorm.io/gorm"
)

func importHealthConnectHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		slog.Info("hc-import: request received", "user_id", claims.UserID)

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			slog.Error("hc-import: parse multipart form", "err", err, "user_id", claims.UserID)
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			slog.Error("hc-import: missing file field", "err", err, "user_id", claims.UserID)
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()
		slog.Info("hc-import: file received", "filename", header.Filename, "size", header.Size, "user_id", claims.UserID)

		// Write zip to a temp file so we can pass it to archive/zip by path.
		tmp, err := os.CreateTemp("", "hc-import-*.zip")
		if err != nil {
			slog.Error("hc-import: create temp zip", "err", err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmp.Name())

		written, err := io.Copy(tmp, file)
		if err != nil {
			tmp.Close()
			slog.Error("hc-import: write temp zip", "err", err, "user_id", claims.UserID)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		tmp.Close()
		slog.Info("hc-import: zip saved to temp", "bytes", written, "path", tmp.Name())

		// Extract health_connect_export.db from the zip.
		t1 := time.Now()
		dbPath, cleanup, err := extractHCDB(tmp.Name())
		if err != nil {
			slog.Error("hc-import: extract db", "err", err, "user_id", claims.UserID)
			http.Error(w, fmt.Sprintf("invalid archive: %s", err), http.StatusUnprocessableEntity)
			return
		}
		defer cleanup()
		slog.Info("hc-import: db extracted", "duration", time.Since(t1))

		// Read all HC tables into a PayloadJSON.
		t2 := time.Now()
		payload, counts, err := hcimport.Read(dbPath)
		if err != nil {
			slog.Error("hc-import: read hc db", "err", err, "user_id", claims.UserID)
			http.Error(w, fmt.Sprintf("parse error: %s", err), http.StatusUnprocessableEntity)
			return
		}
		slog.Info("hc-import: hc db read complete",
			"duration", time.Since(t2),
			"heart_rate", counts.HeartRate,
			"steps", counts.Steps,
			"sleep", counts.Sleep,
			"exercise", counts.Exercise,
			"distance", counts.Distance,
			"total_calories", counts.TotalCalories,
			"oxygen_saturation", counts.OxygenSaturation,
			"speed", counts.Speed,
		)

		user, err := storage.FindUserByID(claims.UserID)
		if err != nil {
			slog.Error("hc-import: find user", "err", err, "user_id", claims.UserID)
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		// Ingest into HealthVault DB.
		t3 := time.Now()
		slog.Info("hc-import: ingest starting", "user", user.Username)
		payloadID := uuid.New()
		err = storage.DB().Transaction(func(tx *gorm.DB) error {
			return ingest.Process(tx, user.ID, user.FamilyID, payloadID, payload)
		})
		if err != nil {
			slog.Error("hc-import: ingest failed", "err", err, "user", user.Username, "duration", time.Since(t3))
			http.Error(w, fmt.Sprintf("ingest error: %s", err), http.StatusInternalServerError)
			return
		}
		slog.Info("hc-import: ingest complete", "duration", time.Since(t3), "user", user.Username)

		slog.Info("hc-import: done", "total_duration", time.Since(start), "user", user.Username)

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
