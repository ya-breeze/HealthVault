package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticIngestionIsolationValidationAndReceipts(t *testing.T) {
	st := newFoodTestStorage(t)
	u, f := seedFoodUser(t, st)
	other := uuid.New()
	h := server.DiagnosticsHandler(st)
	e := database.ClientDiagnosticEvent{ID: uuid.NewString(), OccurredAt: time.Now().UTC(), Operation: "summary", Category: "timeout", RequestID: uuid.NewString(), DurationMillis: 100, Attempt: 1, AppVersion: "1.0", AndroidAPI: 36}
	batch := func(events ...database.ClientDiagnosticEvent) string {
		b, _ := json.Marshal(map[string]any{"events": events})
		return string(b)
	}
	post := func(user uuid.UUID, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h(w, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(body)), user, f))
		return w
	}
	for i := 0; i < 2; i++ {
		w := post(u, batch(e))
		if w.Code != 202 {
			t.Fatalf("ingest/retry: %d %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), e.ID) {
			t.Fatal("missing receipt")
		}
	}
	var count int64
	st.DB().Model(&database.ClientDiagnosticEvent{}).Count(&count)
	if count != 1 {
		t.Fatal("retry duplicated event", count)
	}
	changed := e
	changed.Category = "network"
	if w := post(u, batch(changed)); w.Code != 409 {
		t.Fatal("conflict accepted", w.Code)
	}
	if w := post(other, batch(e)); w.Code != 202 {
		t.Fatal("other account UUID collision", w.Code)
	}
	good := e
	good.ID = uuid.NewString()
	bad := e
	bad.ID = uuid.NewString()
	bad.Category = "raw-secret"
	if w := post(u, batch(good, bad)); w.Code != 400 {
		t.Fatal(w.Code)
	}
	st.DB().Model(&database.ClientDiagnosticEvent{}).Where("id = ?", good.ID).Count(&count)
	if count != 0 {
		t.Fatal("partial invalid batch stored")
	}
	for _, body := range []string{`{"events":[]} {}`, `{"events":[],"password":"secret"}`, strings.Replace(batch(e), `"operation":"summary"`, `"operation":"summary","message":"secret"`, 1)} {
		if w := post(u, body); w.Code != 400 {
			t.Fatal("unsafe body accepted", w.Code)
		}
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(batch(e))))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h(w, withClaimsFamily(httptest.NewRequest("GET", "/api/diagnostics/events?user=other", nil), u, f))
	var out struct {
		Events []database.ClientDiagnosticEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Events) != 1 {
		t.Fatalf("self-only read: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), u.String()) {
		t.Fatal("user id exposed")
	}
}

