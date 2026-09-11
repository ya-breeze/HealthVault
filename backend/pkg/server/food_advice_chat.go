package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

// Limits on one chat request. They are rejections, not truncations: silently
// dropping the tail of a question, or the oldest turns of a conversation,
// would answer something the user did not ask and give them no way to notice.
const (
	nutritionChatMaxBodyBytes    = 16 << 10
	nutritionChatMaxQuestionRune = 500
	// An assistant turn is replayed verbatim, so its limit is the bound the
	// model's own answer is cut to, not the question limit. Taken from the
	// vision package rather than restated, so the two cannot drift.
	nutritionChatMaxAnswerRune = vision.NutritionChatAnswerMaxRunes
	nutritionChatMaxTurns      = 8
	// The Healthiness Label's own window, restated here because the model is
	// told what the means rest on and the server must not guess it.
	nutritionChatWindowDays = 7
	nutritionChatMaxSignals = 5
	// A provider error can carry a whole response body, so the logged form is
	// bounded as well as redacted.
	nutritionChatMaxLoggedErrorRune = 300
)

var nutritionChatVerdicts = map[string]struct{}{"ok": {}, "off": {}, "far": {}}

var nutritionChatUnits = map[string]struct{}{"share": {}, "gramsPerDay": {}}

type nutritionChatSignal struct {
	Code        string   `json:"code"`
	Value       *float64 `json:"value"`
	Unit        string   `json:"unit"`
	Verdict     string   `json:"verdict"`
	Reason      string   `json:"reason,omitempty"`
	OffBoundary *float64 `json:"off_boundary"`
	FarBoundary *float64 `json:"far_boundary"`
}

type nutritionChatTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// nutritionChatRequest carries no user identity: the caller is whoever the
// claims say, and there is no field through which one could name another.
type nutritionChatRequest struct {
	Label        string                `json:"label"`
	Reasons      []string              `json:"reasons"`
	Window       foodAdviceWindow      `json:"window"`
	Signals      []nutritionChatSignal `json:"signals"`
	EligibleDays int                   `json:"eligible_days"`
	Turns        []nutritionChatTurn   `json:"turns"`
	Question     string                `json:"question"`
}

type nutritionChatResponse struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Answer    string `json:"answer,omitempty"`
}

func writeNutritionChatUnavailable(w http.ResponseWriter, reason string) {
	writeJSON(w, nutritionChatResponse{Available: false, Reason: reason})
}

// PostFoodAdviceChat answers one question about the nutrition advice the caller
// is looking at, from the same evidence that advice was built on. It is
// stateless: the conversation lives in the caller's tab, is replayed here on
// every request, and is never written down.
func (h *foodHandlers) PostFoodAdviceChat(w http.ResponseWriter, r *http.Request) {
	if !isSameOriginRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req nutritionChatRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, nutritionChatMaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// The label, reason codes and window are validated by exactly the rule the
	// advice endpoint uses, so the chat can never be asked to explain a
	// judgment the advice path would have rejected.
	adviceReq := foodAdviceRequest{Label: req.Label, Reasons: req.Reasons, Window: req.Window}
	reasons, ok := normalizeAdviceRequest(&adviceReq)
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	signals, ok := normalizeNutritionChatSignals(req.Signals)
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	turns, ok := normalizeNutritionChatTurns(req.Turns)
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" || len([]rune(question)) > nutritionChatMaxQuestionRune {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.EligibleDays < 0 || req.EligibleDays > nutritionChatWindowDays {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	settingsJSON, err := h.callerSettingsJSON(claims.UserID)
	if err != nil {
		slog.Warn("nutrition chat settings lookup failed", "err", err, "user_id", claims.UserID)
		writeNutritionChatUnavailable(w, "unavailable")
		return
	}
	now := time.Now().UTC()
	loc := database.ResolveTimezone(settingsJSON)
	language := primaryAdviceLanguage(displayLanguageFromSettings(settingsJSON))
	target, unavailableReason, err := computeNutritionTargetForProfile(
		h.storage, claims.UserID, now, loc, parseUserProfile(settingsJSON))
	if err != nil {
		slog.Warn("nutrition chat target computation failed", "err", err, "user_id", claims.UserID)
		writeNutritionChatUnavailable(w, "unavailable")
		return
	}
	if unavailableReason != "" {
		writeNutritionChatUnavailable(w, "unavailable")
		return
	}

	in := vision.NutritionChatInput{
		Label: req.Label, Reasons: reasons, Signals: signals,
		EligibleDays: req.EligibleDays, WindowDays: nutritionChatWindowDays,
		MeanCalories: *req.Window.MeanCalories, MeanProteinGrams: *req.Window.MeanProteinGrams,
		MeanCarbsGrams: *req.Window.MeanCarbsGrams, MeanFatGrams: *req.Window.MeanFatGrams,
		MeanSugarGrams: *req.Window.MeanSugarGrams, MeanSodiumGrams: *req.Window.MeanSodiumGrams,
		TargetCalories: target.Calories, TargetProteinGrams: target.ProteinGrams,
		TargetCarbsGrams: target.CarbsGrams, TargetFatGrams: target.FatGrams,
		DisplayLanguage: language,
		Turns:           turns,
		Question:        question,
	}

	tctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()
	result, err := h.vision.NutritionChat(tctx, in)
	if err != nil {
		// The model's own error text can carry provider detail and echo the
		// question back; the caller gets the same two reasons the advice
		// endpoint uses, and nothing else.
		slog.Warn("nutrition chat failed", "err", redactQuestion(err, question), "user_id", claims.UserID)
		if errors.Is(err, vision.ErrNotConfigured) {
			writeNutritionChatUnavailable(w, "unconfigured")
			return
		}
		writeNutritionChatUnavailable(w, "unavailable")
		return
	}
	writeJSON(w, nutritionChatResponse{Available: true, Answer: result.Answer})
}

