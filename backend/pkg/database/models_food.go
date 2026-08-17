package database

import (
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/models"
)

// Food logging tables are user-authored, unlike the health metric tables in
// models.go which are ingested telemetry. They deliberately carry no
// SourcePayloadID: there is no upstream webhook or import to trace them to.
// See openspec/specs/data-model "Food logging tables".

// FoodMeal statuses.
const (
	MealStatusProcessing           = "processing"
	MealStatusPendingClarification = "pending_clarification"
	MealStatusPendingReview        = "pending_review"
	MealStatusConfirmed            = "confirmed"
	MealStatusFailed               = "failed"
)

// FoodItem macro sources. A plain matched/unmatched boolean would conflate
// "bound to a reference food" with "has usable macros"; a manually entered
// item is exactly the case where those diverge, and treating it as unmatched
// would drop it from the meal aggregate.
const (
	MacroSourceReference = "reference" // bound to an FDC or custom food, scaled by weight
	MacroSourceManual    = "manual"    // values supplied directly by the user
	MacroSourceEstimated = "estimated" // no reference/custom-food match; scaled from Recognize's own photo-derived estimate
	MacroSourceNone      = "none"      // unresolved; zeroed, excluded from aggregates
)

// MaxClarifyRounds bounds the clarification loop. Each round costs a model call.
const MaxClarifyRounds = 3

// FoodMeal is one logged meal. Photo-first: the row is committed with status
// processing before the vision call runs, so no analysis outcome can lose the photo.
type FoodMeal struct {
	models.TenantModel
	UserID       uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PhotoPath    string    `gorm:"type:text" json:"photo_path,omitempty"` // relative to the uploads dir; empty for manual entries
	Status       string    `gorm:"type:varchar(32);not null;default:'processing'" json:"status"`
	LoggedAt     time.Time `gorm:"not null;index" json:"logged_at"`
	Name         string    `gorm:"type:text" json:"name"`
	RawResponse  string    `gorm:"type:text" json:"raw_response,omitempty"` // last structured response from the vision model
	ClarifyRound int       `gorm:"not null;default:0" json:"clarify_round"`
	ClarifyLog   string    `gorm:"type:text" json:"clarify_log,omitempty"` // JSON []ClarifyEntry, accumulated across rounds

	// Aggregate over items whose MacroSource is reference or manual, written on confirm.
	Calories          float64 `gorm:"not null;default:0" json:"calories"`
	ProteinGrams      float64 `gorm:"not null;default:0" json:"protein_grams"`
	CarbsGrams        float64 `gorm:"not null;default:0" json:"carbs_grams"`
	FatGrams          float64 `gorm:"not null;default:0" json:"fat_grams"`
	SugarGrams        float64 `gorm:"not null;default:0" json:"sugar_grams"`
	SodiumGrams       float64 `gorm:"not null;default:0" json:"sodium_grams"`
	DietaryFiberGrams float64 `gorm:"not null;default:0" json:"dietary_fiber_grams"`

	Items []FoodItem `gorm:"foreignKey:MealID" json:"items,omitempty"`
}

