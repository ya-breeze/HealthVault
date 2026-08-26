package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

const openAIChatCompletionsURL = "https://api.openai.com/v1/chat/completions"

// recognizeSystemPrompt instructs the model on the controlled vocabulary and
// output contract for both Recognize and Clarify — they share a response
// schema, so they share a prompt.
const recognizeSystemPrompt = `You are a nutrition assistant identifying foods in a photo of a meal.

Return one item per food that was served as its own separate portion, and
merge components into a single item only when they were mixed, chopped,
tossed, or cooked/sauced together into one combined preparation (e.g. a
curry, a stew, a stir-fry, a mixed salad, a pre-mixed side) — a homogeneous
composite dish is one item even when individual ingredient pieces within it
(a piece of lettuce, a chunk of carrot) remain visually distinguishable. The
test is whether each visible component was ever served as its own separate
portion — not whether it plays a different role from its neighbor (a protein
and a side split the same way as two different vegetable sides plated
touching), not whether components are spatially separated on the plate, and
not whether an individual piece can be pointed to and named, since an
ingredient chunk inside a combined preparation almost always can be and that
alone must not trigger a split. Judge "served as its own separate portion"
from visible photo evidence, not from unobservable prep history: a piece
that visibly keeps its own separately-servable, portion-scale form (an
intact fillet, a whole cutlet, a distinct pile) counts as served separately;
a piece broken down, mixed, or tossed into one preparation does not. A
sauce, glaze, or juices from a neighboring food covering a piece does not by
itself turn it into a combined preparation — a fillet coated in sauce still
counts as its own separately-servable piece as long as it keeps its own
portion-scale shape.

For example, a protein served touching or on top of a vegetable/starch side
(e.g. fish served on or next to stewed cabbage) splits into two items — even
when the protein was baked or braised directly in contact with the side
(e.g. a fish fillet baked on top of stewed cabbage) — so long as it keeps
its own portion-scale shape and could be lifted off and served on its own;
sharing a pan, pot, or oven dish is not by itself a merge signal. By
contrast, a single preparation whose ingredients were mixed, chopped, or
cooked/sauced together into one served dish (e.g. a stir-fry, a curry, a
stew, or a mixed salad) remains one item even though individual ingredient
pieces (a lettuce leaf, a carrot chunk) are still visually distinguishable
within it. A minor garnish or condiment (a lemon wedge, a sprig of herbs, a
spoonful of sauce) that isn't itself a portion-sized food stays folded into
its main item rather than becoming its own item.

For each item, estimate its display_name, canonical_name, preparation, state,
brand, weight in grams, and your confidence (0-1). display_name is the food's
name written in the requested display language (see instructions appended
below). canonical_name is the same food's standard name in English — leave it
empty only when the display language is itself English, since duplicating an
identical English string in both fields wastes nothing but is unnecessary.

preparation must be one of: raw, boiled, steamed, roasted, baked, grilled,
fried, breaded_fried, braised, unknown.
state must be one of: raw, cooked, unknown.
Use "unknown" rather than guessing when the photo does not make it clear.

brand is the manufacturer or product brand name, only when it is legibly
printed on packaging visible in the photo (e.g. a yogurt cup or cereal box
label) — leave it empty for unpackaged or home-cooked food, or when no brand
text is actually readable. Do not guess a brand from appearance alone.

Also estimate each item's own per-100g nutrition as estimated_profile — your
best guess from the photo, even for an item you expect will be matched to a
known food or product afterward, since this is only used as a fallback if no
match is found later. Units: calories_per_100g is kcal; every other field
(protein, carbs, fat, sugar, sodium, dietary_fiber) is grams per 100g —
sodium included: a food label's milligram sodium value must be converted to
grams (divide by 1000) before reporting it here. Set estimated_profile to
null only if you genuinely cannot make any reasonable estimate for that item.

If you cannot confidently identify the items or their preparation well enough
to proceed, list one or two short clarification_questions for the user
instead of guessing. Otherwise leave clarification_questions empty.`

