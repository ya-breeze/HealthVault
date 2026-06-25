package server

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/hcimport"
	"github.com/ya-breeze/healthvault/pkg/ingest"
)

func importHealthConnectHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parseAndIngest(w, r, storage, "hc-import", hcParseFn)
	}
}

func hcParseFn(file multipart.File, header *multipart.FileHeader) (*ingest.PayloadJSON, any, int, error) {
	// zip.OpenReader requires a seekable file, so buffer the upload to disk first.
	tmp, err := os.CreateTemp("", "hc-import-*.zip")
	if err != nil {
		return nil, nil, http.StatusInternalServerError, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	written, err := io.Copy(tmp, file)
	if err != nil {
		tmp.Close()
		return nil, nil, http.StatusInternalServerError, fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()
	slog.Info("hc-import: zip saved to temp", "bytes", written, "filename", header.Filename)

	t1 := time.Now()
	dbPath, cleanup, err := extractHCDB(tmp.Name())
	if err != nil {
		return nil, nil, http.StatusUnprocessableEntity, fmt.Errorf("invalid archive: %s", err)
	}
	defer cleanup()

	payload, counts, err := hcimport.Read(dbPath)
	if err != nil {
		return nil, nil, http.StatusUnprocessableEntity, err
	}
	slog.Info("hc-import: db read complete",
		"duration", time.Since(t1),
		"heart_rate", counts.HeartRate,
		"steps", counts.Steps,
		"sleep", counts.Sleep,
		"exercise", counts.Exercise,
		"distance", counts.Distance,
		"total_calories", counts.TotalCalories,
		"oxygen_saturation", counts.OxygenSaturation,
		"speed", counts.Speed,
	)
	return payload, counts, 0, nil
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
