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
	Name        string
	Preparation string
	State       string
	WeightGrams float64
	Confidence  float64
}

// RecognizeResult is the outcome of the first call: what foods are in the
// photo. A non-empty ClarificationQuestions means the model could not
// confidently recognize the photo well enough to proceed to matching.
type RecognizeResult struct {
	Items                  []Item
	ClarificationQuestions []string

	Model            string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
	Raw              string // full raw response body, stored on FoodMeal.RawResponse
}

// Candidate is one retrieved reference food offered to the model for
// selection against a recognized item.
type Candidate struct {
	FdcID        *int64
	CustomFoodID *uuid.UUID
	Description  string
}

// ItemCandidates pairs a recognized item (by its index in the
// RecognizeResult.Items slice that produced it) with its retrieved
// shortlist.
type ItemCandidates struct {
	ItemIndex  int
	Candidates []Candidate
}

// Selection is the model's choice for one item. CandidateIndex is an index
// into that item's Candidates, or -1 for "none of these" — the model is
// never forced to bind a low-confidence match. See design.md "Matching is
// candidate retrieval, not auto-assignment."
type Selection struct {
	ItemIndex      int
	CandidateIndex int
}

// SelectResult is the outcome of the second call: which candidate, if any,
// matches each recognized item.
type SelectResult struct {
	Selections []Selection

	Model            string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
	Raw              string
}

// ClarifyTurn is one question/answer pair replayed to the model on a
// clarification round.
type ClarifyTurn struct {
	Question string
	Answer   string
}

// Client recognizes foods in a photo and selects among retrieved candidates.
// Every implementation sets store:false on outbound requests — see design.md
// "Third-Party Disclosure and Retention".
type Client interface {
	Recognize(ctx context.Context, image []byte, mimeType string) (*RecognizeResult, error)
	// Clarify is text-only: it does not re-send the photo, only the items
	// recognized so far and the full question/answer history. It returns the
	// same RecognizeResult shape — updated items, and further
	// ClarificationQuestions if still ambiguous. See design.md "Clarification
	// Rounds": re-sending the image every round would make image tokens the
	// dominant cost for no additional information.
	Clarify(ctx context.Context, priorItems []Item, history []ClarifyTurn) (*RecognizeResult, error)
	Select(ctx context.Context, itemCandidates []ItemCandidates) (*SelectResult, error)
}
