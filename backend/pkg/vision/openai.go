package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openAIChatCompletionsURL = "https://api.openai.com/v1/chat/completions"

// recognizeSystemPrompt instructs the model on the controlled vocabulary and
// output contract for both Recognize and Clarify — they share a response
// schema, so they share a prompt.
const recognizeSystemPrompt = `You are a nutrition assistant identifying foods in a photo of a meal.

For each distinct food item, estimate its name, preparation, state, weight in
grams, and your confidence (0-1).

preparation must be one of: raw, boiled, steamed, roasted, baked, grilled,
fried, breaded_fried, braised, unknown.
state must be one of: raw, cooked, unknown.
Use "unknown" rather than guessing when the photo does not make it clear.

If you cannot confidently identify the items or their preparation well enough
to proceed, list one or two short clarification_questions for the user
instead of guessing. Otherwise leave clarification_questions empty.`

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

var recognizeJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]any{"type": "string"},
					"preparation":  map[string]any{"type": "string", "enum": preparationEnum},
					"state":        map[string]any{"type": "string", "enum": stateEnum},
					"weight_grams": map[string]any{"type": "number"},
					"confidence":   map[string]any{"type": "number"},
				},
				"required":             []string{"name", "preparation", "state", "weight_grams", "confidence"},
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

type recognizeSchemaItem struct {
	Name        string  `json:"name"`
	Preparation string  `json:"preparation"`
	State       string  `json:"state"`
	WeightGrams float64 `json:"weight_grams"`
	Confidence  float64 `json:"confidence"`
}

type recognizeSchemaResponse struct {
	Items                  []recognizeSchemaItem `json:"items"`
	ClarificationQuestions []string              `json:"clarification_questions"`
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

func toRecognizeResult(resp *chatCompletionResponse, latency time.Duration) (*RecognizeResult, error) {
	var schemaResp recognizeSchemaResponse
	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &schemaResp); err != nil {
		return nil, fmt.Errorf("unmarshal structured content: %w", err)
	}

	items := make([]Item, len(schemaResp.Items))
	for i, it := range schemaResp.Items {
		items[i] = Item{
			Name:        it.Name,
			Preparation: unknownToEmpty(it.Preparation),
			State:       unknownToEmpty(it.State),
			WeightGrams: it.WeightGrams,
			Confidence:  it.Confidence,
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

// Recognize sends the photo and asks the model to identify its foods.
func (c *OpenAIClient) Recognize(ctx context.Context, image []byte, mimeType string) (*RecognizeResult, error) {
	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)
	messages := []chatMessage{
		{Role: "system", Content: recognizeSystemPrompt},
		{Role: "user", Content: []map[string]any{
			{"type": "text", "text": "Identify the foods in this photo."},
			{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
		}},
	}
	resp, latency, err := c.call(ctx, messages, "food_recognition", recognizeJSONSchema)
	if err != nil {
		return nil, err
	}
	return toRecognizeResult(resp, latency)
}

// Clarify is text-only: it replays the items recognized so far and the full
// question/answer history, without re-sending the photo.
func (c *OpenAIClient) Clarify(ctx context.Context, priorItems []Item, history []ClarifyTurn) (*RecognizeResult, error) {
	contextPayload := map[string]any{
		"previously_recognized_items": priorItems,
		"question_answer_history":     history,
	}
	contextJSON, err := json.Marshal(contextPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal clarify context: %w", err)
	}

	messages := []chatMessage{
		{Role: "system", Content: recognizeSystemPrompt},
		{Role: "user", Content: "Here is what was previously recognized and the clarification " +
			"answers given so far. Update the items accordingly, or ask further " +
			"clarification_questions if still unsure:\n" + string(contextJSON)},
	}
	resp, latency, err := c.call(ctx, messages, "food_recognition", recognizeJSONSchema)
	if err != nil {
		return nil, err
	}
	return toRecognizeResult(resp, latency)
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

var _ Client = (*OpenAIClient)(nil)