// ClarifyEntry is one question/answer pair, persisted so later rounds can replay
// the full history instead of re-asking what the user already answered.
type ClarifyEntry struct {
	Round    int    `json:"round"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// FoodItem is one food within a meal. UserID is denormalized from the parent
// meal so item queries are user-scoped; TenantModel supplies only FamilyID.
type FoodItem struct {
	models.TenantModel
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	MealID uuid.UUID `gorm:"type:uuid;not null;index" json:"meal_id"`
	Name   string    `gorm:"not null" json:"name"`

	// Preparation and State feed the retrieval query alongside Name. SR Legacy
	// encodes both as trailing description qualifiers ("...cooked, roasted"), so
	// omitting them leaves indexed tokens unused and ranks the wrong food first.
	// Empty means unknown, which contributes no query token rather than blocking
	// retrieval. Persisted so a clarification answer can re-run retrieval.
	Preparation string `gorm:"type:varchar(24)" json:"preparation,omitempty"`
	State       string `gorm:"type:varchar(16)" json:"state,omitempty"`
	// Brand is extracted at recognition time, when legible on the photo. It
	// gates whether candidate retrieval queries Open Food Facts at all — see
	// openspec/changes/add-open-food-facts-source/design.md. Empty means no
	// brand was legible, not that the food is unbranded.
	Brand string `gorm:"type:varchar(128)" json:"brand,omitempty"`

	FdcID        *int64     `gorm:"index" json:"fdc_id,omitempty"`
	CustomFoodID *uuid.UUID `gorm:"type:uuid;index" json:"custom_food_id,omitempty"`
	OffCode      *string    `gorm:"index" json:"off_code,omitempty"`
	MacroSource  string     `gorm:"type:varchar(16);not null;default:'none'" json:"macro_source"`
	WeightGrams  float64    `gorm:"not null" json:"weight_grams"`
	Confidence   float64    `gorm:"not null" json:"confidence"`

	Calories          float64 `gorm:"not null" json:"calories"`
	ProteinGrams      float64 `gorm:"not null" json:"protein_grams"`
	CarbsGrams        float64 `gorm:"not null" json:"carbs_grams"`
	FatGrams          float64 `gorm:"not null" json:"fat_grams"`
	SugarGrams        float64 `gorm:"not null" json:"sugar_grams"`
	SodiumGrams       float64 `gorm:"not null" json:"sodium_grams"`
	DietaryFiberGrams float64 `gorm:"not null" json:"dietary_fiber_grams"`

	// HasEstimate and the EstimatedXPer100g fields are Recognize's own
	// per-100g macro estimate for this item (see vision.Item.EstimatedProfile),
	// persisted at row-creation time regardless of whether a candidate match
	// is later found. This is what a clarification round's text-only follow-up
	// call cannot regenerate, and what a later weight-only edit rescales from
	// when MacroSource is estimated — see EstimatedProfile and
	// ApplyEstimatedProfile below. HasEstimate distinguishes "Recognize
	// produced no usable estimate" from "the estimate happens to be all
	// zeros" (e.g. water).
	HasEstimate                  bool    `gorm:"not null;default:false" json:"has_estimate,omitempty"`
	EstimatedCaloriesPer100g     float64 `gorm:"not null;default:0" json:"estimated_calories_per_100g,omitempty"`
	EstimatedProteinPer100g      float64 `gorm:"not null;default:0" json:"estimated_protein_per_100g,omitempty"`
	EstimatedCarbsPer100g        float64 `gorm:"not null;default:0" json:"estimated_carbs_per_100g,omitempty"`
	EstimatedFatPer100g          float64 `gorm:"not null;default:0" json:"estimated_fat_per_100g,omitempty"`
	EstimatedSugarPer100g        float64 `gorm:"not null;default:0" json:"estimated_sugar_per_100g,omitempty"`
	EstimatedSodiumPer100g       float64 `gorm:"not null;default:0" json:"estimated_sodium_per_100g,omitempty"`
	EstimatedDietaryFiberPer100g float64 `gorm:"not null;default:0" json:"estimated_dietary_fiber_per_100g,omitempty"`
}

// HasMacros reports whether the item contributes to its meal's aggregate.
func (i FoodItem) HasMacros() bool {
	return i.MacroSource == MacroSourceReference || i.MacroSource == MacroSourceManual ||
		i.MacroSource == MacroSourceEstimated
}

// SetEstimatedProfile persists Recognize's per-item estimate (nil when
// Recognize produced none for this item) onto the row. Called for every
// FoodItem built from a vision.Item, whether or not a candidate match is
// later found — see design.md decision 4.
func (i *FoodItem) SetEstimatedProfile(p *NutrientProfile) {
	if p == nil {
		return
	}
	i.HasEstimate = true
	i.EstimatedCaloriesPer100g = p.CaloriesPer100g
	i.EstimatedProteinPer100g = p.ProteinPer100g
	i.EstimatedCarbsPer100g = p.CarbsPer100g
	i.EstimatedFatPer100g = p.FatPer100g
	i.EstimatedSugarPer100g = p.SugarPer100g
	i.EstimatedSodiumPer100g = p.SodiumPer100g
	i.EstimatedDietaryFiberPer100g = p.DietaryFiberPer100g
}

// EstimatedProfile returns the item's persisted per-100g estimate and
// whether it is present and usable. A present-but-invalid estimate (any
// negative value — e.g. a parse/validation failure on Recognize's output)
// is treated as absent, not stored as nonsensical negative macros.
func (i FoodItem) EstimatedProfile() (NutrientProfile, bool) {
	if !i.HasEstimate {
		return NutrientProfile{}, false
	}
	p := NutrientProfile{
		CaloriesPer100g:     i.EstimatedCaloriesPer100g,
		ProteinPer100g:      i.EstimatedProteinPer100g,
		CarbsPer100g:        i.EstimatedCarbsPer100g,
		FatPer100g:          i.EstimatedFatPer100g,
		SugarPer100g:        i.EstimatedSugarPer100g,
		SodiumPer100g:       i.EstimatedSodiumPer100g,
		DietaryFiberPer100g: i.EstimatedDietaryFiberPer100g,
	}
	if p.CaloriesPer100g < 0 || p.ProteinPer100g < 0 || p.CarbsPer100g < 0 || p.FatPer100g < 0 ||
		p.SugarPer100g < 0 || p.SodiumPer100g < 0 || p.DietaryFiberPer100g < 0 {
		return NutrientProfile{}, false
	}
	return p, true
}

// PlausibleEstimatedProfile returns the item's persisted estimate and whether
// it is usable for automatic-resolution precedence, per
// openspec/specs/food-photo-recognition "Macro Estimate Fallback for
// Unmatched Items". This is stricter than, and separate from, EstimatedProfile:
// EstimatedProfile's present/non-negative semantics stay unchanged so a
// legacy row already persisted with MacroSource estimated keeps rescaling
// consistently on a later weight-only edit even if it would fail these
// bounds. Only the resolveItems binding decision (deciding whether a fresh
// estimate should beat a Select-matched fuzzy candidate) uses this stricter
// check.
func (i FoodItem) PlausibleEstimatedProfile() (NutrientProfile, bool) {
	p, ok := i.EstimatedProfile()
	if !ok {
		return NutrientProfile{}, false
	}

	const macroRoundingTolerance = 2.0 // g/100g, absorbs model rounding
	const macroCeiling = 100.0 + macroRoundingTolerance

	if p.ProteinPer100g > macroCeiling || p.CarbsPer100g > macroCeiling || p.FatPer100g > macroCeiling {
		return NutrientProfile{}, false
	}
	if p.ProteinPer100g+p.CarbsPer100g+p.FatPer100g > macroCeiling {
		return NutrientProfile{}, false
	}
	if p.SugarPer100g+p.DietaryFiberPer100g > p.CarbsPer100g+macroRoundingTolerance {
		return NutrientProfile{}, false
	}

	atwater := p.ProteinPer100g*4 + p.CarbsPer100g*4 + p.FatPer100g*9
	calorieTolerance := math.Max(25.0, atwater*0.15)
	if p.CaloriesPer100g < atwater-calorieTolerance {
		return NutrientProfile{}, false
	}

	return p, true
}

// CustomFood is a user's own per-100g profile, e.g. from a package label.
// Unique on (user_id, name) so name-based precedence over USDA has one winner.
type CustomFood struct {
	models.TenantModel
	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_custom_food_user_name" json:"user_id"`
	Name   string    `gorm:"not null;uniqueIndex:idx_custom_food_user_name" json:"name"`

	CaloriesPer100g     float64 `gorm:"not null" json:"calories_per_100g"`
	ProteinPer100g      float64 `gorm:"not null" json:"protein_per_100g"`
	CarbsPer100g        float64 `gorm:"not null" json:"carbs_per_100g"`
	FatPer100g          float64 `gorm:"not null" json:"fat_per_100g"`
	SugarPer100g        float64 `gorm:"not null" json:"sugar_per_100g"`
	SodiumPer100g       float64 `gorm:"not null" json:"sodium_per_100g"`
	DietaryFiberPer100g float64 `gorm:"not null" json:"dietary_fiber_per_100g"`
}

