// Package vision recognizes foods in a meal photo and selects among
// retrieved reference-food candidates.
//
// This is deliberately two calls, not one: candidate retrieval (see
// pkg/usda) depends on knowing what the model recognized first, so a photo
// cannot be recognized and matched in a single round trip. See
// openspec/changes/photo-food-nutrition-logging/design.md, "USDA Storage and
// Candidate Matching".
package vision

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// Item is one food recognized in a photo, before any candidate matching.
// Preparation and State are a controlled vocabulary; an empty value means
// the model could not tell, not that the food has none.
type Item struct {
	// Name is the Display Name: in the target language passed to Recognize/
	// Clarify. See openspec/specs/display-language "Display Language Passed
	// to Recognition".
	// Tagged display_name, not name: an Item is serialized outbound to the
	// model (Clarify's previously_recognized_items payload), whose system
	// prompt and response schema speak only of display_name/canonical_name.
	// Replaying prior items under a key the instructions never mention asks
	// the model to "update the items accordingly" using vocabulary it wasn't
	// given — a plausible way for a clarification round to lose the language
	// or drop the canonical name. Nothing unmarshals an Item from stored
	// JSON, so the tag is free to match the prompt. Found in code review.
	Name        string  `json:"display_name"`
	Preparation string  `json:"preparation,omitempty"`
	State       string  `json:"state,omitempty"`
	Brand       string  `json:"brand,omitempty"`
	WeightGrams float64 `json:"weight_grams"`
	Confidence  float64 `json:"confidence"`
	// CanonicalName is the same food's English identity, produced by the same
	// call as Name. Empty when the target language is English, rather than
	// duplicating Name — see openspec/specs/food-photo-recognition "Food
	// Recognition and Clarification Questions".
	CanonicalName string `json:"canonical_name,omitempty"`
	// EstimatedProfile is Recognize's own per-100g macro estimate for this
	// item, produced in the same call as recognition — no separate model
	// call. Nil when Recognize produced none (or an invalid one) for this
	// item. Used as a macro-source-of-last-resort when candidate selection
	// finds no match — see
	// openspec/changes/composite-food-recognition/design.md decision 4.
	EstimatedProfile *database.NutrientProfile `json:"estimated_profile,omitempty"`
}

// RecognizeResult is the outcome of the first call: what foods are in the
// photo. A non-empty ClarificationQuestions means the model could not
// confidently recognize the photo well enough to proceed to matching.
type RecognizeResult struct {
	Items                  []Item   `json:"items"`
	ClarificationQuestions []string `json:"clarification_questions,omitempty"`

	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Latency          time.Duration `json:"latency"`
	Raw              string        `json:"-"` // full raw response body, stored on FoodMeal.RawResponse
}

// WeightEstimateInput identifies an expert-supplied component whose weight
// still needs to be estimated. ComponentIndex is stable across the request
// and response, so reordered model output cannot attach a weight to the
// wrong ingredient.
type WeightEstimateInput struct {
	ComponentIndex int    `json:"component_index"`
	Name           string `json:"name"`
}

type WeightEstimate struct {
	ComponentIndex int     `json:"component_index"`
	WeightGrams    float64 `json:"weight_grams"`
}

type WeightEstimateResult struct {
	Estimates []WeightEstimate `json:"estimates"`

	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Latency          time.Duration `json:"latency"`
	Raw              string        `json:"-"`
}

// Candidate is one retrieved reference food offered to the model for
// selection against a recognized item.
type Candidate struct {
	FdcID        *int64     `json:"fdc_id,omitempty"`
	CustomFoodID *uuid.UUID `json:"custom_food_id,omitempty"`
	OffCode      *string    `json:"off_code,omitempty"`
	// Brands is populated for Open Food Facts candidates (empty for USDA and
	// custom-food ones) so the shortlist itself shows which brand each
	// candidate actually is — useful for the selection model and the review
	// UI even though the retrieval query already guarantees a brand match.
	Brands      string `json:"brands,omitempty"`
	Description string `json:"description"`
}

// ItemCandidates pairs a recognized item (by its index in the
// RecognizeResult.Items slice that produced it) with its retrieved
// shortlist, plus the recognized item's own name and brand — Select is a
// genuinely stateless, text-only call that never replays anything from the
// Recognize call, so without these fields the model comparing candidates has
// no idea what it is actually comparing them against. See
// openspec/changes/add-open-food-facts-source/design.md "Selection is
// offered the recognized item's own name and brand".
type ItemCandidates struct {
	ItemIndex  int         `json:"item_index"`
	ItemName   string      `json:"item_name"`
	ItemBrand  string      `json:"item_brand,omitempty"`
	Candidates []Candidate `json:"candidates"`
}