func TestDiagnosticRetentionDoesNotCapYearHistory(t *testing.T) {
	st := newFoodTestStorage(t)
	u, f := seedFoodUser(t, st)
	rows := make([]database.ClientDiagnosticEvent, 1002)
	for i := range rows {
		rows[i] = database.ClientDiagnosticEvent{UserID: u, ID: uuid.NewString(), ReceivedAt: time.Now().UTC().Add(-time.Hour)}
	}
	rows[0].ReceivedAt = time.Now().UTC().Add(-364 * 24 * time.Hour)
	if err := st.DB().CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"events":[{"id":"%s","occurred_at":"%s","operation":"summary","category":"success","request_id":"%s","http_code":200,"duration_millis":10,"attempt":1,"app_version":"1.0","android_api":36}]}`, uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano), uuid.NewString())
	w := httptest.NewRecorder()
	server.DiagnosticsHandler(st)(w, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(body)), u, f))
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var count int64
	st.DB().Model(&database.ClientDiagnosticEvent{}).Where("user_id = ?", u).Count(&count)
	if count != 1003 {
		t.Fatal("year history truncated", count)
	}
	w = httptest.NewRecorder()
	server.DiagnosticsHandler(st)(w, withClaimsFamily(httptest.NewRequest("GET", "/api/diagnostics/events", nil), u, f))
	var out struct {
		Events []database.ClientDiagnosticEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 || len(out.Events) != 100 {
		t.Fatalf("bounded read: status=%d events=%d error=%v", w.Code, len(out.Events), err)
	}
}

func TestDiagnosticAgeRetentionAndClockSkew(t *testing.T) {
	st := newFoodTestStorage(t)
	u, f := seedFoodUser(t, st)
	old := database.ClientDiagnosticEvent{UserID: u, ID: uuid.NewString(), ReceivedAt: time.Now().UTC().Add(-366 * 24 * time.Hour)}
	if err := st.DB().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	retained := database.ClientDiagnosticEvent{UserID: u, ID: uuid.NewString(), ReceivedAt: time.Now().UTC().Add(-364 * 24 * time.Hour)}
	if err := st.DB().Create(&retained).Error; err != nil {
		t.Fatal(err)
	}
	h := server.DiagnosticsHandler(st)
	w := httptest.NewRecorder()
	h(w, withClaimsFamily(httptest.NewRequest("GET", "/api/diagnostics/events", nil), u, f))
	if w.Code != 200 || strings.Contains(w.Body.String(), old.ID) || !strings.Contains(w.Body.String(), retained.ID) {
		t.Fatal("expired read", w.Body.String())
	}
	body := fmt.Sprintf(`{"events":[{"id":"%s","occurred_at":"%s","operation":"summary","category":"success","request_id":"%s","attempt":1,"app_version":"1.0","android_api":36}]}`, uuid.NewString(), time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339Nano), uuid.NewString())
	w = httptest.NewRecorder()
	h(w, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(body)), u, f))
	if w.Code != 202 {
		t.Fatal("clock skew blocks batch", w.Body.String())
	}
	var count int64
	st.DB().Model(&database.ClientDiagnosticEvent{}).Where("user_id = ? AND id = ?", u, old.ID).Count(&count)
	if count != 0 {
		t.Fatal("expired row retained")
	}
	st.DB().Model(&database.ClientDiagnosticEvent{}).Where("user_id = ? AND id = ?", u, retained.ID).Count(&count)
	if count != 1 {
		t.Fatal("year-old history pruned")
	}
}

func TestRequestCorrelationDoesNotLogSecrets(t *testing.T) {
	var logs bytes.Buffer
	r := mux.NewRouter()
	r.Use(server.RequestDiagnostics(slog.New(slog.NewJSONHandler(&logs, nil))))
	r.HandleFunc("/api/example/{name}", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "secret-response", 401) })
	req := httptest.NewRequest("GET", "/api/example/secret-user?token=secret-query", nil)
	id := uuid.NewString()
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Cookie", "secret-cookie")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-ID") != id || !strings.Contains(logs.String(), id) {
		t.Fatal("missing correlation")
	}
	for _, secret := range []string{"secret-user", "secret-query", "secret-cookie", "secret-response"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret in request log", secret)
		}
	}
}

func TestFiberTargetDefaultsOverrideResetAndSettingsPreservation(t *testing.T) {
	st := newFoodTestStorage(t)
	u, f := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(u, f, `{"timezone":"Europe/Prague","display_language":"ru"}`); err != nil {
		t.Fatal(err)
	}
	read := func() *int {
		w := httptest.NewRecorder()
		server.SummaryTodayHandler(st)(w, withClaimsFamily(httptest.NewRequest("GET", "/api/summary/today", nil), u, f))
		var out struct {
			Target struct {
				Grams *int `json:"dietary_fiber_grams"`
			} `json:"target"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Target.Grams
	}
	if n := read(); n == nil || *n != 25 {
		t.Fatal("default without macro profile", n)
	}
	set := func(body string, code int) {
		w := httptest.NewRecorder()
		server.FiberTargetHandler(st)(w, withClaimsFamily(httptest.NewRequest("PUT", "/api/users/me/fiber-target", strings.NewReader(body)), u, f))
		if w.Code != code {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	set(`{"grams":35}`, 200)
	if n := read(); n == nil || *n != 35 {
		t.Fatal("override", n)
	}
	settings, _ := st.GetUserSettings(u)
	if !strings.Contains(settings, "Europe/Prague") || !strings.Contains(settings, "ru") {
		t.Fatal("other settings lost")
	}
	set(`{"grams":0}`, 400)
	set(`{"grams":201}`, 400)
	set(`{"grams":25.5}`, 400)
	set(`{"grams":null}`, 200)
	if n := read(); n == nil || *n != 25 {
		t.Fatal("reset", n)
	}
	birth := time.Now().AddDate(-12, 0, 0).Format("2006-01-02")
	_ = st.UpsertUserSettings(u, f, fmt.Sprintf(`{"birthdate":"%s"}`, birth))
	if read() != nil {
		t.Fatal("adult target assigned to child")
	}
	set(`{"grams":15}`, 200)
	if n := read(); n == nil || *n != 15 {
		t.Fatal("manual child target", n)
	}
}
