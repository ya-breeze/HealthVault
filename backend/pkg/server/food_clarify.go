package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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

	// Claim the round atomically: the WHERE clause repeats the status check
	// above plus an updated_at match against what was just loaded, as part
	// of the same conditional UPDATE rather than a prior read, so two
	// requests racing this endpoint for the same round can't both pass it.
	// Only the winner proceeds to call vision.Clarify; the loser's
	// RowsAffected is 0. Without this, two near-simultaneous submissions (a
	// double-click, two tabs) can both read pending_clarification, both
	// bill a real vision call, and race each other's writes with no
	// defined winner. Moving to processing here also reuses RetryMeal's
	// existing staleness recovery if this call hangs or the process
	// crashes mid-call — the fresh updated_at written here doubles as this
	// attempt's lease token, threaded to processRecognition/failMeal below
	// so a newer attempt claiming this meal (e.g. that same stale-processing
	// recovery, racing this one) can't have its work overwritten by this
	// one arriving late — see analyzeMeal's doc comment in food_upload.go.
	priorUpdatedAt := meal.UpdatedAt
	lease := time.Now().UTC()
	claim := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status = ? AND updated_at = ?", meal.ID, database.MealStatusPendingClarification, priorUpdatedAt).
		Updates(map[string]any{
			"clarify_round": meal.ClarifyRound,
			"clarify_log":   meal.ClarifyLog,
			"status":        database.MealStatusProcessing,
			"updated_at":    lease,
		})
	if claim.Error != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	if claim.RowsAffected == 0 {
		http.Error(w, "meal is not awaiting clarification", http.StatusConflict)
		return
	}
	meal.Status = database.MealStatusProcessing
	meal.UpdatedAt = lease

	history := make([]vision.ClarifyTurn, len(entries))
	for i, e := range entries {
		history[i] = vision.ClarifyTurn{Question: e.Question, Answer: e.Answer}
	}
	priorItems := make([]vision.Item, len(meal.Items))
	for i, it := range meal.Items {
		priorItems[i] = vision.Item{
			Name: it.Name, CanonicalName: it.CanonicalName, Preparation: it.Preparation, State: it.State, Brand: it.Brand,
			WeightGrams: it.WeightGrams, Confidence: it.Confidence,
			EstimatedProfile: estimatedProfilePtr(it),
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()

	displayLanguage := DisplayLanguage(h.storage, meal.UserID)
	recognized, err := h.vision.Clarify(ctx, meal.Description, priorItems, history, displayLanguage)
	if err == nil {
		carryForwardPriorFields(priorItems, recognized.Items, displayLanguage)
	}
	if err != nil {
		applied, failErr := h.failMeal(meal, lease)
		if failErr != nil {
			http.Error(w, "update error", http.StatusInternalServerError)
			return
		}
		result, reloadErr := h.reloadIfSuperseded(meal, applied)
		writeReloadedMeal(w, result, reloadErr, http.StatusOK)
		return
	}
	applied := true
	if err := h.processRecognition(ctx, meal, recognized, lease, false, displayLanguage); err != nil {
		var failErr error
		applied, failErr = h.failMeal(meal, lease)
		if failErr != nil {
			http.Error(w, "update error", http.StatusInternalServerError)
			return
		}
	}
	result, reloadErr := h.reloadIfSuperseded(meal, applied)
	writeReloadedMeal(w, result, reloadErr, http.StatusOK)
}

// carryForwardPriorFields overwrites every recognized item's EstimatedProfile
// with the corresponding prior item's persisted estimate, positionally,
// discarding whatever (if anything) Clarify itself returned for
// estimated_profile first. Clarify shares Recognize's prompt/schema even
// though it is text-only, so a response can still carry a non-null
// estimated_profile — a guess made without the photo, which must never be
// trusted over the original photo-grounded one; processRecognition treats a
// nil EstimatedProfile as "no estimate" when persisting the resulting
// FoodItem rows. A prior item's estimate is only carried forward when the
// item count is unchanged AND the name at that position is still plausibly
// the same item (see namesLikelySameItem): a round that added, removed, or
// reordered items has no reliable position to carry an estimate forward to,
// and a same-count reorder is otherwise indistinguishable from an unchanged
// list by position alone — carrying an estimate to a same-index-but-different
// item would silently misattribute it (e.g. swap "grilled chicken" and
// "mixed salad"). This positional/name heuristic is not a substitute for a
// genuine per-item identity echoed back by the model — a same-count reorder
// between two items whose names happen to share a substring (e.g. "Rice" and
// "Fried rice") can still misattribute; that residual gap needs the model to
// echo back an explicit prior-item index, which is a larger schema/prompt
// change tracked separately rather than solved here.
//
// Under the same identity guard it also restores a Canonical Name the clarify
// response dropped. The schema marks canonical_name required, so the model
// must emit the key — but nothing makes it emit a non-empty value, and an
// empty one is stored as-is by processRecognition, permanently losing the
// item's English identity: canonical_name is deliberately not editable via
// PATCH, so a Russian user would be left with one item in a meal whose Expert
// Mode shows no gloss and no way to restore it. That is the same defensiveness
// the estimate carry-forward exists to provide, keyed on the same
// already-established item identity. Found in code review.
//
// The restore is skipped entirely for an English Display Language, where a
// Canonical Name must stay empty rather than duplicate the Display Name (see
// vision.toRecognizeResult and food-photo-recognition "Recognition in English
// Display Language does not duplicate the name"). It is unreachable in a
// wholly English meal anyway — prior items have no Canonical Name to carry —
// but a user who switches Display Language to English between rounds does have
// one stored, and carrying it forward there would write exactly the duplicate
// that requirement forbids.
func carryForwardPriorFields(priorItems, recognizedItems []vision.Item, displayLanguage string) {
	for i := range recognizedItems {
		recognizedItems[i].EstimatedProfile = nil
	}
	if len(priorItems) != len(recognizedItems) {
		return
	}
	keepCanonicalNames := !vision.IsEnglishDisplayLanguage(displayLanguage)
	for i := range recognizedItems {
		if !namesLikelySameItem(priorItems[i].Name, recognizedItems[i].Name) {
			continue
		}
		recognizedItems[i].EstimatedProfile = priorItems[i].EstimatedProfile
		if keepCanonicalNames && recognizedItems[i].CanonicalName == "" {
			recognizedItems[i].CanonicalName = priorItems[i].CanonicalName
		}
	}
}

// namesLikelySameItem reports whether a recognized item's name is plausibly
// still the same item as a prior round's, tolerating the ordinary case where
// a clarification round narrows a vague name into a more specific one (e.g.
// "Sauce" -> "Tomato sauce", still the same single item) while still
// rejecting a same-count reorder that swapped two genuinely unrelated items
// (e.g. "grilled chicken" and "mixed salad" share no substring either way).
func namesLikelySameItem(prior, recognized string) bool {
	p := strings.ToLower(strings.TrimSpace(prior))
	r := strings.ToLower(strings.TrimSpace(recognized))
	if p == "" || r == "" {
		return false
	}
	return p == r || strings.Contains(r, p) || strings.Contains(p, r)
}

// estimatedProfilePtr returns the item's persisted estimate as a pointer, or nil
// if none is present or usable. The clarify round's follow-up call is
// text-only and cannot regenerate a photo-grounded estimate, so the original
// one — persisted on the row when the item was first created, see
// database.FoodItem.SetEstimatedProfile — is fed back to the model as
// context instead, the same way Name/Preparation/State/Brand/WeightGrams
// already are, so it can carry forward into whatever items the clarify
// response produces. See
// openspec/changes/composite-food-recognition/design.md decision 4.
func estimatedProfilePtr(it database.FoodItem) *database.NutrientProfile {
	p, ok := it.EstimatedProfile()
	if !ok {
		return nil
	}
	return &p
}
