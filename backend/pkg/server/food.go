package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/off"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"github.com/ya-breeze/healthvault/pkg/usda"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

// foodHandlers bundles the dependencies shared by the food logging HTTP
// handlers: /api/food/* and the photo-backed record types.
type foodHandlers struct {
	storage database.Storage
	usda    *usda.Index // nil if no import has run yet
	off     *off.Index  // nil if no import has run yet
	photos  *photostorage.Store

	vision         vision.Client
	maxUploadBytes int64
	visionTimeout  time.Duration
}

// defaultMaxUploadBytes and defaultVisionTimeout mirror config.go's own
// viper defaults, so a foodHandlers built without WithVision (any test that
// doesn't exercise CreateMeal) still fails safe rather than rejecting every
// upload as oversized against a zero-value limit.
const (
	defaultMaxUploadBytes = 10 * 1024 * 1024
	defaultVisionTimeout  = 60 * time.Second
)

// NewFoodHandlers builds the food logging handler bundle. usdaIndex may be
// nil if no import has run yet; the handlers degrade rather than fail.
// vision defaults to vision.Unconfigured{} — call WithVision to set a real
// client and non-default upload limits; tests that don't exercise the photo
// upload/analysis pipeline can skip that call entirely.
func NewFoodHandlers(storage database.Storage, usdaIndex *usda.Index, uploadsDir string) *foodHandlers {
	return &foodHandlers{
		storage:        storage,
		usda:           usdaIndex,
		photos:         photostorage.New(uploadsDir),
		vision:         vision.Unconfigured{},
		maxUploadBytes: defaultMaxUploadBytes,
		visionTimeout:  defaultVisionTimeout,
	}
}

// WithVision sets the vision client and upload limits used by CreateMeal.
func (h *foodHandlers) WithVision(client vision.Client, maxUploadBytes int64, visionTimeout time.Duration) *foodHandlers {
	h.vision = client
	h.maxUploadBytes = maxUploadBytes
	h.visionTimeout = visionTimeout
	return h
}

// WithOFF sets the Open Food Facts index used by candidate resolution.
// offIndex may be nil if no import has run yet; handlers degrade to
// USDA-only rather than fail. A separate builder method, not a
// NewFoodHandlers parameter, so the many existing call sites that don't
// exercise OFF-specific behavior don't all need updating.
func (h *foodHandlers) WithOFF(offIndex *off.Index) *foodHandlers {
	h.off = offIndex
	return h
}

// FoodSearchResult is one candidate returned by GET /api/food/search: either
// the user's own custom food or a USDA reference food.
type FoodSearchResult struct {
	Source       string                   `json:"source"` // "custom" or "usda"
	CustomFoodID *uuid.UUID               `json:"custom_food_id,omitempty"`
	FdcID        *int64                   `json:"fdc_id,omitempty"`
	Name         string                   `json:"name"`
	Profile      database.NutrientProfile `json:"profile"`
}

// FoodSearchResponse is the body of GET /api/food/search.
type FoodSearchResponse struct {
	Results []FoodSearchResult `json:"results"`
	// USDAUnavailable is set when no USDA import has run yet, so an empty
	// Results is distinguishable from "nothing matched".
	USDAUnavailable bool `json:"usda_unavailable,omitempty"`
}

// Search handles GET /api/food/search?q=&preparation=&state=. A custom food
// whose name exactly matches (case-insensitive) wins outright and is returned
// alone; otherwise the query falls through to the USDA candidate shortlist.
func (h *foodHandlers) Search(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("q"))
	if name == "" {
		writeJSON(w, FoodSearchResponse{Results: []FoodSearchResult{}})
		return
	}

	var custom database.CustomFood
	err := h.storage.DB().
		Where("user_id = ? AND LOWER(name) = LOWER(?)", claims.UserID, name).
		First(&custom).Error
	switch {
	case err == nil:
		id := custom.ID
		writeJSON(w, FoodSearchResponse{Results: []FoodSearchResult{{
			Source:       "custom",
			CustomFoodID: &id,
			Name:         custom.Name,
			Profile:      custom.Profile(),
		}}})
		return
	case errors.Is(err, gorm.ErrRecordNotFound):
		// No exact custom match; fall through to USDA search.
	default:
		http.Error(w, "search error", http.StatusInternalServerError)
		return
	}

	if h.usda == nil {
		writeJSON(w, FoodSearchResponse{Results: []FoodSearchResult{}, USDAUnavailable: true})
		return
	}

	term := usda.QueryFor(name, r.URL.Query().Get("preparation"), r.URL.Query().Get("state"))
	foods, err := h.usda.Search(term, usda.DefaultCandidates)
	if err != nil {
		if errors.Is(err, usda.ErrNoDatabase) {
			writeJSON(w, FoodSearchResponse{Results: []FoodSearchResult{}, USDAUnavailable: true})
			return
		}
		http.Error(w, "search error", http.StatusInternalServerError)
		return
	}

	results := make([]FoodSearchResult, 0, len(foods))
	for _, f := range foods {
		fdcID := f.FdcID
		results = append(results, FoodSearchResult{
			Source:  "usda",
			FdcID:   &fdcID,
			Name:    f.Description,
			Profile: f.Profile,
		})
	}
	writeJSON(w, FoodSearchResponse{Results: results})
}

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONStatus(w, http.StatusOK, v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
