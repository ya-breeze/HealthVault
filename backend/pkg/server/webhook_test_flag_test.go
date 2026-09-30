package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestWebhook_TestPayloadIsAuditedButNotIngested(t *testing.T) {
	storage, userID, _ := newDashboardInternalStorage(t)
	webhook := requireWebhookToken("test-token", webhookHandler(storage))
	post := func(body string, token bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/webhook/dashboard-user", strings.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"username": "dashboard-user"})
		if token {
			req.Header.Set(webhookTokenHeader, "test-token")
		}
		recorder := httptest.NewRecorder()
		webhook.ServeHTTP(recorder, req)
		return recorder
	}
	count := func(table string) int64 {
		t.Helper()
		var got int64
		if err := storage.DB().Table(table).Where("user_id = ?", userID).Count(&got).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return got
	}

	testPayload := `{"test":true,"app_version":"test","steps":[{"count":8432,"start_time":"2026-06-24T00:00:00Z","end_time":"2026-06-24T01:00:00Z"}],"weight":[{"kilograms":75.5,"time":"2026-06-24T07:30:00Z"}]}`
	if got := post(testPayload, true).Code; got != http.StatusNoContent {
		t.Fatalf("test payload status = %d, want 204", got)
	}
	if got := count("webhook_payloads"); got != 1 {
		t.Fatalf("audit payload count = %d, want 1", got)
	}
	var audit database.WebhookPayload
	if err := storage.DB().Where("user_id = ?", userID).First(&audit).Error; err != nil {
		t.Fatalf("read raw audit payload: %v", err)
	}
	if audit.Raw != testPayload {
		t.Fatalf("raw audit payload = %q, want exact request body", audit.Raw)
	}
	if got := count("steps"); got != 0 {
		t.Errorf("test payload created %d step rows, want none", got)
	}
	if got := count("weights"); got != 0 {
		t.Errorf("test payload created %d weight rows, want none", got)
	}

	ordinaryPayload := `{"test":false,"steps":[{"count":1200,"start_time":"2026-06-25T00:00:00Z","end_time":"2026-06-25T01:00:00Z"}],"weight":[{"kilograms":74.8,"time":"2026-06-25T07:30:00Z"}]}`
	if got := post(ordinaryPayload, true).Code; got != http.StatusNoContent {
		t.Fatalf("test:false payload status = %d, want 204", got)
	}
	if got := post(`{"steps":[{"count":500,"start_time":"2026-06-26T00:00:00Z","end_time":"2026-06-26T01:00:00Z"}]}`, true).Code; got != http.StatusNoContent {
		t.Fatalf("unmarked payload status = %d, want 204", got)
	}
	if got := count("steps"); got != 2 {
		t.Errorf("ordinary step rows = %d, want 2", got)
	}
	if got := count("weights"); got != 1 {
		t.Errorf("ordinary weight rows = %d, want 1", got)
	}
	if got := count("webhook_payloads"); got != 3 {
		t.Errorf("raw audit rows = %d, want all 3 accepted payloads", got)
	}

	for _, malformed := range []string{`{"test":null}`, `{"test":1}`, `{"test":"true"}`} {
		if got := post(malformed, true).Code; got != http.StatusBadRequest {
			t.Errorf("malformed test flag %s status = %d, want 400", malformed, got)
		}
	}
	if got := post(testPayload, false).Code; got != http.StatusUnauthorized {
		t.Errorf("unauthenticated test payload status = %d, want 401", got)
	}
	if got := count("webhook_payloads"); got != 3 {
		t.Errorf("rejected requests created %d audit rows, want none", got-3)
	}
}
