package server

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactQuestion(t *testing.T) {
	question := "why does it say my sodium is high?"

	t.Run("removes the question a provider echoed back", func(t *testing.T) {
		err := errors.New(`openai: 400 invalid request for prompt "` + question + `"`)
		got := redactQuestion(err, question)
		if strings.Contains(got, question) {
			t.Fatalf("the question survived redaction: %q", got)
		}
		if !strings.Contains(got, "<question>") || !strings.Contains(got, "400 invalid request") {
			t.Fatalf("redaction lost the diagnosable part of the error: %q", got)
		}
	})

	t.Run("bounds a provider error carrying a whole response body", func(t *testing.T) {
		got := redactQuestion(errors.New(strings.Repeat("я", 5000)), question)
		if len([]rune(got)) != nutritionChatMaxLoggedErrorRune+1 {
			t.Fatalf("expected a bounded error plus an ellipsis, got %d runes", len([]rune(got)))
		}
	})

	t.Run("leaves an unrelated error alone", func(t *testing.T) {
		if got := redactQuestion(errors.New("context deadline exceeded"), question); got != "context deadline exceeded" {
			t.Fatalf("an unrelated error was altered: %q", got)
		}
	})

	t.Run("does not blank the error when there is no question to redact", func(t *testing.T) {
		if got := redactQuestion(errors.New("model down"), ""); got != "model down" {
			t.Fatalf("empty question redacted the whole error: %q", got)
		}
	})
}