// FoodSearchTranslation is a user's cached free-text-to-USDA-vocabulary
// search-query translation, e.g. "porridge" -> "oatmeal" or "овсянка" ->
// "oatmeal". Unique on (user_id, original_query) so a refresh upserts the
// existing row in place rather than accumulating history. OriginalQuery is
// normalized (trimmed, lowercased) by the caller before it is ever compared
// or stored; TranslatedQuery is the term actually used to search USDA.
type FoodSearchTranslation struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_food_search_translation_user_query" json:"user_id"`
	OriginalQuery   string    `gorm:"not null;uniqueIndex:idx_food_search_translation_user_query" json:"original_query"`
	TranslatedQuery string    `gorm:"not null" json:"translated_query"`
}

// FoodCalibrationSample is a weighed-food ground-truth photo used to benchmark
// vision models. It never produces a FoodMeal.
type FoodCalibrationSample struct {
	models.TenantModel
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PhotoPath   string    `gorm:"type:text;not null" json:"photo_path"`
	GroundTruth string    `gorm:"type:text;not null" json:"ground_truth"` // JSON []GroundTruthItem
	CapturedAt  time.Time `gorm:"not null" json:"captured_at"`
}

// GroundTruthItem is one expected food in a calibration sample. Aliases and
// RefID make prediction matching deterministic rather than string-similarity based.
type GroundTruthItem struct {
	CanonicalName string   `json:"canonical_name"`
	Aliases       []string `json:"aliases,omitempty"`
	FdcID         *int64   `json:"fdc_id,omitempty"`
	CustomFoodID  *string  `json:"custom_food_id,omitempty"`
	OffCode       *string  `json:"off_code,omitempty"`
	WeightGrams   float64  `json:"weight_grams"`
}

