package backupapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testCredential = "backup-test-credential_123"
const testPayload = `{"trigger":"pre_deploy","target_revision":"5cda0c8","metadata":{"stack":"hcw-wip"}}`

func openTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return db
}

func newTestService(t *testing.T, runner Runner) *Service {
	t.Helper()
	dir := t.TempDir()
	service, err := New(openTestDB(t, filepath.Join(dir, "jobs.sqlite")), runner)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func testRequest(method, path, body, credential, key string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func responseJSON(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON response: %v: %s", err, recorder.Body.String())
	}
	return response
}

func TestHandlerAuthenticationAndClosedErrors(t *testing.T) {
	handler := NewHandler(testCredential, newTestService(t, nil))
	for _, test := range []struct {
		name       string
		credential string
		wantStatus int
		wantCode   string
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "wrong", credential: "wrong-token", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "valid", credential: testCredential, wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, testRequest(http.MethodGet, statusPath, "", test.credential, ""))
			response := responseJSON(t, recorder)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if test.wantCode != "" && response["error"].(map[string]any)["code"] != test.wantCode {
				t.Fatalf("error = %#v", response["error"])
			}
		})
	}
	for _, target := range []string{"/internal/backups/v2/status", "/internal/backups/v1/unknown", "/internal/backups/v1/jobs/j1/extra"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, testRequest(http.MethodGet, target, "", testCredential, ""))
		response := responseJSON(t, recorder)
		if recorder.Code != http.StatusNotFound || response["error"].(map[string]any)["code"] != "not_found" {
			t.Fatalf("%s => %d %s", target, recorder.Code, recorder.Body.String())
		}
	}
	for _, bad := range []string{"", "bad token", strings.Repeat("x", 513)} {
		unconfigured := httptest.NewRecorder()
		NewHandler(bad, newTestService(t, nil)).ServeHTTP(unconfigured,
			testRequest(http.MethodGet, statusPath, "", bad, ""))
		response := responseJSON(t, unconfigured)
		if unconfigured.Code != http.StatusServiceUnavailable || response["error"].(map[string]any)["code"] != "unavailable" {
			t.Fatalf("invalid configured credential => %d %s", unconfigured.Code, unconfigured.Body.String())
		}
	}
}

