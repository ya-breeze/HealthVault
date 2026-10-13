package server_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func TestWidgetDiagnosticFlowRoundTripReplayAndIsolation(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	handler := server.DiagnosticsHandler(st)
	flow, work := uuid.NewString(), uuid.NewString()
	age, changed := int64(60000), false
	stages := [][2]string{{"widget_browser", "launched"}, {"widget_browser", "returned"}, {"widget_work", "enqueue_requested"}, {"widget_work", "started"}, {"widget_fetch", "success"}, {"widget_snapshot", "success"}, {"widget_redraw", "started"}, {"widget_redraw", "success"}, {"widget_work", "success"}}
	events := make([]database.ClientDiagnosticEvent, 0, len(stages))
	for _, stage := range stages {
		events = append(events, database.ClientDiagnosticEvent{ID: uuid.NewString(), OccurredAt: time.Now().UTC(), Operation: stage[0], Category: stage[1], RequestID: flow, WorkID: work, Trigger: "food_entry", Provider: "home", SnapshotAgeMillis: &age, SnapshotChanged: &changed, Attempt: 1, AppVersion: "1.0", AndroidAPI: 36})
	}
	body, _ := json.Marshal(map[string]any{"events": events})
	for range 2 {
		response := httptest.NewRecorder()
		handler(response, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(string(body))), user, family))
		var receipt struct {
			AcceptedIDs []string `json:"accepted_ids"`
		}
		if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &receipt) != nil || len(receipt.AcceptedIDs) != len(events) {
			t.Fatalf("receipt: %d %s", response.Code, response.Body.String())
		}
	}
	read := func(account uuid.UUID) []database.ClientDiagnosticEvent {
		response := httptest.NewRecorder()
		handler(response, withClaimsFamily(httptest.NewRequest("GET", "/api/diagnostics/events", nil), account, family))
		var result struct {
			Events []database.ClientDiagnosticEvent `json:"events"`
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal(response.Body.String())
		}
		return result.Events
	}
	saved := read(user)
	if len(saved) != len(events) {
		t.Fatalf("duplicated or missing stages: %d", len(saved))
	}
	for _, event := range saved {
		if event.RequestID != flow || event.WorkID != work || event.SnapshotChanged == nil || *event.SnapshotChanged || event.SnapshotAgeMillis == nil || *event.SnapshotAgeMillis != age {
			t.Fatalf("metadata lost: %+v", event)
		}
	}
	if len(read(uuid.New())) != 0 {
		t.Fatal("another account can read widget flow")
	}
	events[0].SnapshotChanged = new(bool)
	*events[0].SnapshotChanged = true
	body, _ = json.Marshal(map[string]any{"events": events[:1]})
	response := httptest.NewRecorder()
	handler(response, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(string(body))), user, family))
	if response.Code != 409 {
		t.Fatal("changed replay accepted", response.Code)
	}
}

func TestWidgetDiagnosticRejectsUnsafeMetadataAndInvalidStages(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	handler := server.DiagnosticsHandler(st)
	base := database.ClientDiagnosticEvent{ID: uuid.NewString(), OccurredAt: time.Now().UTC(), Operation: "widget_work", Category: "started", RequestID: uuid.NewString(), Attempt: 1, AppVersion: "1.0", AndroidAPI: 36}
	for name, mutate := range map[string]func(*database.ClientDiagnosticEvent){
		"work id":        func(e *database.ClientDiagnosticEvent) { e.WorkID = "private browser URL" },
		"trigger":        func(e *database.ClientDiagnosticEvent) { e.Trigger = "food contents" },
		"provider":       func(e *database.ClientDiagnosticEvent) { e.Provider = "secret" },
		"age":            func(e *database.ClientDiagnosticEvent) { age := int64(-1); e.SnapshotAgeMillis = &age },
		"category":       func(e *database.ClientDiagnosticEvent) { e.Category = "raw exception" },
		"stage mismatch": func(e *database.ClientDiagnosticEvent) { e.Category = "returned" },
		"operation":      func(e *database.ClientDiagnosticEvent) { e.Operation = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			event := base
			mutate(&event)
			body, _ := json.Marshal(map[string]any{"events": []database.ClientDiagnosticEvent{base, event}})
			response := httptest.NewRecorder()
			handler(response, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(string(body))), user, family))
			if response.Code != 400 {
				t.Fatal("invalid metadata accepted", response.Code)
			}
		})
	}
	var count int64
	st.DB().Model(&database.ClientDiagnosticEvent{}).Count(&count)
	if count != 0 {
		t.Fatal("part of rejected batch persisted")
	}
}

func TestWidgetDiagnosticAgeHasSigned64BitBoundary(t *testing.T) {
	st := newFoodTestStorage(t)
	user, family := seedFoodUser(t, st)
	handler := server.DiagnosticsHandler(st)
	age := int64(9223372036854775807)
	event := database.ClientDiagnosticEvent{ID: uuid.NewString(), OccurredAt: time.Now().UTC(), Operation: "widget_snapshot", Category: "success", RequestID: uuid.NewString(), Attempt: 1, AppVersion: "1.0", AndroidAPI: 36, SnapshotAgeMillis: &age}
	body, _ := json.Marshal(map[string]any{"events": []database.ClientDiagnosticEvent{event}})
	for _, item := range []struct {
		body   string
		status int
	}{
		{string(body), 202},
		{strings.Replace(string(body), "9223372036854775807", "9223372036854775808", 1), 400},
	} {
		response := httptest.NewRecorder()
		handler(response, withClaimsFamily(httptest.NewRequest("POST", "/api/diagnostics/events", strings.NewReader(item.body)), user, family))
		if response.Code != item.status {
			t.Fatalf("integer boundary: %d %s", response.Code, response.Body.String())
		}
	}
}
