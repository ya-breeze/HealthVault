package vision

import (
	"context"
	"errors"
)

// ErrNotConfigured is returned by Unconfigured for every call.
var ErrNotConfigured = errors.New("OpenAI Vision is not configured (HCW_OPENAI_API_KEY is empty)")

// Unconfigured is the Client used when no OpenAI API key is set. Every call
// fails immediately rather than the server refusing to start: most of food
// logging (manual entry, custom foods, search) needs no vision access at
// all, and photo uploads should fail per-request as "failed", not take the
// whole server down.
type Unconfigured struct{}

func (Unconfigured) Recognize(context.Context, []byte, string) (*RecognizeResult, error) {
	return nil, ErrNotConfigured
}

func (Unconfigured) Clarify(context.Context, []Item, []ClarifyTurn) (*RecognizeResult, error) {
	return nil, ErrNotConfigured
}

func (Unconfigured) Select(context.Context, []ItemCandidates) (*SelectResult, error) {
	return nil, ErrNotConfigured
}