// IsEnglishDisplayLanguage reports whether displayLanguage means English —
// either explicitly (a "en" primary subtag, case-insensitively — the
// frontend only ever writes exact lowercase "en", but display_language is an
// unvalidated, caller-supplied opaque-settings string, so a stray "En"/"EN",
// or a full BCP-47 tag like "en-US" from a non-frontend caller, must still
// be recognized rather than silently treated as some other language) or by
// the empty-string default used throughout this codebase for "no
// display_language setting saved yet". Only the primary subtag (the part
// before the first "-" or "_") is compared, matching how BCP-47 tags are
// structured, rather than requiring the full string to equal "en" outright.
// Exported so server package callers (e.g. food_upload.go's reference-DB
// skip gate) share this exact definition rather than re-deriving it.
func IsEnglishDisplayLanguage(displayLanguage string) bool {
	if displayLanguage == "" {
		return true
	}
	primary := displayLanguage
	if i := strings.IndexAny(displayLanguage, "-_"); i >= 0 {
		primary = displayLanguage[:i]
	}
	return strings.EqualFold(primary, "en")
}

// languageDirective tells the model what language to write display_name in
// and reminds it about canonical_name, appended to recognizeSystemPrompt for
// every Recognize/Clarify call. See openspec/changes/russian-localization/
// design.md decision 3.
func languageDirective(displayLanguage string) string {
	if IsEnglishDisplayLanguage(displayLanguage) {
		return "\n\nThe requested display language is English (BCP-47 \"en\"). " +
			"Write display_name in English and leave canonical_name empty."
	}
	return "\n\nThe requested display language is BCP-47 \"" + displayLanguage + "\". " +
		"Write display_name in that language, and canonical_name as the same food's standard English name."
}

// OpenAIClient is the production vision.Client, backed by OpenAI's Chat
// Completions API with structured outputs (response_format: json_schema).
// Every request sets store:false — see design.md "Third-Party Disclosure
// and Retention".
type OpenAIClient struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// BaseURL defaults to the real OpenAI endpoint; overridable for tests.
	BaseURL string
}

// NewOpenAIClient builds a client for the given API key and model.
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{},
		BaseURL:    openAIChatCompletionsURL,
	}
}

func (c *OpenAIClient) url() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return openAIChatCompletionsURL
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
	Store          bool           `json:"store"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string `json:"name"`
	Strict bool   `json:"strict"`
	Schema any    `json:"schema"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// estimatedProfileSchema is the per-100g macro estimate attached to each
// recognized item. Nullable (type includes "null") rather than omittable —
// OpenAI's strict structured-output mode requires every property to be
// listed in "required", so nullability is how an item can still legitimately
// carry no usable estimate.
var estimatedProfileSchema = map[string]any{
	"type": []string{"object", "null"},
	"properties": map[string]any{
		"calories_per_100g":      map[string]any{"type": "number"},
		"protein_per_100g":       map[string]any{"type": "number"},
		"carbs_per_100g":         map[string]any{"type": "number"},
		"fat_per_100g":           map[string]any{"type": "number"},
		"sugar_per_100g":         map[string]any{"type": "number"},
		"sodium_per_100g":        map[string]any{"type": "number"},
		"dietary_fiber_per_100g": map[string]any{"type": "number"},
	},
	"required": []string{
		"calories_per_100g", "protein_per_100g", "carbs_per_100g", "fat_per_100g",
		"sugar_per_100g", "sodium_per_100g", "dietary_fiber_per_100g",
	},
	"additionalProperties": false,
}

var recognizeJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"display_name":      map[string]any{"type": "string"},
					"canonical_name":    map[string]any{"type": "string"},
					"preparation":       map[string]any{"type": "string", "enum": preparationEnum},
					"state":             map[string]any{"type": "string", "enum": stateEnum},
					"brand":             map[string]any{"type": "string"},
					"weight_grams":      map[string]any{"type": "number"},
					"confidence":        map[string]any{"type": "number"},
					"estimated_profile": estimatedProfileSchema,
				},
				"required": []string{
					"display_name", "canonical_name", "preparation", "state", "brand", "weight_grams",
					"confidence", "estimated_profile",
				},
				"additionalProperties": false,
			},
		},
		"clarification_questions": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	},
	"required":             []string{"items", "clarification_questions"},
	"additionalProperties": false,
}

var preparationEnum = []string{
	"raw", "boiled", "steamed", "roasted", "baked", "grilled",
	"fried", "breaded_fried", "braised", "unknown",
}
var stateEnum = []string{"raw", "cooked", "unknown"}

