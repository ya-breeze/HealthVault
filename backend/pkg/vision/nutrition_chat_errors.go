package vision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
)

// NutritionChatError preserves provider status without retaining its message or prompt.
type NutritionChatError struct {
	StatusCode int
	Code       string
	Retryable  bool
}

func (e *NutritionChatError) Error() string {
	return fmt.Sprintf("nutrition chat provider failure (status %d, retryable %t)", e.StatusCode, e.Retryable)
}

// NutritionChatErrorRetryable defaults unknown failures to final errors.
func NutritionChatErrorRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrNotConfigured) {
		return false
	}
	var provider *NutritionChatError
	if errors.As(err, &provider) {
		return provider.Retryable
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// http.Client wraps every transport failure in url.Error, including
	// unknown errors. Inspect its cause instead of treating the wrapper as
	// evidence of a network failure.
	var request *url.Error
	if errors.As(err, &request) {
		return NutritionChatErrorRetryable(request.Err)
	}
	var network net.Error
	return errors.As(err, &network)
}

func nutritionChatProviderError(status int, code, errorType string) *NutritionChatError {
	retryable := status >= 500 && status <= 599 || status == 408 || status == 429
	switch code {
	case "insufficient_quota", "credit_balance_exhausted", "organization_spend_limit_exceeded", "project_spend_limit_exceeded", "organization_usage_limit_exceeded":
		retryable = false
	}
	if errorType == "insufficient_quota" {
		retryable = false
	}
	return &NutritionChatError{StatusCode: status, Code: code, Retryable: retryable}
}
