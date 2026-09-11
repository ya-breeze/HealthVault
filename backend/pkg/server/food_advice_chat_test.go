package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

type chatTestResponse struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Answer    string `json:"answer"`
}

func chatSignal(code, unit, verdict, reason string, value, off, far float64) map[string]any {
	signal := map[string]any{
		"code": code, "value": value, "unit": unit, "verdict": verdict,
		"off_boundary": off, "far_boundary": far,
	}
	if reason != "" {
		signal["reason"] = reason
	}
	return signal
}

func chatBody(question string, turns []map[string]any) map[string]any {
	return map[string]any{
		"label":   "needs_attention",
		"reasons": []string{"sodium_high"},
		"window": map[string]any{
			"mean_calories": 1820.5, "mean_protein_grams": 74.25,
			"mean_carbs_grams": 210.25, "mean_fat_grams": 62.5,
			"mean_sugar_grams": 88.75, "mean_sodium_grams": 4.1,
		},
		"signals": []map[string]any{
			chatSignal("sodium", "gramsPerDay", "far", "sodium_high", 4.1, 2.3, 3.5),
			chatSignal("protein", "share", "ok", "", 0.22, 0.15, 0.1),
		},
		"eligible_days": 5,
		"turns":         turns,
		"question":      question,
	}
}

func newChatRequest(t *testing.T, userID, familyID uuid.UUID, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal chat body: %v", err)
	}
	return withClaimsFamily(
		httptest.NewRequest(http.MethodPost, "/api/food/advice/chat", bytes.NewReader(b)),
		userID, familyID,
	)
}

func callChat(t *testing.T, h interface {
	PostFoodAdviceChat(http.ResponseWriter, *http.Request)
}, req *http.Request) (*httptest.ResponseRecorder, chatTestResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	h.PostFoodAdviceChat(w, req)
	var response chatTestResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode chat response: %v", err)
		}
	}
	return w, response
}

func TestFoodAdviceChat_AnswersFromTheCallersOwnEvidence(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "ru-RU")
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	turns := []map[string]any{
		{"role": "user", "text": "я уже уменьшил соль"},
		{"role": "assistant", "text": "за какие дни вы это сделали?"},
	}
	w, response := callChat(t, h, newChatRequest(t, userID, familyID, chatBody("  за какие дни?  ", turns)))
	if w.Code != http.StatusOK || !response.Available {
		t.Fatalf("chat: %d: %s", w.Code, w.Body.String())
	}
	if response.Answer != "fake answer to: за какие дни?" {
		t.Fatalf("answer did not travel through the handler: %q", response.Answer)
	}
	if len(fake.NutritionChatCalls) != 1 {
		t.Fatalf("expected exactly one model call, got %d", len(fake.NutritionChatCalls))
	}
	in := fake.NutritionChatCalls[0]
	if in.Question != "за какие дни?" {
		t.Errorf("expected a trimmed question, got %q", in.Question)
	}
	if len(in.Turns) != 2 || in.Turns[0].Role != "user" || in.Turns[1].Role != "assistant" {
		t.Fatalf("expected the prior turns replayed in order, got %#v", in.Turns)
	}
	if in.Turns[0].Text != "я уже уменьшил соль" {
		t.Errorf("first turn did not survive: %q", in.Turns[0].Text)
	}
	if in.EligibleDays != 5 || in.WindowDays != 7 {
		t.Errorf("expected 5 eligible days over a 7-day window, got %d over %d", in.EligibleDays, in.WindowDays)
	}
	if len(in.Signals) != 2 || in.Signals[0].Code != "sodium" || in.Signals[0].OffBoundary != 2.3 {
		t.Fatalf("signals did not survive: %#v", in.Signals)
	}
	if in.Signals[1].Reason != "" || in.Signals[1].Verdict != "ok" {
		t.Errorf("an ok signal must carry no reason code: %#v", in.Signals[1])
	}
	// The target and the language are the server's, resolved from the caller's
	// own profile, never anything the request could have named.
	if in.TargetCalories == 0 || in.DisplayLanguage != "ru" {
		t.Errorf("expected a server-resolved target and language, got %d / %q", in.TargetCalories, in.DisplayLanguage)
	}
	if in.MeanSodiumGrams != 4.1 {
		t.Errorf("expected the posted window means, got %v", in.MeanSodiumGrams)
	}
}