// Selection is the model's choice for one item. CandidateIndex is an index
// into that item's Candidates, or -1 for "none of these" — the model is
// never forced to bind a low-confidence match. See design.md "Matching is
// candidate retrieval, not auto-assignment."
type Selection struct {
	ItemIndex      int `json:"item_index"`
	CandidateIndex int `json:"candidate_index"`
}

// SelectResult is the outcome of the second call: which candidate, if any,
// matches each recognized item.
type SelectResult struct {
	Selections []Selection `json:"selections"`

	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Latency          time.Duration `json:"latency"`
	Raw              string        `json:"-"`
}

// ClarifyTurn is one question/answer pair replayed to the model on a
// clarification round.
type ClarifyTurn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// AdviceInput is everything the model is told when writing nutrition advice.
// Label and Reasons are the caller's already-computed Healthiness Label inputs,
// not a judgment for the model to revisit. Target figures are supplied by the
// server from the user's Nutrition Target.
type AdviceInput struct {
	Label              string   `json:"label"`
	Reasons            []string `json:"reasons"`
	MeanCalories       float64  `json:"mean_calories"`
	MeanProteinGrams   float64  `json:"mean_protein_grams"`
	MeanCarbsGrams     float64  `json:"mean_carbs_grams"`
	MeanFatGrams       float64  `json:"mean_fat_grams"`
	MeanSugarGrams     float64  `json:"mean_sugar_grams"`
	MeanSodiumGrams    float64  `json:"mean_sodium_grams"`
	TargetCalories     int      `json:"target_calories"`
	TargetProteinGrams int      `json:"target_protein_grams"`
	TargetCarbsGrams   int      `json:"target_carbs_grams"`
	TargetFatGrams     int      `json:"target_fat_grams"`
	DisplayLanguage    string   `json:"display_language"`
}

