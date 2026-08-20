package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

const (
	// maxReanalyzeBodyBytes bounds the request body read before decoding, so
	// an oversized payload is rejected before any JSON parsing work.
	maxReanalyzeBodyBytes          = 4 * 1024
	maxExpertComponents            = 20
	maxComponentNameLength         = 100
	maxCombinedComponentNameLength = 500
)

// reanalyzeEligibleStatuses are the meal statuses Reanalyze can be called
// against. processing and pending_clarification are excluded: the former has
// a call already running (or is a stale remnant Retry already handles), the
// latter has its own clarify flow.
var reanalyzeEligibleStatuses = []string{
	database.MealStatusFailed, database.MealStatusPendingReview, database.MealStatusConfirmed,
}

type expertComponentRequest struct {
	Name        string   `json:"name"`
	WeightGrams *float64 `json:"weight_grams,omitempty"`
}

// UnmarshalJSON keeps an omitted optional weight distinct from an explicit
// null. Omitted means "estimate this weight"; null is not a positive finite
// number and must be rejected before the meal is claimed or vision is called.
func (c *expertComponentRequest) UnmarshalJSON(data []byte) error {
	var wire struct {
		Name        string          `json:"name"`
		WeightGrams json.RawMessage `json:"weight_grams"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	c.Name = wire.Name
	c.WeightGrams = nil
	if len(wire.WeightGrams) == 0 {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(wire.WeightGrams), []byte("null")) {
		return errors.New("weight_grams must be a number when present")
	}
	var weight float64
	if err := json.Unmarshal(wire.WeightGrams, &weight); err != nil {
		return err
	}
	c.WeightGrams = &weight
	return nil
}

type reanalyzeInput struct {
	hint       *string
	components []expertComponentRequest
}

func parseReanalyzeInput(body []byte) (reanalyzeInput, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return reanalyzeInput{}, errors.New("bad request")
	}
	hintRaw, hasHint := object["hint"]
	componentsRaw, hasComponents := object["components"]
	if hasHint == hasComponents {
		return reanalyzeInput{}, errors.New("provide exactly one of hint or components")
	}
	if hasHint {
		var rawHint string
		if string(hintRaw) == "null" || json.Unmarshal(hintRaw, &rawHint) != nil {
			return reanalyzeInput{}, errors.New("hint must be a string")
		}
		hint, err := normalizeHint(rawHint)
		if err != nil {
			return reanalyzeInput{}, err
		}
		if hint == "" {
			return reanalyzeInput{}, errors.New("hint is required")
		}
		return reanalyzeInput{hint: &hint}, nil
	}

	var components []expertComponentRequest
	if string(componentsRaw) == "null" || json.Unmarshal(componentsRaw, &components) != nil {
		return reanalyzeInput{}, errors.New("components must be an array")
	}
	if len(components) < 1 || len(components) > maxExpertComponents {
		return reanalyzeInput{}, fmt.Errorf("components must contain between 1 and %d items", maxExpertComponents)
	}
	combinedLength := 0
	for i := range components {
		components[i].Name = strings.TrimSpace(components[i].Name)
		nameLength := utf8.RuneCountInString(components[i].Name)
		if nameLength == 0 {
			return reanalyzeInput{}, fmt.Errorf("component %d name is required", i+1)
		}
		if nameLength > maxComponentNameLength {
			return reanalyzeInput{}, fmt.Errorf("component names must be at most %d characters", maxComponentNameLength)
		}
		combinedLength += nameLength
		if components[i].WeightGrams != nil && (!isPositiveFinite(*components[i].WeightGrams)) {
			return reanalyzeInput{}, fmt.Errorf("component %d weight_grams must be a positive finite number", i+1)
		}
	}
	if combinedLength > maxCombinedComponentNameLength {
		return reanalyzeInput{}, fmt.Errorf("combined component names must be at most %d characters", maxCombinedComponentNameLength)
	}
	return reanalyzeInput{components: components}, nil
}

func isPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// Reanalyze handles POST /api/food/meals/{id}/reanalyze: either re-runs
// recognition with a required hint or rebuilds the meal from expert-supplied
// components, replacing its items. Eligible from failed, pending_review, or confirmed —
// unlike Retry, which only recovers from failed/stale-processing and takes
// no hint. See design.md "Reanalyze claims the meal atomically" and
// "Reanalyze failure reverts to the meal's prior state" for the rationale.
func (h *foodHandlers) Reanalyze(w http.ResponseWriter, r *http.Request) {
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

	var meal database.FoodMeal
	err = h.storage.DB().Where("id = ? AND user_id = ?", id, claims.UserID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	// Read the whole bounded body before decoding, rather than handing the
	// MaxBytesReader straight to json.Decoder: Decode stops as soon as it has
	// parsed one complete JSON value and never reads further, so trailing
	// bytes past a small, valid JSON prefix (e.g. a real hint followed by
	// thousands of padding bytes) would never actually reach the
	// MaxBytesReader's limit check, silently defeating the 4 KiB cap this
	// requirement promises. io.ReadAll reads to EOF, so oversized trailing
	// content is guaranteed to trip the limit here.
	r.Body = http.MaxBytesReader(w, r.Body, maxReanalyzeBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	input, err := parseReanalyzeInput(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if meal.PhotoPath == "" {
		http.Error(w, "meal has no photo to reanalyze", http.StatusConflict)
		return
	}

	// Captured before the atomic claim below overwrites them, for use on
	// failure — see revertReanalyze.
	priorStatus := meal.Status
	priorClarifyRound := meal.ClarifyRound
	priorClarifyLog := meal.ClarifyLog
	priorUpdatedAt := meal.UpdatedAt

	eligible := false
	for _, s := range reanalyzeEligibleStatuses {
		if priorStatus == s {
			eligible = true
			break
		}
	}
	if !eligible {
		http.Error(w, "meal is not eligible for reanalysis", http.StatusConflict)
		return
	}

	// Atomic claim on the *exact* (status, updated_at) just observed — an
	// optimistic-concurrency check, not just a status match. Two hazards
	// this closes:
	//
	//  1. Status alone ("status IN eligible") isn't enough: ConfirmMeal
	//     transitions pending_review -> confirmed, and Reanalyze is
	//     eligible from both. If ConfirmMeal committed that transition in
	//     the gap between this handler's read above and this claim, a
	//     status-only claim would still match (confirmed is also
	//     eligible), silently claim the meal, and on failure revert to the
	//     *stale* pending_review captured before ConfirmMeal ran —
	//     discarding a legitimate concurrent confirm.
	//  2. RetryMeal treats `processing` as retryable once `updated_at` is
	//     older than the vision timeout — the same threshold this
	//     handler's own vision call runs against. If this attempt's call
	//     is slow enough to cross that threshold right as it fails, a
	//     concurrent RetryMeal can legitimately claim the same meal as
	//     "stale processing" before this handler's revert runs.
	//     `updated_at` doubles as this attempt's lease token: the claim
	//     below writes a fresh one, and every later write for *this*
	//     attempt (persistAnalysis, failMeal, revertReanalyze) is
	//     conditioned on that exact value — so a newer claim (which writes
	//     its own fresh updated_at) silently invalidates this attempt's
	//     right to write anything further, rather than one attempt
	//     clobbering the other's in-flight or completed work.
	lease := time.Now().UTC()
	claim := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status = ? AND updated_at = ?", meal.ID, priorStatus, priorUpdatedAt).
		Updates(map[string]any{
			"status":        database.MealStatusProcessing,
			"clarify_round": 0,
			"clarify_log":   "",
			"updated_at":    lease,
		})
	if claim.Error != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	if claim.RowsAffected == 0 {
		http.Error(w, "meal is not eligible for reanalysis", http.StatusConflict)
		return
	}
	meal.Status = database.MealStatusProcessing
	meal.ClarifyRound = 0
	meal.ClarifyLog = ""
	meal.UpdatedAt = lease

	ctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()

	// strict=true: unlike upload/retry/clarify, a Select failure here must
	// not be swallowed into a silently-unresolved item set — this can
	// replace a confirmed meal's real, reviewed items, and the whole point
	// of Reanalyze's contract is that a failure leaves the meal untouched.
	// See resolveItems's doc comment.
	var analysisErr error
	if input.hint != nil {
		analysisErr = h.runAnalysis(ctx, &meal, *input.hint, lease, true)
	} else {
		analysisErr = h.runExpertAnalysis(ctx, &meal, input.components, lease)
	}
	if analysisErr != nil {
		// Non-destructive failure: restore exactly what the claim overwrote.
		// Items and aggregate were never touched — persistAnalysis's
		// delete-and-replace only runs on a successful recognition — so the
		// meal is left exactly as it was found, PROVIDED this attempt's
		// lease is still current (see revertReanalyze). Responding with 502
		// (not 200 with a mutated meal) lets the caller distinguish
		// "reanalysis failed, nothing changed" from a normal state
		// transition.
		revert := h.revertReanalyze(meal.ID, lease, priorStatus, priorClarifyRound, priorClarifyLog)
		if revert.err != nil {
			// The revert itself failed: the meal is genuinely stuck in
			// processing, not "unchanged" — say so distinctly rather than
			// claiming a guarantee that no longer holds. RetryMeal's stale-
			// processing recovery remains available once the vision timeout
			// elapses.
			http.Error(w, "reanalysis failed and the meal could not be restored to its prior state", http.StatusInternalServerError)
			return
		}
		if !revert.applied {
			// The lease was already gone: a newer attempt (e.g. a
			// stale-processing RetryMeal, once this call ran long enough to
			// cross the same vision timeout) has since claimed this meal.
			// That attempt owns the row now — reverting would stomp on
			// whatever it's doing. This request's own attempt still
			// failed, but the meal is NOT guaranteed unchanged — the newer
			// attempt may already have altered it, and this handler has no
			// way to know its outcome. HTTP 412 (Precondition Failed) — the
			// standard code for "your conditional update's precondition, the
			// lease, no longer holds" — signals that distinctly from both
			// the plain "not eligible" 409 (nothing was ever attempted) and
			// the "vision failed, revert succeeded" 502 (genuinely
			// unchanged): the caller should refetch rather than assume
			// nothing changed.
			http.Error(w, "reanalysis failed; the meal was claimed by another operation in the meantime", http.StatusPreconditionFailed)
			return
		}
		http.Error(w, "reanalysis failed; the meal is unchanged", http.StatusBadGateway)
		return
	}

	writeJSON(w, &meal)
}

// runExpertAnalysis trusts names and supplied weights as user input. It asks
// the model only for omitted weights, keyed by the original component index,
// then uses the same candidate-resolution path as ordinary recognition.
// Expert mode deliberately cannot enter clarification.
func (h *foodHandlers) runExpertAnalysis(
	ctx context.Context, meal *database.FoodMeal, components []expertComponentRequest, lease time.Time,
) error {
	items := make([]vision.Item, len(components))
	missing := make([]vision.WeightEstimateInput, 0, len(components))
	for i, component := range components {
		items[i] = vision.Item{Name: component.Name, Confidence: 1}
		if component.WeightGrams == nil {
			missing = append(missing, vision.WeightEstimateInput{ComponentIndex: i, Name: component.Name})
		} else {
			items[i].WeightGrams = *component.WeightGrams
		}
	}

	raw := ""
	if len(missing) > 0 {
		photoBytes, err := h.photos.Read(meal.PhotoPath)
		if err != nil {
			return err
		}
		result, err := h.vision.EstimateWeights(ctx, photoBytes, mimeTypeForExt(extOf(meal.PhotoPath)), missing)
		if err != nil {
			return err
		}
		if result == nil {
			return errors.New("weight estimator returned no result")
		}
		expected := make(map[int]struct{}, len(missing))
		for _, component := range missing {
			expected[component.ComponentIndex] = struct{}{}
		}
		seen := make(map[int]struct{}, len(result.Estimates))
		for _, estimate := range result.Estimates {
			if _, ok := expected[estimate.ComponentIndex]; !ok {
				return fmt.Errorf("weight estimator returned unexpected component_index %d", estimate.ComponentIndex)
			}
			if _, duplicate := seen[estimate.ComponentIndex]; duplicate {
				return fmt.Errorf("weight estimator returned duplicate component_index %d", estimate.ComponentIndex)
			}
			if !isPositiveFinite(estimate.WeightGrams) {
				return fmt.Errorf("weight estimator returned invalid weight for component_index %d", estimate.ComponentIndex)
			}
			seen[estimate.ComponentIndex] = struct{}{}
			items[estimate.ComponentIndex].WeightGrams = estimate.WeightGrams
		}
		if len(seen) != len(expected) {
			return errors.New("weight estimator omitted a component")
		}
		raw = result.Raw
	}

	displayLanguage := DisplayLanguage(h.storage, meal.UserID)
	resolved, err := h.resolveItems(ctx, meal, items, true, displayLanguage)
	if err != nil {
		return err
	}
	return h.persistAnalysis(meal, database.MealStatusPendingReview, raw, resolved, nil, lease)
}

// revertResult reports whether a revert/persist write conditioned on a lease
// token actually applied (revertReanalyze, persistAnalysis, failMeal all
// follow this pattern): applied is false when the lease no longer matches —
// i.e. a newer analysis attempt has since claimed the row — which is a
// normal, expected outcome, not an error.
type revertResult struct {
	applied bool
	err     error
}

// revertReanalyze restores a meal's status/clarify fields to their pre-claim
// values after a failed reanalysis attempt, conditioned on this attempt's
// lease (the updated_at its own claim wrote) still being current. If a newer
// attempt (see the claim's doc comment above) has since claimed the row,
// updated_at no longer matches and this is a no-op — the newer attempt owns
// the row now, and stomping its in-flight or completed work would be worse
// than leaving this attempt's revert undone.
func (h *foodHandlers) revertReanalyze(
	mealID uuid.UUID, lease time.Time, status string, clarifyRound int, clarifyLog string,
) revertResult {
	res := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND updated_at = ?", mealID, lease).
		Updates(map[string]any{
			"status":        status,
			"clarify_round": clarifyRound,
			"clarify_log":   clarifyLog,
		})
	return revertResult{applied: res.RowsAffected > 0, err: res.Error}
}
