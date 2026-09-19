package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

func TestDataTypesPresence_ExecutesExactlyOneSQLStatementAfterUserResolution(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	storage := database.NewStorage(db)
	familyID := uuid.New()
	userID := uuid.New()
	if err := db.Create(&kinmodels.Family{ID: familyID, Name: "PresenceFamily"}).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	if err := db.Create(&kinmodels.User{
		ID: userID, Username: "presence-user", PasswordHash: "x", FamilyID: familyID,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	target, err := resolveUser(
		httpReqForPresence(), storage, userID, familyID,
	)
	if err != nil {
		t.Fatalf("resolveUser: %v", err)
	}

	var statements atomic.Int64
	const queryHook = "test:presence-one-statement-query"
	const rowHook = "test:presence-one-statement-row"
	db.Callback().Query().Before("gorm:query").Register(queryHook, func(*gorm.DB) {
		statements.Add(1)
	})
	db.Callback().Row().Before("gorm:row").Register(rowHook, func(*gorm.DB) {
		statements.Add(1)
	})
	t.Cleanup(func() {
		db.Callback().Query().Remove(queryHook) //nolint:errcheck
		db.Callback().Row().Remove(rowHook)     //nolint:errcheck
	})

	presence, err := dataTypesPresence(storage.DB(), target.ID)
	if err != nil {
		t.Fatalf("dataTypesPresence: %v", err)
	}
	if statements.Load() != 1 {
		t.Fatalf("dataTypesPresence executed %d SQL statements, want exactly 1", statements.Load())
	}
	if len(presence) != len(typeRegistry) {
		t.Fatalf("dataTypesPresence returned %d types, want %d", len(presence), len(typeRegistry))
	}
}

func httpReqForPresence() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/data-types/presence", nil)
}