// normalizeNutritionChatSignals accepts only the workings the Healthiness Label
// can actually produce. A signal the client invented, or a boundary it moved,
// would reach the model as fact and come back to the user as an explanation of
// a threshold that does not exist.
func normalizeNutritionChatSignals(in []nutritionChatSignal) ([]vision.NutritionChatSignal, bool) {
	if len(in) > nutritionChatMaxSignals {
		return nil, false
	}
	out := make([]vision.NutritionChatSignal, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, signal := range in {
		if !adviceReasonCode.MatchString(signal.Code) {
			return nil, false
		}
		if _, exists := seen[signal.Code]; exists {
			return nil, false
		}
		seen[signal.Code] = struct{}{}
		if _, valid := nutritionChatVerdicts[signal.Verdict]; !valid {
			return nil, false
		}
		if _, valid := nutritionChatUnits[signal.Unit]; !valid {
			return nil, false
		}
		if signal.Reason != "" && !adviceReasonCode.MatchString(signal.Reason) {
			return nil, false
		}
		// An `ok` signal contributed no reason code, and a flagged one must
		// carry the code it contributed — otherwise the signals and the
		// reasons on screen could disagree about what was flagged.
		if (signal.Verdict == "ok") != (signal.Reason == "") {
			return nil, false
		}
		for _, figure := range [...]*float64{signal.Value, signal.OffBoundary, signal.FarBoundary} {
			if figure == nil || math.IsNaN(*figure) || math.IsInf(*figure, 0) ||
				*figure < 0 || *figure > 100000 {
				return nil, false
			}
		}
		out = append(out, vision.NutritionChatSignal{
			Code: signal.Code, Value: *signal.Value, Unit: signal.Unit,
			Verdict: signal.Verdict, Reason: signal.Reason,
			OffBoundary: *signal.OffBoundary, FarBoundary: *signal.FarBoundary,
		})
	}
	return out, true
}

func normalizeNutritionChatTurns(in []nutritionChatTurn) ([]vision.NutritionChatTurn, bool) {
	if len(in) > nutritionChatMaxTurns {
		return nil, false
	}
	out := make([]vision.NutritionChatTurn, 0, len(in))
	for _, turn := range in {
		limit := nutritionChatMaxQuestionRune
		switch turn.Role {
		case "user":
		case "assistant":
			limit = nutritionChatMaxAnswerRune
		default:
			return nil, false
		}
		text := strings.TrimSpace(turn.Text)
		if text == "" || len([]rune(text)) > limit {
			return nil, false
		}
		out = append(out, vision.NutritionChatTurn{Role: turn.Role, Text: text})
	}
	return out, true
}

// redactQuestion keeps a model error useful in the log without writing the
// user's own words into it. A provider that fails mid-request can quote the
// prompt back in its error, and the prompt carries a medical question; the
// spec's storage boundary says no raw prompt log, and an application log is
// one. The text is bounded as well, because a provider error can also carry a
// whole response body.
func redactQuestion(err error, question string) string {
	text := err.Error()
	if question != "" {
		text = strings.ReplaceAll(text, question, "<question>")
	}
	if runes := []rune(text); len(runes) > nutritionChatMaxLoggedErrorRune {
		text = string(runes[:nutritionChatMaxLoggedErrorRune]) + "…"
	}
	return text
}
