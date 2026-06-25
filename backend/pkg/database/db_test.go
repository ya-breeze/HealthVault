package database_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestOpen(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAllTablesCreated(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	tmpDir := t.TempDir()
	db, err := database.Open(logger, tmpDir+"/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	tables := []string{
		"families", "users", "blacklisted_tokens", "refresh_tokens",
		"webhook_payloads",
		"steps", "heart_rates", "heart_rate_variabilities", "sleeps", "sleep_stages",
		"blood_pressures", "distances", "active_calories", "total_calories", "weights",
		"heights", "blood_glucoses", "oxygen_saturations", "body_temperatures",
		"skin_temperatures", "respiratory_rates", "resting_heart_rates", "exercises",
		"hydrations", "nutritions", "basal_metabolic_rates", "body_fats", "lean_body_masses",
		"vo2_maxes", "bone_masses",
	}
	for _, tbl := range tables {
		if !db.Migrator().HasTable(tbl) {
			t.Errorf("missing table: %s", tbl)
		}
	}
}
