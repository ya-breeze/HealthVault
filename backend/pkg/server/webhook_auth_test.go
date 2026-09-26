package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireWebhookToken(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		headers    []string
		wantStatus int
		wantCalls  int
	}{
		{"unset token", "", []string{"valid-token"}, http.StatusServiceUnavailable, 0},
		{"whitespace token", "   ", []string{"   "}, http.StatusServiceUnavailable, 0},
		{"padded token", "valid-token ", []string{"valid-token"}, http.StatusServiceUnavailable, 0},
		{"missing header", "valid-token", nil, http.StatusUnauthorized, 0},
		{"wrong header", "valid-token", []string{"wrong-token"}, http.StatusUnauthorized, 0},
		{"duplicate header", "valid-token", []string{"valid-token", "valid-token"}, http.StatusUnauthorized, 0},
		{"correct header", "valid-token", []string{"valid-token"}, http.StatusNoContent, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			handler := requireWebhookToken(tt.configured, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != "payload" {
					t.Fatalf("handler received body %q, error %v", body, err)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodPost, "/webhook/alice", strings.NewReader("payload"))
			for _, value := range tt.headers {
				req.Header.Add(webhookTokenHeader, value)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != tt.wantStatus || calls != tt.wantCalls {
				t.Fatalf("status=%d calls=%d, want status=%d calls=%d", resp.Code, calls, tt.wantStatus, tt.wantCalls)
			}
			if calls == 0 {
				body, err := io.ReadAll(req.Body)
				if err != nil || string(body) != "payload" {
					t.Fatalf("rejected request body was read: body=%q err=%v", body, err)
				}
			}
		})
	}
}