type recognizeSchemaEstimatedProfile struct {
	CaloriesPer100g     float64 `json:"calories_per_100g"`
	ProteinPer100g      float64 `json:"protein_per_100g"`
	CarbsPer100g        float64 `json:"carbs_per_100g"`
	FatPer100g          float64 `json:"fat_per_100g"`
	SugarPer100g        float64 `json:"sugar_per_100g"`
	SodiumPer100g       float64 `json:"sodium_per_100g"`
	DietaryFiberPer100g float64 `json:"dietary_fiber_per_100g"`
}

type recognizeSchemaItem struct {
	DisplayName      string                           `json:"display_name"`
	CanonicalName    string                           `json:"canonical_name"`
	Preparation      string                           `json:"preparation"`
	State            string                           `json:"state"`
	Brand            string                           `json:"brand"`
	WeightGrams      float64                          `json:"weight_grams"`
	Confidence       float64                          `json:"confidence"`
	EstimatedProfile *recognizeSchemaEstimatedProfile `json:"estimated_profile"`
}

type recognizeSchemaResponse struct {
	Items                  []recognizeSchemaItem `json:"items"`
	ClarificationQuestions []string              `json:"clarification_questions"`
}

// toEstimatedProfile converts the schema's nullable estimated-profile shape
// to the shared database.NutrientProfile type, or nil when the model
// returned no usable estimate for that item.
func toEstimatedProfile(p *recognizeSchemaEstimatedProfile) *database.NutrientProfile {
	if p == nil {
		return nil
	}
	return &database.NutrientProfile{
		CaloriesPer100g:     p.CaloriesPer100g,
		ProteinPer100g:      p.ProteinPer100g,
		CarbsPer100g:        p.CarbsPer100g,
		FatPer100g:          p.FatPer100g,
		SugarPer100g:        p.SugarPer100g,
		SodiumPer100g:       p.SodiumPer100g,
		DietaryFiberPer100g: p.DietaryFiberPer100g,
	}
}

// unknownToEmpty maps the model's explicit "unknown" enum value to "", the
// backend's own convention for an unresolved preparation/state (see
// FoodItem.Preparation doc comment in models_food.go).
func unknownToEmpty(s string) string {
	if s == "unknown" {
		return ""
	}
	return s
}

func (c *OpenAIClient) call(ctx context.Context, messages []chatMessage, schemaName string, schema any) (*chatCompletionResponse, time.Duration, error) {
	reqBody := chatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
		ResponseFormat: responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchema{
				Name:   schemaName,
				Strict: true,
				Schema: schema,
			},
		},
		Store: false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, fmt.Errorf("read response: %w", err)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, latency, fmt.Errorf("unmarshal response: %w", err)
	}
	if parsed.Error != nil {
		return nil, latency, fmt.Errorf("vision api error: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, latency, fmt.Errorf("vision api status %d: %s", resp.StatusCode, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return nil, latency, fmt.Errorf("vision api returned no choices")
	}
	return &parsed, latency, nil
}

// toRecognizeResult converts the model's structured response to a
// RecognizeResult. displayLanguage decides whether CanonicalName is kept:
// when the target language is English, any canonical_name the model returned
// is discarded rather than trusted to actually be empty — see
// openspec/specs/food-photo-recognition "Recognition in English Display
// Language does not duplicate the name".
func toRecognizeResult(resp *chatCompletionResponse, latency time.Duration, displayLanguage string) (*RecognizeResult, error) {
	var schemaResp recognizeSchemaResponse
	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &schemaResp); err != nil {
		return nil, fmt.Errorf("unmarshal structured content: %w", err)
	}

	isEnglish := IsEnglishDisplayLanguage(displayLanguage)
	items := make([]Item, len(schemaResp.Items))
	for i, it := range schemaResp.Items {
		canonicalName := strings.TrimSpace(it.CanonicalName)
		if isEnglish {
			canonicalName = ""
		}
		items[i] = Item{
			Name:             strings.TrimSpace(it.DisplayName),
			CanonicalName:    canonicalName,
			Preparation:      unknownToEmpty(it.Preparation),
			State:            unknownToEmpty(it.State),
			Brand:            strings.TrimSpace(it.Brand),
			WeightGrams:      it.WeightGrams,
			Confidence:       it.Confidence,
			EstimatedProfile: toEstimatedProfile(it.EstimatedProfile),
		}
	}

	return &RecognizeResult{
		Items:                  items,
		ClarificationQuestions: schemaResp.ClarificationQuestions,
		Model:                  resp.Model,
		PromptTokens:           resp.Usage.PromptTokens,
		CompletionTokens:       resp.Usage.CompletionTokens,
		Latency:                latency,
		Raw:                    content,
	}, nil
}