func TestFoodAdviceChat_AuthenticationOriginAndInput(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	t.Run("no claims is unauthorized", func(t *testing.T) {
		b, err := json.Marshal(chatBody("why?", nil))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		w := httptest.NewRecorder()
		h.PostFoodAdviceChat(w, httptest.NewRequest(http.MethodPost, "/api/food/advice/chat", bytes.NewReader(b)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("a cross-site request is forbidden", func(t *testing.T) {
		for _, site := range []string{"cross-site", "same-site", "none"} {
			req := newChatRequest(t, userID, familyID, chatBody("why?", nil))
			req.Header.Set("Sec-Fetch-Site", site)
			w := httptest.NewRecorder()
			h.PostFoodAdviceChat(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("Sec-Fetch-Site %q status = %d, want 403", site, w.Code)
			}
		}
	})

	longQuestion := strings.Repeat("я", 501)
	longAnswer := strings.Repeat("я", 901)
	tooManyTurns := make([]map[string]any, 0, 9)
	for i := 0; i < 9; i++ {
		tooManyTurns = append(tooManyTurns, map[string]any{"role": "user", "text": "q"})
	}

	for name, mutate := range map[string]func(map[string]any){
		"an empty question":        func(b map[string]any) { b["question"] = "   " },
		"an over-long question":    func(b map[string]any) { b["question"] = longQuestion },
		"an over-long answer turn": func(b map[string]any) { b["turns"] = []map[string]any{{"role": "assistant", "text": longAnswer}} },
		"an empty turn":            func(b map[string]any) { b["turns"] = []map[string]any{{"role": "user", "text": " "}} },
		"an unknown turn role":     func(b map[string]any) { b["turns"] = []map[string]any{{"role": "system", "text": "x"}} },
		"too many turns":           func(b map[string]any) { b["turns"] = tooManyTurns },
		"an invalid label":         func(b map[string]any) { b["label"] = "excellent" },
		"an invalid reason code":   func(b map[string]any) { b["reasons"] = []string{"Sodium High"} },
		"a missing window mean":    func(b map[string]any) { delete(b["window"].(map[string]any), "mean_sodium_grams") },
		"an unknown verdict": func(b map[string]any) {
			b["signals"] = []map[string]any{chatSignal("sodium", "gramsPerDay", "terrible", "sodium_high", 4.1, 2.3, 3.5)}
		},
		"an unknown unit": func(b map[string]any) {
			b["signals"] = []map[string]any{chatSignal("sodium", "milligrams", "far", "sodium_high", 4.1, 2.3, 3.5)}
		},
		"a duplicated signal": func(b map[string]any) {
			s := chatSignal("sodium", "gramsPerDay", "far", "sodium_high", 4.1, 2.3, 3.5)
			b["signals"] = []map[string]any{s, s}
		},
		"an ok signal with a reason": func(b map[string]any) {
			b["signals"] = []map[string]any{chatSignal("sodium", "gramsPerDay", "ok", "sodium_high", 1.1, 2.3, 3.5)}
		},
		"a flagged signal with no reason": func(b map[string]any) {
			b["signals"] = []map[string]any{chatSignal("sodium", "gramsPerDay", "far", "", 4.1, 2.3, 3.5)}
		},
		"an out-of-range eligible day count": func(b map[string]any) { b["eligible_days"] = 8 },
		"an unknown field":                   func(b map[string]any) { b["user"] = "someone-else" },
	} {
		t.Run(name, func(t *testing.T) {
			body := chatBody("why?", nil)
			mutate(body)
			w, _ := callChat(t, h, newChatRequest(t, userID, familyID, body))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			if len(fake.NutritionChatCalls) != 0 {
				t.Fatalf("a rejected request reached the model: %#v", fake.NutritionChatCalls)
			}
		})
	}

	t.Run("trailing JSON", func(t *testing.T) {
		b, err := json.Marshal(chatBody("why?", nil))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := withClaimsFamily(httptest.NewRequest(
			http.MethodPost, "/api/food/advice/chat", bytes.NewReader(append(b, []byte(`{"extra":1}`)...)),
		), userID, familyID)
		w := httptest.NewRecorder()
		h.PostFoodAdviceChat(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	// The byte cap is defence in depth: the turn and question limits already
	// keep a legitimate body under it, so reaching it takes a body no valid
	// client would send. It must still fail at the reader rather than being
	// decoded.
	t.Run("an over-sized body", func(t *testing.T) {
		body := chatBody("why?", nil)
		turns := make([]map[string]any, 0, 30)
		for i := 0; i < 30; i++ {
			turns = append(turns, map[string]any{"role": "assistant", "text": strings.Repeat("я", 900)})
		}
		body["turns"] = turns
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if len(b) <= 16<<10 {
			t.Fatalf("fixture is only %d bytes, which does not exceed the 16 KiB limit", len(b))
		}
		w, _ := callChat(t, h, newChatRequest(t, userID, familyID, body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestFoodAdviceChat_ModelFailureIsReportedAsUnavailable(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")

	for name, client := range map[string]vision.Client{
		"a model failure":         &vision.Fake{NutritionChatErr: errors.New("model down: за какие дни?")},
		"an unconfigured api key": vision.Unconfigured{},
	} {
		t.Run(name, func(t *testing.T) {
			h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(client, 10<<20, time.Second)
			w, response := callChat(t, h, newChatRequest(t, userID, familyID, chatBody("why?", nil)))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 carrying an unavailable body: %s", w.Code, w.Body.String())
			}
			if response.Available || response.Reason != "unavailable" || response.Answer != "" {
				t.Fatalf("expected an unavailable response, got %+v", response)
			}
			if strings.Contains(w.Body.String(), "model down") {
				t.Error("the model's own error text reached the caller")
			}
		})
	}
}

func TestFoodAdviceChat_WithoutATargetIsUnavailable(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	// No profile and no weight, so no Nutrition Target can be computed.
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w, response := callChat(t, h, newChatRequest(t, userID, familyID, chatBody("why?", nil)))
	if w.Code != http.StatusOK || response.Available || response.Reason != "unavailable" {
		t.Fatalf("expected an unavailable response, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.NutritionChatCalls) != 0 {
		t.Fatal("the model was called without a resolved target")
	}
}

func TestFoodAdviceChat_CallerCannotReachAnotherUsersEvidence(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	otherUserID, otherFamilyID := seedSecondFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "ru-RU")
	fake := &vision.Fake{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	// The other user has no profile, so their own target cannot be computed.
	// Claims decide whose profile is read, and the request carries no field
	// that could name someone else, so this must be unavailable rather than
	// answering from the first user's target.
	w, response := callChat(t, h, newChatRequest(t, otherUserID, otherFamilyID, chatBody("why?", nil)))
	if w.Code != http.StatusOK || response.Available {
		t.Fatalf("expected an unavailable response for a user with no target, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.NutritionChatCalls) != 0 {
		t.Fatalf("the other user's request reached the model: %#v", fake.NutritionChatCalls)
	}
}