// NutrientProfile is per-100g nutrition, the common shape returned by both the
// USDA index and custom foods so scaling has a single implementation.
type NutrientProfile struct {
	CaloriesPer100g     float64 `json:"calories_per_100g"`
	ProteinPer100g      float64 `json:"protein_per_100g"`
	CarbsPer100g        float64 `json:"carbs_per_100g"`
	FatPer100g          float64 `json:"fat_per_100g"`
	SugarPer100g        float64 `json:"sugar_per_100g"`
	SodiumPer100g       float64 `json:"sodium_per_100g"`
	DietaryFiberPer100g float64 `json:"dietary_fiber_per_100g"`
}

// Profile returns the custom food's per-100g values.
func (c CustomFood) Profile() NutrientProfile {
	return NutrientProfile{
		CaloriesPer100g:     c.CaloriesPer100g,
		ProteinPer100g:      c.ProteinPer100g,
		CarbsPer100g:        c.CarbsPer100g,
		FatPer100g:          c.FatPer100g,
		SugarPer100g:        c.SugarPer100g,
		SodiumPer100g:       c.SodiumPer100g,
		DietaryFiberPer100g: c.DietaryFiberPer100g,
	}
}

// ApplyProfile scales a per-100g profile by the item's weight and marks it
// as reference-sourced.
func (i *FoodItem) ApplyProfile(p NutrientProfile) {
	i.applyScaledProfile(p, MacroSourceReference)
}

// ApplyEstimatedProfile scales the item's own persisted per-100g estimate
// (set by SetEstimatedProfile at creation time) by its current weight and
// marks it as an AI estimate rather than a database-bound reference. Returns
// false, changing nothing, when no usable estimate is present or the item's
// weight is not positive — callers should leave the item at whatever
// MacroSource it already has (typically none) in that case. The weight check
// lives here, not only at call sites, so every caller (including the
// unresolved-item fallback loop in resolveItems, which has no weight check
// of its own) is protected from silently "resolving" a zero-weight item to a
// zeroed-out estimate instead of leaving it flagged as needing review.
func (i *FoodItem) ApplyEstimatedProfile() bool {
	if i.WeightGrams <= 0 {
		return false
	}
	p, ok := i.EstimatedProfile()
	if !ok {
		return false
	}
	i.applyScaledProfile(p, MacroSourceEstimated)
	return true
}

func (i *FoodItem) applyScaledProfile(p NutrientProfile, source string) {
	f := i.WeightGrams / 100.0
	i.Calories = p.CaloriesPer100g * f
	i.ProteinGrams = p.ProteinPer100g * f
	i.CarbsGrams = p.CarbsPer100g * f
	i.FatGrams = p.FatPer100g * f
	i.SugarGrams = p.SugarPer100g * f
	i.SodiumGrams = p.SodiumPer100g * f
	i.DietaryFiberGrams = p.DietaryFiberPer100g * f
	i.MacroSource = source
}

// Aggregate sums the 7 macros over items that have usable macros. Items with
// MacroSource none are excluded rather than counted as zero-value foods.
func (m *FoodMeal) Aggregate(items []FoodItem) {
	m.Calories, m.ProteinGrams, m.CarbsGrams, m.FatGrams = 0, 0, 0, 0
	m.SugarGrams, m.SodiumGrams, m.DietaryFiberGrams = 0, 0, 0
	for _, it := range items {
		if !it.HasMacros() {
			continue
		}
		m.Calories += it.Calories
		m.ProteinGrams += it.ProteinGrams
		m.CarbsGrams += it.CarbsGrams
		m.FatGrams += it.FatGrams
		m.SugarGrams += it.SugarGrams
		m.SodiumGrams += it.SodiumGrams
		m.DietaryFiberGrams += it.DietaryFiberGrams
	}
}
