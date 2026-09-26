package backupapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"filippo.io/age"
)

type conformanceRunner struct {
	inner    *SetRunner
	failNext atomic.Bool
}

func (s *conformanceRunner) Run(ctx context.Context, job Job) (*Evidence, *Error) {
	if s.failNext.Swap(false) {
		return nil, &Error{Code: "publish_failed"}
	}
	return s.inner.Run(ctx, job)
}

func TestIdeaForgeBackupConformanceAgainstDisposableHTTPHandler(t *testing.T) {
	forgeRoot := os.Getenv("IDEA_FORGE_ROOT")
	if forgeRoot == "" {
		t.Skip("set IDEA_FORGE_ROOT to run the cross-repository conformance check")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("IDEA_FORGE_ROOT is set but python3 is unavailable: %v", err)
	}
	if info, err := os.Stat(filepath.Join(forgeRoot, "idea_forge", "backup_conformance.py")); err != nil || info.IsDir() {
		t.Fatalf("IdeaForge conformance runner missing at %q: %v", forgeRoot, err)
	}
	fx := newBackupFixture(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	baseRunner := fixtureRunner(t, fx, identity.Recipient().String())
	runner := &conformanceRunner{inner: baseRunner}
	service := newTestService(t, runner)
	mux := http.NewServeMux()
	mux.Handle("/internal/backups/", NewHandler(testCredential, service))
	mux.HandleFunc("/__test__/fail-next", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		runner.failNext.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	python := `
import sys
import urllib.request
from idea_forge import backup_conformance

base, credential = sys.argv[1], sys.argv[2]
def arrange_failure():
    request = urllib.request.Request(base + "/__test__/fail-next", method="POST")
    with urllib.request.urlopen(request, timeout=3) as response:
        if response.status != 204:
            raise RuntimeError("test store did not arm failure")

report = backup_conformance.run(base, credential, arrange_failure=arrange_failure,
    deadline_seconds=10, interval_seconds=0.01, request_timeout=3,
    revision_prefix="healthvault-disposable")
for check in report.checks:
    print(f"{check.name}: {check.outcome}")
if report.skipped:
    print("unexpected skipped conformance checks:", file=sys.stderr)
    for check in report.skipped:
        print(f"{check.name}: {check.detail}", file=sys.stderr)
    sys.exit(3)
report.raise_for_failures()
`
	cmd := exec.Command("python3", "-c", python, server.URL, testCredential)
	cmd.Env = append(os.Environ(), "PYTHONPATH="+forgeRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("IdeaForge black-box conformance failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "job.failed: pass") || !strings.Contains(string(output), "status.after_failure: pass") {
		t.Fatalf("conformance did not prove the forced failure path:\n%s", output)
	}
	t.Logf("IdeaForge backup conformance results:\n%s", strings.TrimSpace(string(output)))
}
