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
)

// Item is one food recognized in a photo, before any candidate matching.
// Preparation and State are a controlled vocabulary; an empty value means
// the model could not tell, not that the food has none.
type Item struct {
	Name        string  `json:"name"`
	Preparation string  `json:"preparation,omitempty"`
	State       string  `json:"state,omitempty"`
	WeightGrams float64 `json:"weight_grams"`
	Confidence  float64 `json:"confidence"`
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
	Description  string     `json:"description"`
}

// ItemCandidates pairs a recognized item (by its index in the
// RecognizeResult.Items slice that produced it) with its retrieved
// shortlist.
type ItemCandidates struct {
	ItemIndex  int         `json:"item_index"`
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

// Client recognizes foods in a photo and selects among retrieved candidates.
// Every implementation sets store:false on outbound requests — see design.md
// "Third-Party Disclosure and Retention".
type Client interface {
	// Recognize sends the photo and asks the model to identify its foods. hint,
	// when non-empty, is a caller-supplied free-text correction (e.g. "this is
	// chicken and rice, not berries") included in the prompt for that call —
	// used by initial upload and hint-driven reanalysis. An empty hint is the
	// automatic upload/retry path and leaves the request unchanged from before
	// hint support existed.
	Recognize(ctx context.Context, image []byte, mimeType, hint string) (*RecognizeResult, error)
	// EstimateWeights estimates only the expert components whose weights were
	// omitted. Implementations must preserve ComponentIndex in their response.
	EstimateWeights(ctx context.Context, image []byte, mimeType string, components []WeightEstimateInput) (*WeightEstimateResult, error)
	// Clarify is text-only: it does not re-send the photo, only the items
	// recognized so far and the full question/answer history. It returns the
	// same RecognizeResult shape — updated items, and further
	// ClarificationQuestions if still ambiguous. See design.md "Clarification
	// Rounds": re-sending the image every round would make image tokens the
	// dominant cost for no additional information.
	Clarify(ctx context.Context, priorItems []Item, history []ClarifyTurn) (*RecognizeResult, error)
	Select(ctx context.Context, itemCandidates []ItemCandidates) (*SelectResult, error)
}
