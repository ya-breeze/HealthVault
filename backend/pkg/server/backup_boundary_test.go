package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNginxKeepsInternalBackupAPIPrivate(t *testing.T) {
	config, err := os.ReadFile("../../../nginx/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	start := strings.Index(text, "location ^~ /internal/backups {")
	if start < 0 {
		t.Fatal("nginx must define an explicit private backup boundary")
	}
	end := strings.Index(text[start:], "\n    }")
	if end < 0 {
		t.Fatal("nginx backup boundary block is not closed")
	}
	block := text[start : start+end]
	if !strings.Contains(block, "return 404") || strings.Contains(block, "proxy_pass") || strings.Contains(block, "try_files") {
		t.Fatalf("nginx backup boundary must return 404 without proxying or SPA fallback: %s", block)
	}
}

func TestCaptureBarrierKeepsBackupStatusReadinessAndMCPReachable(t *testing.T) {
	barrier := &sync.RWMutex{}
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := captureAwareHandler(router, barrier)
	barrier.Lock()
	defer barrier.Unlock()
	for _, path := range []string{"/internal/backups/v1/status", "/internal/ready", "/mcp"} {
		completed := make(chan int, 1)
		go func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			completed <- recorder.Code
		}()
		select {
		case code := <-completed:
			if code != http.StatusNoContent {
				t.Fatalf("%s returned %d", path, code)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s blocked behind backup capture", path)
		}
	}
}