func TestMalformedRequestsAreJSON400(t *testing.T) {
	handler := NewHandler(testCredential, newTestService(t, nil))
	for _, test := range []struct {
		name, body, key, contentType string
	}{
		{name: "malformed json", body: "{", key: "k", contentType: "application/json"},
		{name: "unknown field", body: `{"trigger":"pre_deploy","target_revision":"rev","extra":true}`, key: "k", contentType: "application/json"},
		{name: "missing revision", body: `{"trigger":"pre_deploy"}`, key: "k", contentType: "application/json"},
		{name: "wrong trigger", body: `{"trigger":"manual","target_revision":"rev"}`, key: "k", contentType: "application/json"},
		{name: "invalid revision", body: `{"trigger":"pre_deploy","target_revision":"\n"}`, key: "k", contentType: "application/json"},
		{name: "revision contains path", body: `{"trigger":"pre_deploy","target_revision":"/private/path/file"}`, key: "k", contentType: "application/json"},
		{name: "revision contains short absolute path", body: `{"trigger":"pre_deploy","target_revision":"/data"}`, key: "k", contentType: "application/json"},
		{name: "revision contains secret assignment", body: `{"trigger":"pre_deploy","target_revision":"token=othersecret"}`, key: "k", contentType: "application/json"},
		{name: "metadata too large", body: `{"trigger":"pre_deploy","target_revision":"rev","metadata":{"a":"` + strings.Repeat("x", 257) + `"}}`, key: "k", contentType: "application/json"},
		{name: "metadata null value", body: `{"trigger":"pre_deploy","target_revision":"rev","metadata":{"stack":null}}`, key: "k", contentType: "application/json"},
		{name: "missing key", body: testPayload, contentType: "application/json"},
		{name: "invalid key", body: testPayload, key: "spaces not allowed", contentType: "application/json"},
		{name: "wrong content type", body: testPayload, key: "k", contentType: "text/plain"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := testRequest(http.MethodPost, jobsPath, test.body, testCredential, test.key)
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			response := responseJSON(t, recorder)
			if recorder.Code != http.StatusBadRequest || response["error"].(map[string]any)["code"] != "invalid_request" {
				t.Fatalf("got %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestScheduledTriggerIsAccepted(t *testing.T) {
	request, ok := parseRequest([]byte(`{"trigger":"scheduled","target_revision":"currently-running-revision"}`))
	if !ok || request.Trigger != "scheduled" || request.TargetRevision != "currently-running-revision" {
		t.Fatalf("scheduled trigger was rejected: request=%+v ok=%v", request, ok)
	}
}

func TestIdempotentReplayConflictAndLifecycle(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	service := newTestService(t, RunnerFunc(func(context.Context, Job) (*Evidence, *Error) {
		close(started)
		<-finish
		return nil, &Error{Code: "storage_unconfigured", Message: "backup storage is not configured"}
	}))
	handler := NewHandler(testCredential, service)
	post := func(body string) (*httptest.ResponseRecorder, map[string]any) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, testRequest(http.MethodPost, jobsPath, body, testCredential, "same-key"))
		return recorder, responseJSON(t, recorder)
	}
	first, firstJSON := post(testPayload)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first POST status = %d: %s", first.Code, first.Body.String())
	}
	<-started
	ref := firstJSON["job_id"].(string)
	active, err := service.Get(context.Background(), ref)
	if err != nil || active.State != "running" {
		t.Fatalf("runner did not move job to running: %#v, %v", active, err)
	}
	second, secondJSON := post(testPayload)
	if second.Code != http.StatusAccepted || secondJSON["job_id"] != ref {
		t.Fatalf("replay did not return original job: %d %s", second.Code, second.Body.String())
	}
	conflictBody := `{"trigger":"pre_deploy","target_revision":"different"}`
	conflict, conflictJSON := post(conflictBody)
	if conflict.Code != http.StatusConflict || conflictJSON["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatalf("conflict = %d %s", conflict.Code, conflict.Body.String())
	}
	close(finish)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := service.Get(context.Background(), ref)
		if err == nil && job.State == "failed" {
			if job.Error == nil || job.Error.Code != "storage_unconfigured" || job.Evidence != nil || job.CompletedAt == nil {
				t.Fatalf("bad failed job: %#v", job)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not complete")
}

func TestRestartRecoveryFailsActiveJobsWithoutRerunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.sqlite")
	db := openTestDB(t, path)
	_, err := New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	if err := db.Create(&row{ID: "j_pending", IdempotencyKey: "persisted-key", RequestSHA256: strings.Repeat("a", 64),
		Trigger: "pre_deploy", TargetRevision: "rev", State: "running", CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	restartedDB := openTestDB(t, path)
	restarted, err := New(restartedDB, RunnerFunc(func(context.Context, Job) (*Evidence, *Error) {
		t.Error("recovered job must not be rerun")
		return nil, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	job, err := restarted.Get(context.Background(), "j_pending")
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "failed" || job.Error == nil || job.Error.Code != "internal" || job.CompletedAt == nil {
		t.Fatalf("active job was not safely failed on restart: %#v", job)
	}
}

func TestStatusRetainsMostRecentSuccessAfterFailure(t *testing.T) {
	service := newTestService(t, nil)
	success := row{ID: "j_success", IdempotencyKey: "success-key", RequestSHA256: strings.Repeat("a", 64),
		Trigger: "pre_deploy", TargetRevision: "old", State: "succeeded", CreatedAt: time.Now().UTC().Add(-time.Minute),
		CompletedAt: timePtr(time.Now().UTC().Add(-time.Second))}
	evidence, _ := json.Marshal(Evidence{BackupSetType: "full", ManifestSHA256: strings.Repeat("a", 64), ReadyFileID: "6f0d5b9a-61dc-4c53-b15e-64d46273eec1.age",
		CiphertextSHA256: strings.Repeat("b", 64), CiphertextSizeBytes: 1, EncryptionKeyID: "key-1"})
	evidenceText := string(evidence)
	success.EvidenceJSON = &evidenceText
	if err := service.db.Create(&success).Error; err != nil {
		t.Fatal(err)
	}
	failed := row{ID: "j_failed", IdempotencyKey: "failed-key", RequestSHA256: strings.Repeat("c", 64),
		Trigger: "pre_deploy", TargetRevision: "new", State: "failed", CreatedAt: time.Now().UTC(),
		CompletedAt: timePtr(time.Now().UTC()), ErrorJSON: strPtr(`{"code":"storage_unconfigured","message":"not configured"}`)}
	if err := service.db.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	result, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.LastAttempt == nil || result.LastAttempt.JobID != "j_failed" || result.LastSuccess == nil || result.LastSuccess.JobID != "j_success" {
		t.Fatalf("unexpected status: %#v", result)
	}
}

func TestResponsesNeverEchoCredentialOrRequestData(t *testing.T) {
	service := newTestService(t, nil)
	handler := NewHandler(testCredential, service)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, testRequest(http.MethodPost, jobsPath,
		`{"trigger":"pre_deploy","target_revision":"rev","metadata":{"label":"`+testCredential+`"}}`, testCredential, "key"))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), testCredential) || strings.Contains(recorder.Body.String(), "label") {
		t.Fatalf("response leaked credential or metadata: %s", recorder.Body.String())
	}
	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, testRequest(http.MethodGet, statusPath, "", "wrong-secret", ""))
	if strings.Contains(bad.Body.String(), "wrong-secret") || strings.Contains(bad.Body.String(), testCredential) {
		t.Fatalf("error response leaked a credential: %s", bad.Body.String())
	}
	// The POST schedules a background job. Let it finish before the temporary
	// database is closed by test cleanup.
	var ref JobReference
	if err := json.Unmarshal(recorder.Body.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := service.Get(context.Background(), ref.JobID)
		if err == nil && (job.State == "failed" || job.State == "succeeded") {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background job did not finish before test cleanup")
}

func TestRequestBodyLimit(t *testing.T) {
	handler := NewHandler(testCredential, newTestService(t, nil))
	body := fmt.Sprintf(`{"trigger":"pre_deploy","target_revision":"%s"}`, strings.Repeat("x", maxBody))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, testRequest(http.MethodPost, jobsPath, body, testCredential, "large"))
	response := responseJSON(t, recorder)
	if recorder.Code != http.StatusBadRequest || response["error"].(map[string]any)["code"] != "invalid_request" {
		t.Fatalf("oversized body got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestUnconfiguredRunnerReturnsStableFailure(t *testing.T) {
	evidence, failure := UnconfiguredRunner(context.Background(), Job{})
	if evidence != nil || failure == nil || failure.Code != "storage_unconfigured" {
		t.Fatalf("runner = %#v, %#v", evidence, failure)
	}
}

func TestDefaultRunnerFailsClosedAfterAcceptingJob(t *testing.T) {
	service := newTestService(t, nil)
	handler := NewHandler(testCredential, service)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, testRequest(http.MethodPost, jobsPath, testPayload, testCredential, "default-runner"))
	response := responseJSON(t, recorder)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d: %s", recorder.Code, recorder.Body.String())
	}
	jobID := response["job_id"].(string)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := service.Get(context.Background(), jobID)
		if err == nil && job.State == "failed" {
			if job.Error == nil || job.Error.Code != "storage_unconfigured" || job.Error.Message != "backup storage is not configured" {
				t.Fatalf("unexpected unconfigured failure: %#v", job)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("default runner left the job pending")
}

func TestServiceSerializesBackupJobs(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	defer close(release)
	service := newTestService(t, RunnerFunc(func(_ context.Context, job Job) (*Evidence, *Error) {
		started <- job.JobID
		<-release
		return nil, &Error{Code: "publish_failed"}
	}))
	request := Request{Trigger: "pre_deploy", TargetRevision: "revision-a"}
	first, _, err := service.Create(context.Background(), "serial-a", request)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-started:
		if id != first.JobID {
			t.Fatalf("first runner job = %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first job did not start")
	}
	request.TargetRevision = "revision-b"
	second, _, err := service.Create(context.Background(), "serial-b", request)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-started:
		t.Fatalf("second job %s began before the first finished", id)
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case id := <-started:
		if id != second.JobID {
			t.Fatalf("second runner job = %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second job did not start")
	}
	release <- struct{}{}
	for _, id := range []string{first.JobID, second.JobID} {
		deadline := time.Now().Add(2 * time.Second)
		for {
			job, err := service.Get(context.Background(), id)
			if err == nil && job.State == "failed" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("job %s did not finish before cleanup", id)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func TestHandlerConfiguredTokenMustBeValidASCII(t *testing.T) {
	if validCredential("a") == false || validCredential(strings.Repeat("a", 512)) == false {
		t.Fatal("valid credential rejected")
	}
	for _, value := range []string{"", strings.Repeat("x", 513), "café", "with space"} {
		if validCredential(value) {
			t.Fatalf("invalid credential accepted: %q", value)
		}
	}
}

func timePtr(value time.Time) *time.Time { return &value }
func strPtr(value string) *string        { return &value }
