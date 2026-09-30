package vision_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/vision"
)

func TestNutritionChatProviderErrorClassification(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		retryable bool
	}{
		{"unsupported parameter", 400, `{"error":{"message":"Unsupported parameter: reasoning_effort; SECRET QUESTION", "code":"unsupported_parameter"}}`, false},
		{"unauthorized", 401, `{}`, false},
		{"forbidden", 403, `{}`, false},
		{"not found", 404, `{}`, false},
		{"request timeout", 408, `<html>timeout</html>`, true},
		{"rate limited", 429, `{"error":{"type":"rate_limit_error"}}`, true},
		{"quota code", 429, `{"error":{"code":"insufficient_quota"}}`, false},
		{"quota type", 429, `{"error":{"type":"insufficient_quota"}}`, false},
		{"credit balance", 429, `{"error":{"code":"credit_balance_exhausted"}}`, false},
		{"organization spending", 429, `{"error":{"code":"organization_spend_limit_exceeded"}}`, false},
		{"project spending", 429, `{"error":{"code":"project_spend_limit_exceeded"}}`, false},
		{"organization usage", 429, `{"error":{"code":"organization_usage_limit_exceeded"}}`, false},
		{"server JSON", 503, `{"error":{"message":"SECRET QUESTION","code":"server_error"}}`, true},
		{"server HTML", 502, `<html>bad gateway</html>`, true},
		{"server plain text", 503, `service unavailable`, true},
		{"server unreadable JSON", 500, `{`, true},
		{"bad request HTML", 400, `<html>bad request</html>`, false},
		{"bad successful JSON", 200, `{`, false},
		{"refusal", 200, `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal"}]}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.NutritionChat(context.Background(), vision.NutritionChatInput{Question: "SECRET QUESTION"})
			if err == nil {
				t.Fatal("expected failure")
			}
			if got := vision.NutritionChatErrorRetryable(fmt.Errorf("wrapped: %w", err)); got != tc.retryable {
				t.Fatalf("retryable=%t want %t: %v", got, tc.retryable, err)
			}
			if tc.status != 200 {
				var typed *vision.NutritionChatError
				if !errors.As(err, &typed) || typed.StatusCode != tc.status {
					t.Fatalf("lost HTTP status: %v", err)
				}
			}
			if strings.Contains(err.Error(), "SECRET QUESTION") {
				t.Fatal("provider message leaked")
			}
			if requests != 1 {
				t.Fatalf("unexpected internal retries: %d", requests)
			}
		})
	}
}

type nutritionChatTransport func(*http.Request) (*http.Response, error)

func (f nutritionChatTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type nutritionChatTimeout struct{}

func (nutritionChatTimeout) Error() string   { return "timeout" }
func (nutritionChatTimeout) Timeout() bool   { return true }
func (nutritionChatTimeout) Temporary() bool { return true }

func TestNutritionChatTransportAndUnknownErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		retryable bool
	}{
		{"timeout", nutritionChatTimeout{}, true},
		{"connection closed", io.EOF, true},
		{"truncated connection", io.ErrUnexpectedEOF, true},
		{"deadline", context.DeadlineExceeded, true},
		{"canceled", context.Canceled, false},
		{"unknown", errors.New("something failed"), false},
		{"unconfigured", vision.ErrNotConfigured, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := vision.NewOpenAIClient("test-key", "gpt-6-luna")
			c.HTTPClient = &http.Client{Transport: nutritionChatTransport(func(*http.Request) (*http.Response, error) { return nil, tc.err })}
			_, err := c.NutritionChat(context.Background(), vision.NutritionChatInput{Question: "why?"})
			if got := vision.NutritionChatErrorRetryable(err); got != tc.retryable {
				t.Fatalf("retryable=%t want %t: %v", got, tc.retryable, err)
			}
		})
	}
}

// A response body cut off after HTTP headers must not lose that HTTP verdict.
type truncatedNutritionBody struct{}

func (truncatedNutritionBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestNutritionChatTruncatedResponseClassification(t *testing.T) {
	for _, tc := range []struct {
		status    int
		retryable bool
	}{{200, true}, {400, false}, {503, true}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			c := vision.NewOpenAIClient("test-key", "gpt-6-luna")
			c.HTTPClient = &http.Client{Transport: nutritionChatTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(truncatedNutritionBody{}), Header: make(http.Header)}, nil
			})}
			_, err := c.NutritionChat(context.Background(), vision.NutritionChatInput{Question: "why?"})
			if err == nil || vision.NutritionChatErrorRetryable(err) != tc.retryable {
				t.Fatalf("status=%d retryable=%t err=%v", tc.status, tc.retryable, err)
			}
		})
	}
}
