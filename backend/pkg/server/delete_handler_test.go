package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/kin-core/auth"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"gorm.io/gorm"
)

// mockStorage is a minimal Storage that only implements DeleteRecord.
type mockStorage struct {
	deleteFunc func(tableName string, id uuid.UUID, userID uuid.UUID) error
}

func (m *mockStorage) DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error {
	return m.deleteFunc(tableName, id, userID)
}
func (m *mockStorage) FindUserByName(_ string) (*kinmodels.User, error)              { return nil, nil }
func (m *mockStorage) FindUserByID(_ uuid.UUID) (*kinmodels.User, error)             { return nil, nil }
func (m *mockStorage) FindUsersByFamilyID(_ uuid.UUID) ([]kinmodels.User, error)     { return nil, nil }
func (m *mockStorage) AllUsers() ([]kinmodels.User, error)                           { return nil, nil }
func (m *mockStorage) SaveWebhookPayload(_ *database.WebhookPayload) error           { return nil }
func (m *mockStorage) QueryRecords(_ string, _ string, _ uuid.UUID, _ database.TimeRange) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockStorage) SummarySteps(_ uuid.UUID, _ database.TimeRange) (int, error)            { return 0, nil }
func (m *mockStorage) SummaryAvgHeartRate(_ uuid.UUID, _ database.TimeRange) (float64, error) { return 0, nil }
func (m *mockStorage) SummarySleepSeconds(_ uuid.UUID, _ database.TimeRange) (int, error)     { return 0, nil }
func (m *mockStorage) DB() *gorm.DB                                                           { return nil }

// withClaims injects claims into the request context (bypassing JWT middleware for tests).
func withClaims(r *http.Request, userID uuid.UUID) *http.Request {
	claims := &auth.Claims{UserID: userID}
	return r.WithContext(context.WithValue(r.Context(), server.ClaimsContextKey, claims))
}

func newDeleteRequest(typeName, id string) *http.Request {
	r := httptest.NewRequest(http.MethodDelete, "/api/data/"+typeName+"/"+id, nil)
	r = mux.SetURLVars(r, map[string]string{"type": typeName, "id": id})
	return r
}

func TestDeleteRecordHandler_Success(t *testing.T) {
	userID := uuid.New()
	recID := uuid.New()

	st := &mockStorage{
		deleteFunc: func(tableName string, id uuid.UUID, uid uuid.UUID) error {
			if tableName != "weights" || id != recID || uid != userID {
				t.Errorf("unexpected DeleteRecord args: table=%s id=%s user=%s", tableName, id, uid)
			}
			return nil
		},
	}

	h := server.DeleteRecordHandler(st)
	w := httptest.NewRecorder()
	r := withClaims(newDeleteRequest("weight", recID.String()), userID)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteRecordHandler_UnknownType(t *testing.T) {
	st := &mockStorage{deleteFunc: func(_ string, _ uuid.UUID, _ uuid.UUID) error { return nil }}
	h := server.DeleteRecordHandler(st)
	w := httptest.NewRecorder()
	r := withClaims(newDeleteRequest("unknown_type", uuid.New().String()), uuid.New())
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteRecordHandler_NotFound(t *testing.T) {
	st := &mockStorage{
		deleteFunc: func(_ string, _ uuid.UUID, _ uuid.UUID) error { return database.ErrNotFound },
	}
	h := server.DeleteRecordHandler(st)
	w := httptest.NewRecorder()
	r := withClaims(newDeleteRequest("weight", uuid.New().String()), uuid.New())
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteRecordHandler_Unauthenticated(t *testing.T) {
	st := &mockStorage{deleteFunc: func(_ string, _ uuid.UUID, _ uuid.UUID) error { return nil }}
	h := server.DeleteRecordHandler(st)
	w := httptest.NewRecorder()
	// No claims injected — simulates missing/invalid JWT.
	r := newDeleteRequest("weight", uuid.New().String())
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDeleteRecordHandler_InvalidUUID(t *testing.T) {
	st := &mockStorage{deleteFunc: func(_ string, _ uuid.UUID, _ uuid.UUID) error { return nil }}
	h := server.DeleteRecordHandler(st)
	w := httptest.NewRecorder()
	r := withClaims(newDeleteRequest("weight", "not-a-uuid"), uuid.New())
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// Ensure mockStorage satisfies the interface at compile time.
var _ database.Storage = (*mockStorage)(nil)