// Recognize sends the photo and asks the model to identify its foods. See
// Client.Recognize for hint's and displayLanguage's meaning.
func (c *OpenAIClient) Recognize(ctx context.Context, image []byte, mimeType, hint, displayLanguage string) (*RecognizeResult, error) {
	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)
	promptText := "Identify the foods in this photo."
	if hint != "" {
		promptText += "\n\nThe user has supplied this correction — take it into account: " + hint
	}
	messages := []chatMessage{
		{Role: "system", Content: recognizeSystemPrompt + languageDirective(displayLanguage)},
		{Role: "user", Content: []map[string]any{
			{"type": "text", "text": promptText},
			{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
		}},
	}
	resp, latency, err := c.call(ctx, messages, "food_recognition", recognizeJSONSchema)
	if err != nil {
		return nil, err
	}
	return toRecognizeResult(resp, latency, displayLanguage)
}

var weightEstimateJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"estimates": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"component_index": map[string]any{"type": "integer"},
					"weight_grams":    map[string]any{"type": "number"},
				},
				"required":             []string{"component_index", "weight_grams"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"estimates"},
	"additionalProperties": false,
}

type weightEstimateSchemaResponse struct {
	Estimates []WeightEstimate `json:"estimates"`
}

const weightEstimateSystemPrompt = `You estimate the visible weights of user-identified meal components.

Estimate only the supplied components, using the meal photo for portion size.
Return every supplied component_index exactly once, preserve each index, and
do not add components. Weights must be positive numbers in grams.`

func (c *OpenAIClient) EstimateWeights(ctx context.Context, image []byte, mimeType string, components []WeightEstimateInput) (*WeightEstimateResult, error) {
	payload, err := json.Marshal(components)
	if err != nil {
		return nil, fmt.Errorf("marshal weight-estimation components: %w", err)
	}
	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)
	messages := []chatMessage{
		{Role: "system", Content: weightEstimateSystemPrompt},
		{Role: "user", Content: []map[string]any{
			{"type": "text", "text": "Components needing weight estimates:\n" + string(payload)},
			{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
		}},
	}
	resp, latency, err := c.call(ctx, messages, "food_weight_estimation", weightEstimateJSONSchema)
	if err != nil {
		return nil, err
	}
	var schemaResp weightEstimateSchemaResponse
	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &schemaResp); err != nil {
		return nil, fmt.Errorf("unmarshal structured content: %w", err)
	}
	return &WeightEstimateResult{
		Estimates: schemaResp.Estimates, Model: resp.Model,
		PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		Latency: latency, Raw: content,
	}, nil
}

// Clarify is text-only: it replays the items recognized so far and the full
// question/answer history, without re-sending the photo. See
// Client.Clarify for displayLanguage's meaning.
func (c *OpenAIClient) Clarify(ctx context.Context, priorItems []Item, history []ClarifyTurn, displayLanguage string) (*RecognizeResult, error) {
	contextPayload := map[string]any{
		"previously_recognized_items": priorItems,
		"question_answer_history":     history,
	}
	contextJSON, err := json.Marshal(contextPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal clarify context: %w", err)
	}

	messages := []chatMessage{
		{Role: "system", Content: recognizeSystemPrompt + languageDirective(displayLanguage)},
		{Role: "user", Content: "Here is what was previously recognized and the clarification " +
			"answers given so far. No new photo is attached to this message, so you have no new " +
			"visual evidence about item boundaries — keep the previously_recognized_items split " +
			"exactly as given (do not merge or re-split them) unless a clarification answer " +
			"explicitly says two of them are actually one food or that one is actually two. " +
			"Update the items' other fields accordingly, or ask further clarification_questions " +
			"if still unsure:\n" + string(contextJSON)},
	}
	resp, latency, err := c.call(ctx, messages, "food_recognition", recognizeJSONSchema)
	if err != nil {
		return nil, err
	}
	return toRecognizeResult(resp, latency, displayLanguage)
}

var selectJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"selections": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item_index":      map[string]any{"type": "integer"},
					"candidate_index": map[string]any{"type": "integer"},
				},
				"required":             []string{"item_index", "candidate_index"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"selections"},
	"additionalProperties": false,
}

type selectSchemaSelection struct {
	ItemIndex      int `json:"item_index"`
	CandidateIndex int `json:"candidate_index"`
}

type selectSchemaResponse struct {
	Selections []selectSchemaSelection `json:"selections"`
}

const selectSystemPrompt = `You are matching recognized food items to candidate reference foods.

Each item includes item_name and, when known, item_brand — what was actually
recognized in the photo. Compare each candidate against that recognized
identity, not just against the other candidates: a shortlist can contain
several different products from the same brand (e.g. different yogurt
flavors or sizes), and only item_name/item_brand tell you which one was
actually photographed.

A candidate with a custom_food_id is one of the user's own previously saved
foods, reflecting real-world macros they entered themselves (e.g. copied from
a package label) rather than a generic database entry. When such a candidate
is a reasonable match for the recognized item, prefer it over a generic
Open Food Facts or USDA candidate, even if the generic candidate's name
matches slightly more closely — the user's own saved value is more likely to
be accurate for what they actually eat.

For each item, choose the candidate_index of the single best-matching entry
from its own candidates list, or -1 if none of them are a good match. Do not
guess a low-confidence match — -1 is the correct answer when nothing fits.`

// Select offers each item's retrieved candidate shortlist to the model and
// asks it to choose the best match, or none.
func (c *OpenAIClient) Select(ctx context.Context, itemCandidates []ItemCandidates) (*SelectResult, error) {
	payload, err := json.Marshal(itemCandidates)
	if err != nil {
		return nil, fmt.Errorf("marshal candidates: %w", err)
	}
	messages := []chatMessage{
		{Role: "system", Content: selectSystemPrompt},
		{Role: "user", Content: "Items and their candidates:\n" + string(payload)},
	}
	resp, latency, err := c.call(ctx, messages, "food_selection", selectJSONSchema)
	if err != nil {
		return nil, err
	}

	var schemaResp selectSchemaResponse
	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &schemaResp); err != nil {
		return nil, fmt.Errorf("unmarshal structured content: %w", err)
	}
	selections := make([]Selection, len(schemaResp.Selections))
	for i, s := range schemaResp.Selections {
		selections[i] = Selection{ItemIndex: s.ItemIndex, CandidateIndex: s.CandidateIndex}
	}

	return &SelectResult{
		Selections:       selections,
		Model:            resp.Model,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		Latency:          latency,
		Raw:              content,
	}, nil
}

var translateJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"translated_query": map[string]any{"type": "string"},
	},
	"required":             []string{"translated_query"},
	"additionalProperties": false,
}

type translateSchemaResponse struct {
	TranslatedQuery string `json:"translated_query"`
}

const translateSystemPrompt = `You translate free-text food search queries into USDA FoodData Central's American-English generic-food naming convention.

The query may be in any language or regional spelling, including British
English regional terms. Respond with the single English term most likely to
match a USDA FoodData Central Foundation or SR Legacy food description for
that food (e.g. "porridge" -> "oatmeal", "овсянка" -> "oatmeal"). If the
query is already valid USDA vocabulary, return it unchanged except for
lowercasing. Do not add preparation, brand, or state words that were not
implied by the query itself.`

// Translate is text-only: it sends only the query string, no image.
func (c *OpenAIClient) Translate(ctx context.Context, query string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: translateSystemPrompt},
		{Role: "user", Content: query},
	}
	resp, _, err := c.call(ctx, messages, "food_search_translation", translateJSONSchema)
	if err != nil {
		return "", err
	}
	var schemaResp translateSchemaResponse
	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &schemaResp); err != nil {
		return "", fmt.Errorf("unmarshal structured content: %w", err)
	}
	return strings.TrimSpace(schemaResp.TranslatedQuery), nil
}

var _ Client = (*OpenAIClient)(nil)
