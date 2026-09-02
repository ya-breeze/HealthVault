package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

const (
	// maxDescribeBodyBytes bounds the request body read before decoding —
	// see Reanalyze's identical reasoning for why io.ReadAll, not a bare
	// json.Decoder, is required to make this cap actually bite: Decode stops
	// at the first complete JSON value and never reads the padding trailing
	// it, so a small valid prefix followed by megabytes of garbage would
	// never trip a MaxBytesReader limit checked only by Decode itself.
	maxDescribeBodyBytes = 8 * 1024
	// maxDescriptionLength bounds description in runes, not bytes: a
	// non-Latin script (e.g. Cyrillic) can describe the same meal in fewer
	// bytes per rune than English, so a byte cap would silently limit
	// non-English users to shorter descriptions for no reason connected to
	// the limit's actual purpose (bounding prompt size and abuse).
	maxDescriptionLength = 1000
	// describeNameLength bounds the meal Name derived from the description
	// when the caller omits name — long enough to be legible in a history
	// list, short enough not to dominate it.
	describeNameLength = 60
)

// describeMealRequest is the body of POST /api/food/meals/describe. name and
// logged_at are optional: name defaults to the description itself
// (truncated), logged_at to now.
type describeMealRequest struct {
	Description string     `json:"description"`
	Name        string     `json:"name,omitempty"`
	LoggedAt    *time.Time `json:"logged_at,omitempty"`
}

// CreateDescribedMeal handles POST /api/food/meals/describe: text-only
// manual entry that recognizes foods from the user's own written account of
// a meal, rather than a photo or a structured item-by-item list.
// Description-first, the same way CreateMeal (food_upload.go) is
// photo-first: the FoodMeal row is committed as processing, with the
// description stored and no PhotoPath, before the model call runs, so no
// outcome of that call — success, failure, or timeout — can lose what the
// user typed.
func (h *foodHandlers) CreateDescribedMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxDescribeBodyBytes)
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

	var req describeMealRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(req.Description)
	descLength := utf8.RuneCountInString(description)
	if descLength == 0 {
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	}
	if descLength > maxDescriptionLength {
		http.Error(w, fmt.Sprintf("description must be at most %d characters", maxDescriptionLength), http.StatusBadRequest)
		return
	}

	loggedAt := time.Now().UTC()
	if req.LoggedAt != nil {
		// UTC-normalize before storing — see PatchMeal/ConfirmMeal/
		// CreateManualMeal's identical treatment for why an un-normalized
		// offset breaks history ordering and its keyset cursor.
		loggedAt = req.LoggedAt.UTC()
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = truncateWithEllipsis(description, describeNameLength)
	}

	familyID := FamilyIDFromCtx(r)
	meal := database.FoodMeal{
		UserID:      claims.UserID,
		Status:      database.MealStatusProcessing,
		LoggedAt:    loggedAt,
		Name:        name,
		Description: description,
	}
	meal.ID = uuid.New()
	meal.FamilyID = familyID
	if err := h.storage.DB().Create(&meal).Error; err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}

	// meal.UpdatedAt, set by GORM's Create above, is this attempt's lease
	// token from the start — see analyzeMeal's doc comment (food_upload.go).
	applied, failErr := h.analyzeDescribedMeal(r.Context(), &meal, meal.UpdatedAt)
	if failErr != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	result, err := h.reloadIfSuperseded(&meal, applied)
	writeReloadedMeal(w, result, err, http.StatusCreated)
}

// truncateWithEllipsis returns s unchanged if it has at most n runes,
// otherwise its first n runes followed by a trailing ellipsis. Rune-based,
// not byte-based, so a multi-byte script is never cut mid-character.
func truncateWithEllipsis(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// runDescribeAnalysis calls vision.Describe with the meal's stored
// description and the user's real Display Language — unlike
// runExpertAnalysis's forced "en", passing the actual Display Language here
// is the substance of this change: it lets a non-English Display Name come
// back correctly and lets resolveItems apply its non-English gate
// (retrieveCandidates skips USDA/Open Food Facts, matching the photo path).
// The result is handed to the existing processRecognition, so the clarify
// loop, item resolution, and persistence are shared unmodified with
// Recognize/Clarify.
func (h *foodHandlers) runDescribeAnalysis(ctx context.Context, meal *database.FoodMeal, lease time.Time) error {
	displayLanguage := DisplayLanguage(h.storage, meal.UserID)
	recognized, err := h.vision.Describe(ctx, meal.Description, displayLanguage)
	if err != nil {
		return err
	}
	// strict=false, mirroring analyzeMeal (upload/retry/clarify): this path
	// has no prior reviewed content to lose, so a Select hiccup should
	// degrade every candidate item to its own estimate rather than fail the
	// whole analysis — see resolveItems's doc comment on strict.
	return h.processRecognition(ctx, meal, recognized, lease, false, displayLanguage)
}

// analyzeDescribedMeal mirrors analyzeMeal (food_upload.go) for the
// description-only path: same vision-timeout budget, same failMeal fallback
// on any error (including a timeout), so no analysis outcome can leave the
// meal stuck in processing. See analyzeMeal's doc comment for the
// (applied, err) contract this shares.
func (h *foodHandlers) analyzeDescribedMeal(ctx context.Context, meal *database.FoodMeal, lease time.Time) (applied bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, h.visionTimeout)
	defer cancel()
	if err := h.runDescribeAnalysis(ctx, meal, lease); err != nil {
		return h.failMeal(meal, lease)
	}
	return true, nil
}
