package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
)

// Prevent a new web-visible metric from silently disappearing from agent reads.
func TestPhoneHealthCatalogCoversWebMetricTypes(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	catalog, err := database.HealthDataTypes(context.Background(), db, uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(catalog))
	for _, dataset := range catalog {
		names[dataset.Name] = true
	}
	for name := range typeRegistry {
		if !names[name] {
			t.Errorf("web metric %q has no discoverable user MCP dataset", name)
		}
	}
}