// NutritionChatSignal is one Healthiness Label signal's workings, exactly as
// the deterministic heuristic computed them: what was measured, the unit that
// measurement is in, the verdict it produced, and the two boundaries it was
// judged against. The model is given these so it can explain a flagged advice
// line from the same arithmetic the user is looking at on screen, rather than
// inventing a threshold of its own.
type NutritionChatSignal struct {
	Code        string  `json:"code"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Verdict     string  `json:"verdict"`
	Reason      string  `json:"reason,omitempty"`
	OffBoundary float64 `json:"off_boundary"`
	FarBoundary float64 `json:"far_boundary"`
}

// NutritionChatTurn is one exchange already on screen. Role is "user" or
// "assistant".
type NutritionChatTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// NutritionChatInput is everything the model is told when answering a question
// about the nutrition advice. It is the same evidence Advise was given, plus
// the signal workings behind it and the conversation so far.
//
// Deliberately absent: weight, steps and sleep. The Healthiness Label is
// computed from logged food alone, so handing the model signals the label never
// measured would invite it to assert a link the arithmetic never established,
// and the user could not tell that assertion apart from the rest of the answer.
type NutritionChatInput struct {
	Label              string                `json:"label"`
	Reasons            []string              `json:"reasons"`
	Signals            []NutritionChatSignal `json:"signals"`
	EligibleDays       int                   `json:"eligible_days"`
	WindowDays         int                   `json:"window_days"`
	MeanCalories       float64               `json:"mean_calories"`
	MeanProteinGrams   float64               `json:"mean_protein_grams"`
	MeanCarbsGrams     float64               `json:"mean_carbs_grams"`
	MeanFatGrams       float64               `json:"mean_fat_grams"`
	MeanSugarGrams     float64               `json:"mean_sugar_grams"`
	MeanSodiumGrams    float64               `json:"mean_sodium_grams"`
	TargetCalories     int                   `json:"target_calories"`
	TargetProteinGrams int                   `json:"target_protein_grams"`
	TargetCarbsGrams   int                   `json:"target_carbs_grams"`
	TargetFatGrams     int                   `json:"target_fat_grams"`
	DisplayLanguage    string                `json:"display_language"`
	// Turns is the conversation already on screen, oldest first, and Question
	// is what the user just asked. Turns is replayed in full on every call
	// because no implementation keeps a thread.
	Turns    []NutritionChatTurn `json:"turns,omitempty"`
	Question string              `json:"question"`
}

// NutritionChatResult is one answer. It carries the usual accounting fields so
// a chat call is as traceable as any other call in this package.
type NutritionChatResult struct {
	Answer string `json:"answer"`

	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Latency          time.Duration `json:"latency"`
}

// Client recognizes foods in a photo and selects among retrieved candidates.
// Every implementation sets store:false on outbound requests — see design.md
// "Third-Party Disclosure and Retention".
type Client interface {
	// Recognize sends the photo and asks the model to identify its foods. hint,
	// when non-empty, is a caller-supplied free-text correction (e.g. "this is
	// chicken and rice, not berries") included in the prompt for that call —
	// used by initial upload and hint-driven reanalysis. An empty hint is the
	// automatic upload/retry path and leaves the request unchanged from before
	// hint support existed. displayLanguage is a BCP-47-ish code (e.g. "en",
	// "ru") the caller's Display Name is written in; each Item's
	// CanonicalName is additionally produced in English, left empty when
	// displayLanguage is "en". See openspec/specs/display-language "Display
	// Language Passed to Recognition".
	Recognize(ctx context.Context, image []byte, mimeType, hint, displayLanguage string) (*RecognizeResult, error)
	// EstimateWeights estimates only the expert components whose weights were
	// omitted. Implementations must preserve ComponentIndex in their response.
	EstimateWeights(ctx context.Context, image []byte, mimeType string, components []WeightEstimateInput) (*WeightEstimateResult, error)
	// Clarify is text-only: it does not re-send the photo, only the items
	// recognized so far and the full question/answer history. It returns the
	// same RecognizeResult shape — updated items, and further
	// ClarificationQuestions if still ambiguous. See design.md "Clarification
	// Rounds": re-sending the image every round would make image tokens the
	// dominant cost for no additional information. displayLanguage has the
	// same meaning as in Recognize.
	//
	// description is the user's own written description of the meal on the
	// describe path, and empty on the photo path. It has to be replayed every
	// round because it is the only evidence a described meal has: Describe is
	// told to ask clarification_questions "instead of guessing" when the text
	// is vague, so the very meals that reach Clarify are the ones most likely
	// to arrive with an empty priorItems — leaving the model nothing at all to
	// clarify. Unlike a photo, replaying it costs a handful of text tokens.
	Clarify(ctx context.Context, description string, priorItems []Item, history []ClarifyTurn, displayLanguage string) (*RecognizeResult, error)
	Select(ctx context.Context, itemCandidates []ItemCandidates) (*SelectResult, error)
	// Translate maps a free-text food-search query, in any language or
	// regional spelling, to the term most likely to appear in USDA
	// FoodData Central's American-English generic-food naming (e.g.
	// "porridge" -> "oatmeal", "овсянка" -> "oatmeal"). Text-only, no
	// image. See openspec/changes/multilingual-food-search/design.md.
	Translate(ctx context.Context, query string) (string, error)
	// Advise is text-only. Label and Reasons are an already-computed judgment
	// that implementations must never dispute; returned lines are written in
	// DisplayLanguage.
	Advise(ctx context.Context, in AdviceInput) ([]string, error)
	// Describe is text-only: it identifies foods from the user's own written
	// description of a meal, with no image at all — the manual-entry
	// counterpart to Recognize. It returns the same RecognizeResult shape and
	// reuses recognizeJSONSchema, so every downstream stage (processRecognition,
	// resolveItems, persistAnalysis, the clarify loop) works on its result
	// exactly as it does on Recognize's. displayLanguage has the same meaning
	// it has on Recognize: the Display Name comes back in that language, and
	// each Item's CanonicalName is additionally produced in English.
	Describe(ctx context.Context, description, displayLanguage string) (*RecognizeResult, error)
	// NutritionChat is text-only and bounded: it answers one question about the
	// advice the user is looking at, from the evidence that advice was built
	// on. The conversation so far is replayed on every call, the way Clarify
	// replays its rounds, because nothing here keeps a thread — the turns live
	// in the browser tab and are discarded when it closes.
	//
	// Implementations must hold the same standing rule Advise has: the label
	// and its reason codes are an already-computed judgment, and the model
	// never disputes them.
	NutritionChat(ctx context.Context, in NutritionChatInput) (*NutritionChatResult, error)
}
