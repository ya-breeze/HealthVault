package server_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

// Keep discovery on the real HTTP surface. This catches catalog/schema
// incompatibilities that direct service tests cannot observe.
func healthRPC(t *testing.T, h http.Handler, method, params, protocol string) map[string]any {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/phone-mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`))
	r.Header.Set("Authorization", "Bearer test-phone-token")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", protocol)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%s HTTP %d: %s", method, w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			body = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	var reply map[string]any
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return reply
}

func healthTextParity(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	out := phoneSuccess(t, result)
	content, ok := result["content"].([]any)
	if !ok {
		t.Fatal("missing textual fallback")
	}
	for _, item := range content {
		part, ok := item.(map[string]any)
		if !ok || part["type"] != "text" {
			continue
		}
		text, _ := part["text"].(string)
		var decoded map[string]any
		if json.Unmarshal([]byte(text), &decoded) == nil && reflect.DeepEqual(out, decoded) {
			return out
		}
	}
	t.Fatal("text fallback does not contain the complete structured result")
	return nil
}

func TestPhoneHealthPortableDiscovery(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	for _, protocol := range []string{"2025-03-26", "2025-11-25"} {
		reply := healthRPC(t, h, "tools/list", `{}`, protocol)
		if reply["error"] != nil {
			t.Fatalf("discovery failed: %+v", reply)
		}
		tools := reply["result"].(map[string]any)["tools"].([]any)
		found := map[string]map[string]any{}
		for _, value := range tools {
			tool := value.(map[string]any)
			found[tool["name"].(string)] = tool
		}
		for _, name := range []string{"list_health_data_types", "query_health_data", "get_health_import", "get_health_record"} {
			tool, ok := found[name]
			if !ok {
				t.Fatalf("%s missing for %s", name, protocol)
			}
			annotations := tool["annotations"].(map[string]any)
			if annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false || annotations["openWorldHint"] != false {
				t.Fatalf("unsafe or absent read annotations: %+v", annotations)
			}
			schema := tool["inputSchema"].(map[string]any)
			props, _ := schema["properties"].(map[string]any)
			for _, selector := range []string{"user_id", "username", "family_id", "table", "sql"} {
				if _, ok := props[selector]; ok {
					t.Fatalf("%s exposes %s selector", name, selector)
				}
			}
			if name == "query_health_data" {
				typeSchema := props["type"].(map[string]any)
				if typeSchema["type"] != "string" || typeSchema["enum"] != nil {
					t.Fatalf("catalog expansion requires client schema changes: %+v", typeSchema)
				}
			}
			if tool["outputSchema"] == nil {
				t.Fatalf("%s missing structured output schema", name)
			}
		}
		for _, previous := range []string{"describe_food_meal", "get_food_meal", "list_food_meals", "get_activity_daily_totals", "list_activity_exercises"} {
			if found[previous] == nil {
				t.Fatalf("existing tool %s disappeared", previous)
			}
		}
	}
	cat := healthTextParity(t, phoneCall(t, h, "list_health_data_types", map[string]any{}))
	seen := map[string]bool{}
	for _, value := range cat["types"].([]any) {
		d := value.(map[string]any)
		name := d["name"].(string)
		seen[name] = true
		if d["record_count"] != float64(0) || d["first_time"] != nil || d["last_time"] != nil {
			t.Fatalf("empty synthetic catalog fabricated coverage: %+v", d)
		}
		if d["time_field"] == "" || len(d["fields"].([]any)) == 0 {
			t.Fatalf("%s lacks read semantics", name)
		}
		out := healthTextParity(t, phoneCall(t, h, "query_health_data", map[string]any{"type": name}))
		if len(out["records"].([]any)) != 0 || out["has_more"] != false || out["next_cursor"] != "" {
			t.Fatalf("empty dataset %s not readable: %+v", name, out)
		}
	}
	for _, required := range []string{"heart_rate", "sleep", "sleep_stage", "weight", "steps", "speed", "exercise", "food_meal", "food_item", "custom_food", "webhook_import"} {
		if !seen[required] {
			t.Errorf("stored health type %s not discoverable", required)
		}
	}
}

func TestPhoneHealthRecordsPaginationAndScope(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	stamp := func(value string) time.Time {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	create := func(user, fam uuid.UUID, value string, bpm int) database.HeartRate {
		r := database.HeartRate{UserID: user, Time: stamp(value), BPM: bpm, SourcePayloadID: uuid.New()}
		r.ID, r.FamilyID = uuid.New(), fam
		if err := st.DB().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := create(owner, family, "2025-10-26T02:30:00+02:00", 61)
	second := create(owner, family, "2025-10-26T02:30:00+01:00", 62)
	tied := create(owner, family, "2025-10-26T01:30:00Z", 63)
	last := create(owner, family, "2025-10-26T22:59:59.999999999Z", 64)
	lower := create(owner, family, "2025-10-25T22:00:00Z", 0)
	create(owner, family, "2025-10-25T21:59:59.999999999Z", 11)
	create(owner, family, "2025-10-26T23:00:00Z", 12)
	create(uuid.New(), family, "2025-10-26T05:00:00Z", 99)
	create(owner, uuid.New(), "2025-10-26T06:00:00Z", 98)
	deleted := create(owner, family, "2025-10-26T07:00:00Z", 97)
	if err := st.DB().Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	args := map[string]any{"type": "heart_rate", "from": "2025-10-25T22:00:00Z", "to": "2025-10-26T23:00:00Z", "limit": 1}
	ties := []string{second.ID.String(), tied.ID.String()}
	sort.Strings(ties)
	expect := []string{lower.ID.String(), first.ID.String(), ties[0], ties[1], last.ID.String()}
	var savedCursor string
	for i, want := range expect {
		result := phoneCall(t, h, "query_health_data", args)
		if result["isError"] == true {
			cursor, _ := args["cursor"].(string)
			decoded, _ := base64.RawURLEncoding.DecodeString(cursor)
			t.Fatalf("page %d failed with cursor %s: %+v", i, decoded, result)
		}
		out := healthTextParity(t, result)
		rows := out["records"].([]any)
		if len(rows) != 1 {
			t.Fatalf("page %d: %+v", i, out)
		}
		r := rows[0].(map[string]any)
		if r["id"] != want {
			t.Fatalf("page %d got %v want %s", i, r["id"], want)
		}
		if i == 0 && r["bpm"] != float64(0) {
			t.Fatal("stored zero was lost")
		}
		if r["source_payload_id"] == nil {
			t.Fatal("source evidence link absent")
		}
		for _, key := range []string{"user_id", "family_id", "deleted_at", "password_hash"} {
			if _, ok := r[key]; ok {
				t.Fatalf("internal field %s exposed", key)
			}
		}
		more := i+1 < len(expect)
		if out["has_more"] != more {
			t.Fatalf("page completion wrong: %+v", out)
		}
		if more {
			cursor := out["next_cursor"].(string)
			if cursor == "" {
				t.Fatal("missing continuation")
			}
			args["cursor"] = cursor
			if savedCursor == "" {
				savedCursor = cursor
			}
		} else if out["next_cursor"] != "" {
			t.Fatal("terminal continuation not empty")
		}
	}
	for _, changed := range []map[string]any{
		{"type": "weight", "from": args["from"], "to": args["to"], "cursor": savedCursor},
		{"type": "heart_rate", "from": "2025-10-25T23:00:00Z", "to": args["to"], "cursor": savedCursor},
		{"type": "heart_rate", "cursor": "malformed"},
		{"type": "heart_rate", "from": "2025-10-26", "to": args["to"]},
		{"type": "heart_rate", "from": args["to"], "to": args["from"]},
		{"type": "heart_rate", "limit": 101},
		{"type": "heart_rate", "limit": -1},
		{"type": "users"}, {"type": "heart_rates; SELECT * FROM users"},
	} {
		if phoneCall(t, h, "query_health_data", changed)["isError"] != true {
			t.Fatalf("invalid query accepted: %+v", changed)
		}
	}
	// A selector outside the schema must either be rejected or leave the
	// configured identity untouched; it must never retrieve the other user.
	reply := healthRPC(t, h, "tools/call", `{"name":"query_health_data","arguments":{"type":"heart_rate","user_id":"`+uuid.NewString()+`"}}`, "2025-11-25")
	if reply["error"] == nil {
		result := reply["result"].(map[string]any)
		if result["isError"] != true {
			out := phoneSuccess(t, result)
			for _, row := range out["records"].([]any) {
				if row.(map[string]any)["bpm"] == float64(99) {
					t.Fatal("caller selector widened scope")
				}
			}
		}
	}
	cat := phoneSuccess(t, phoneCall(t, h, "list_health_data_types", map[string]any{}))
	for _, value := range cat["types"].([]any) {
		d := value.(map[string]any)
		if d["name"] == "heart_rate" && d["record_count"] != float64(7) {
			t.Fatalf("catalog count leaked foreign/deleted records: %+v", d)
		}
	}
	var count int64
	if err := st.DB().Model(&database.HeartRate{}).Unscoped().Count(&count).Error; err != nil || count != 10 {
		t.Fatalf("read changed records: %d %v", count, err)
	}
}

func TestPhoneHealthNestedAndNullableEvidence(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	start := time.Date(2026, 1, 1, 22, 0, 0, 0, time.UTC)
	sequence := 0
	makeSleep := func(user uuid.UUID) database.Sleep {
		sessionStart := start.Add(time.Duration(sequence) * 24 * time.Hour)
		sequence++
		r := database.Sleep{UserID: user, SourcePayloadID: uuid.New(), StartTime: sessionStart, SessionEndTime: sessionStart.Add(8 * time.Hour), DurationSeconds: 28800}
		r.ID, r.FamilyID = uuid.New(), family
		if err := st.DB().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
		stage := database.SleepStage{SleepID: r.ID, StartTime: sessionStart, EndTime: sessionStart.Add(time.Hour), DurationSeconds: 3600, Stage: "deep"}
		if err := st.DB().Create(&stage).Error; err != nil {
			t.Fatal(err)
		}
		return r
	}
	owned := makeSleep(owner)
	makeSleep(uuid.New())
	deleted := makeSleep(uuid.New())
	// The deleted parent has the connected user's identity, while its child
	// has no user_id. A child-table-only filter would expose this row.
	if err := st.DB().Model(&deleted).Update("user_id", owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.DB().Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	weight := database.Weight{UserID: owner, Time: start, Kilograms: 82.5}
	weight.ID, weight.FamilyID = uuid.New(), family
	if err := st.DB().Create(&weight).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	sleeps := healthTextParity(t, phoneCall(t, h, "query_health_data", map[string]any{"type": "sleep"}))["records"].([]any)
	if len(sleeps) != 1 || sleeps[0].(map[string]any)["id"] != owned.ID.String() {
		t.Fatalf("sleep ownership failed: %+v", sleeps)
	}
	stages := phoneSuccess(t, phoneCall(t, h, "query_health_data", map[string]any{"type": "sleep_stage"}))["records"].([]any)
	if len(stages) != 1 || stages[0].(map[string]any)["sleep_id"] != owned.ID.String() {
		t.Fatalf("stage parent isolation failed: %+v", stages)
	}
	weights := phoneSuccess(t, phoneCall(t, h, "query_health_data", map[string]any{"type": "weight"}))["records"].([]any)
	if len(weights) != 1 || weights[0].(map[string]any)["kilograms"] != 82.5 || weights[0].(map[string]any)["source_payload_id"] != nil {
		t.Fatalf("manual weight source fabricated: %+v", weights)
	}
}

func TestPhoneHealthOversizedRecordRemainsReadable(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	name := strings.Repeat("Long saved food name ž😀 ", 3000)
	meal := database.FoodMeal{UserID: owner, LoggedAt: time.Now(), Name: name, Status: database.MealStatusConfirmed, Calories: 200, PhotoPath: "private/photo.jpg", RawResponse: "private-model-output", ClarifyLog: "private-log", RequestFingerprint: "private-fingerprint"}
	meal.ID, meal.FamilyID = uuid.New(), family
	if err := st.DB().Create(&meal).Error; err != nil {
		t.Fatal(err)
	}
	other := meal
	other.ID, other.UserID = uuid.New(), uuid.New()
	if err := st.DB().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	listed := phoneSuccess(t, phoneCall(t, h, "query_health_data", map[string]any{"type": "food_meal"}))["records"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["record_chunk_required"] != true || listed[0].(map[string]any)["id"] != meal.ID.String() {
		t.Fatalf("large record not reachable: %+v", listed)
	}
	var data strings.Builder
	offset := 0
	var checksum string
	for attempts := 0; attempts < 100; attempts++ {
		out := healthTextParity(t, phoneCall(t, h, "get_health_record", map[string]any{"type": "food_meal", "record_id": meal.ID.String(), "offset": offset}))
		piece := out["data"].(string)
		if !utf8.ValidString(piece) || len(piece) > 8192 || out["offset"] != float64(offset) {
			t.Fatal("invalid record chunk")
		}
		if checksum != "" && checksum != out["sha256"] {
			t.Fatal("record changed between chunks")
		}
		checksum = out["sha256"].(string)
		data.WriteString(piece)
		next := int(out["next_offset"].(float64))
		if next != offset+len(piece) {
			t.Fatal("record bytes skipped")
		}
		offset = next
		if out["has_more"] == false {
			if out["total_bytes"] != float64(data.Len()) {
				t.Fatal("incomplete record bytes")
			}
			break
		}
		if piece == "" {
			t.Fatal("record chunk stalled")
		}
	}
	hash := sha256.Sum256([]byte(data.String()))
	if checksum != hex.EncodeToString(hash[:]) {
		t.Fatal("record digest mismatch")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(data.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["name"] != name || decoded["calories"] != float64(200) {
		t.Fatal("record details truncated")
	}
	for _, key := range []string{"photo_path", "raw_response", "clarify_log", "request_fingerprint", "user_id", "family_id"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("unapproved field %s exposed by chunk path", key)
		}
	}
	for _, args := range []map[string]any{{"type": "food_meal", "record_id": other.ID.String()}, {"type": "users", "record_id": owner.String()}, {"type": "food_meal", "record_id": "invalid"}, {"type": "food_meal", "record_id": meal.ID.String(), "offset": -1}} {
		if phoneCall(t, h, "get_health_record", args)["isError"] != true {
			t.Fatalf("invalid/foreign record accepted: %+v", args)
		}
	}
}

func TestPhoneHealthOriginalImportChunkRecovery(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	raw := `{"future_sensor":{"unit":"arbitrary","text":"` + strings.Repeat("ž😀", 4000) + `","value":12},"data_origin":"watch.example"}`
	imported := database.WebhookPayload{UserID: owner, ReceivedAt: time.Now(), PayloadTs: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), AppVersion: "future", Raw: raw}
	imported.ID, imported.FamilyID = uuid.New(), family
	if err := st.DB().Create(&imported).Error; err != nil {
		t.Fatal(err)
	}
	other := imported
	other.ID, other.UserID = uuid.New(), uuid.New()
	if err := st.DB().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	meta := phoneSuccess(t, phoneCall(t, h, "query_health_data", map[string]any{"type": "webhook_import"}))["records"].([]any)
	if len(meta) != 1 || meta[0].(map[string]any)["id"] != imported.ID.String() {
		t.Fatalf("import ownership failed: %+v", meta)
	}
	if _, ok := meta[0].(map[string]any)["raw"]; ok {
		t.Fatal("large original JSON in listing")
	}
	var assembled strings.Builder
	offset := 0
	var checksum string
	for attempts := 0; attempts < 10; attempts++ {
		out := healthTextParity(t, phoneCall(t, h, "get_health_import", map[string]any{"import_id": imported.ID.String(), "offset": offset}))
		data := out["data"].(string)
		if !utf8.ValidString(data) || out["offset"] != float64(offset) || out["total_bytes"] != float64(len(raw)) {
			t.Fatalf("invalid byte chunk: %+v", out)
		}
		if checksum != "" && checksum != out["sha256"] {
			t.Fatal("payload changed between chunks")
		}
		checksum = out["sha256"].(string)
		assembled.WriteString(data)
		next := int(out["next_offset"].(float64))
		if next != offset+len(data) {
			t.Fatal("chunk offset skips bytes")
		}
		if out["has_more"] == false {
			offset = next
			break
		}
		if len(data) == 0 || len(data) > 8192 {
			t.Fatal("unbounded or stalled chunk")
		}
		offset = next
	}
	if assembled.String() != raw || offset != len(raw) {
		t.Fatal("unknown imported fields lost or UTF-8 split")
	}
	hash := sha256.Sum256([]byte(raw))
	if checksum != hex.EncodeToString(hash[:]) {
		t.Fatal("import digest does not identify returned bytes")
	}
	for _, args := range []map[string]any{
		{"import_id": other.ID.String()}, {"import_id": imported.ID.String(), "offset": -1},
		{"import_id": imported.ID.String(), "offset": len(raw) + 1}, {"import_id": "invalid"},
		{"import_id": imported.ID.String(), "offset": strings.Index(raw, "ž") + 1},
	} {
		if phoneCall(t, h, "get_health_import", args)["isError"] != true {
			t.Fatalf("invalid import read accepted: %+v", args)
		}
	}
	if err := st.DB().Delete(&imported).Error; err != nil {
		t.Fatal(err)
	}
	if phoneCall(t, h, "get_health_import", map[string]any{"import_id": imported.ID.String()})["isError"] != true {
		t.Fatal("deleted import accessible")
	}
}

func TestPhoneHealthReadBarrierAndRemovedIdentity(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, _ := seedFoodUser(t, st)
	barrier := &sync.RWMutex{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), barrier)
	calls := []struct {
		name string
		args map[string]any
	}{
		{"list_health_data_types", map[string]any{}},
		{"query_health_data", map[string]any{"type": "weight"}},
		{"get_health_import", map[string]any{"import_id": uuid.NewString()}},
		{"get_health_record", map[string]any{"type": "weight", "record_id": uuid.NewString()}},
	}
	for _, call := range calls {
		barrier.Lock()
		done := make(chan struct{})
		go func() {
			defer close(done)
			phoneCall(t, h, call.name, call.args)
		}()
		select {
		case <-done:
			barrier.Unlock()
			t.Fatalf("%s bypassed capture barrier", call.name)
		case <-time.After(20 * time.Millisecond):
		}
		barrier.Unlock()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s stayed blocked after capture", call.name)
		}
	}
	// The configured identity is resolved per call, rather than captured when
	// the long-lived handler is constructed.
	if err := st.DB().Exec("DELETE FROM users WHERE id = ?", owner).Error; err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if phoneCall(t, h, call.name, call.args)["isError"] != true {
			t.Fatalf("%s accepted removed configured identity", call.name)
		}
	}
}
