package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

type clarifyMealRequest struct {
	Answers []string `json:"answers"`
}

// ClarifyMeal handles POST /api/food/meals/{id}/clarify: submits answers to
// the current round's pending questions (stored, unanswered, in
// clarify_log), replays the full question/answer history to the model —
// text-only, the photo is never re-sent — and either asks another round or
// proceeds to candidate matching. Capped at database.MaxClarifyRounds
// rounds, after which the meal moves to pending_review regardless.
func (h *foodHandlers) ClarifyMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	meal, err := h.loadOwnedMeal(id, claims.UserID)
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if meal.Status != database.MealStatusPendingClarification {
		http.Error(w, "meal is not awaiting clarification", http.StatusConflict)
		return
	}

	var entries []database.ClarifyEntry
	if meal.ClarifyLog != "" {
		if err := json.Unmarshal([]byte(meal.ClarifyLog), &entries); err != nil {
			http.Error(w, "corrupt clarify log", http.StatusInternalServerError)
			return
		}
	}
	pendingRound := meal.ClarifyRound + 1
	var pendingIdx []int
	for i, e := range entries {
		if e.Round == pendingRound && e.Answer == "" {
			pendingIdx = append(pendingIdx, i)
		}
	}
	if len(pendingIdx) == 0 {
		http.Error(w, "no pending clarification questions", http.StatusConflict)
		return
	}

	var req clarifyMealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.Answers) != len(pendingIdx) {
		http.Error(w, "answers must match the number of pending questions", http.StatusBadRequest)
		return
	}
	for i, idx := range pendingIdx {
		entries[idx].Answer = req.Answers[i]
	}

	logBytes, err := json.Marshal(entries)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	meal.ClarifyLog = string(logBytes)
	meal.ClarifyRound = pendingRound
	if err := h.storage.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Updates(map[string]any{"clarify_round": meal.ClarifyRound, "clarify_log": meal.ClarifyLog}).Error; err != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}

	history := make([]vision.ClarifyTurn, len(entries))
	for i, e := range entries {
		history[i] = vision.ClarifyTurn{Question: e.Question, Answer: e.Answer}
	}
	priorItems := make([]vision.Item, len(meal.Items))
	for i, it := range meal.Items {
		priorItems[i] = vision.Item{
			Name: it.Name, Preparation: it.Preparation, State: it.State,
			WeightGrams: it.WeightGrams, Confidence: it.Confidence,
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()

	recognized, err := h.vision.Clarify(ctx, priorItems, history)
	if err != nil {
		h.failMeal(meal)
		writeJSON(w, meal)
		return
	}
	h.processRecognition(ctx, meal, recognized)
	writeJSON(w, meal)
}
