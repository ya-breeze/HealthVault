package vision_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/vision"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *vision.OpenAIClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := vision.NewOpenAIClient("test-key", "gpt-5.6-luna")
	c.BaseURL = srv.URL
	return c
}

func chatResponse(t *testing.T, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"model": "gpt-5.6-luna-2026-01-01",
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
		"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
	})
	if err != nil {
		t.Fatalf("marshal chat response: %v", err)
	}
	return string(b)
}

func TestOpenAIClient_Recognize_SetsStoreFalseAndSendsImage(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header with test-key, got %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"name":"chicken breast","preparation":"roasted","state":"cooked","weight_grams":180,"confidence":0.9}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{0xFF, 0xD8, 0xFF}, "image/jpeg", "")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	if store, _ := capturedBody["store"].(bool); store {
		t.Error("expected store=false in the request body")
	}
	if model, _ := capturedBody["model"].(string); model != "gpt-5.6-luna" {
		t.Errorf("expected model gpt-5.6-luna, got %q", model)
	}
	messages, _ := capturedBody["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	userMsg := messages[1].(map[string]any)
	content, _ := userMsg["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected image_url content alongside text, got %+v", content)
	}
	imagePart := content[1].(map[string]any)
	if imagePart["type"] != "image_url" {
		t.Errorf("expected an image_url content part, got %+v", imagePart)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Name != "chicken breast" || result.Items[0].WeightGrams != 180 {
		t.Errorf("unexpected item: %+v", result.Items[0])
	}
	if result.Model != "gpt-5.6-luna-2026-01-01" {
		t.Errorf("expected returned model id, got %q", result.Model)
	}
	if result.PromptTokens != 100 || result.CompletionTokens != 20 {
		t.Errorf("expected token usage to be captured, got %+v", result)
	}
}

func TestOpenAIClient_Recognize_UnknownMapsToEmptyString(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"name":"mystery sauce","preparation":"unknown","state":"unknown","weight_grams":30,"confidence":0.4}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if result.Items[0].Preparation != "" || result.Items[0].State != "" {
		t.Errorf("expected unknown to map to empty string, got preparation=%q state=%q",
			result.Items[0].Preparation, result.Items[0].State)
	}
}

func TestOpenAIClient_Recognize_ClarificationQuestions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[],"clarification_questions":["Is this dish spicy?"]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(result.ClarificationQuestions) != 1 {
		t.Fatalf("expected 1 clarification question, got %+v", result.ClarificationQuestions)
	}
}

func TestOpenAIClient_Recognize_HintIncludedAlongsideImage(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
		w.Write([]byte(chatResponse(t,                //nolint:errcheck
			`{"items":[{"name":"chicken breast","preparation":"roasted","state":"cooked","weight_grams":180,"confidence":0.9}],"clarification_questions":[]}`)))
	})

	_, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "this is chicken and rice, not berries")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	b, err := json.Marshal(capturedBody)
	if err != nil {
		t.Fatalf("marshal captured body: %v", err)
	}
	if !strings.Contains(string(b), "this is chicken and rice, not berries") {
		t.Errorf("expected the hint text in the request body, got %s", b)
	}
	if !strings.Contains(string(b), "image_url") {
		t.Error("expected the image to still be sent alongside the hint")
	}
}

func TestOpenAIClient_Recognize_NoHintUnchangedPrompt(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)                                 //nolint:errcheck
		w.Write([]byte(chatResponse(t, `{"items":[],"clarification_questions":[]}`))) //nolint:errcheck
	})

	_, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	messages, _ := capturedBody["messages"].([]any)
	userMsg := messages[1].(map[string]any)
	content, _ := userMsg["content"].([]any)
	textPart := content[0].(map[string]any)
	if textPart["text"] != "Identify the foods in this photo." {
		t.Errorf("expected the unchanged prompt text with no hint, got %q", textPart["text"])
	}
}

func TestOpenAIClient_Clarify_SendsNoImageContent(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
		w.Write([]byte(chatResponse(t,                //nolint:errcheck
			`{"items":[{"name":"sauce","preparation":"unknown","state":"unknown","weight_grams":30,"confidence":0.6}],"clarification_questions":[]}`)))
	})

	_, err := c.Clarify(context.Background(),
		[]vision.Item{{Name: "sauce", WeightGrams: 30}},
		[]vision.ClarifyTurn{{Question: "Cream or tomato based?", Answer: "Tomato-based"}})
	if err != nil {
		t.Fatalf("Clarify: %v", err)
	}

	b, _ := json.Marshal(capturedBody)
	if strings.Contains(string(b), "image_url") {
		t.Error("expected no image_url content in a Clarify request")
	}
}

func TestOpenAIClient_EstimateWeightsPreservesIndexesAndSendsImage(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)                                                                                        //nolint:errcheck
		w.Write([]byte(chatResponse(t, `{"estimates":[{"component_index":2,"weight_grams":40},{"component_index":0,"weight_grams":150}]}`))) //nolint:errcheck
	})
	result, err := c.EstimateWeights(context.Background(), []byte{1, 2, 3}, "image/jpeg", []vision.WeightEstimateInput{
		{ComponentIndex: 0, Name: "Chicken"}, {ComponentIndex: 2, Name: "Salsa"},
	})
	if err != nil {
		t.Fatalf("EstimateWeights: %v", err)
	}
	if len(result.Estimates) != 2 || result.Estimates[0].ComponentIndex != 2 || result.Estimates[1].WeightGrams != 150 {
		t.Fatalf("unexpected estimates: %+v", result.Estimates)
	}
	body, _ := json.Marshal(capturedBody)
	if !strings.Contains(string(body), "image_url") || !strings.Contains(string(body), "component_index") {
		t.Fatalf("expected image and indexed components, got %s", body)
	}
	if store, _ := capturedBody["store"].(bool); store {
		t.Error("expected store=false")
	}
}

func TestOpenAIClient_Select_ChoosesCandidateOrNone(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"selections":[{"item_index":0,"candidate_index":1},{"item_index":1,"candidate_index":-1}]}`)))
	})

	result, err := c.Select(context.Background(), []vision.ItemCandidates{
		{ItemIndex: 0, Candidates: []vision.Candidate{{Description: "a"}, {Description: "b"}}},
		{ItemIndex: 1, Candidates: []vision.Candidate{{Description: "c"}}},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(result.Selections) != 2 {
		t.Fatalf("expected 2 selections, got %+v", result.Selections)
	}
	if result.Selections[0].CandidateIndex != 1 {
		t.Errorf("expected candidate_index 1, got %d", result.Selections[0].CandidateIndex)
	}
	if result.Selections[1].CandidateIndex != -1 {
		t.Errorf("expected candidate_index -1 (no match), got %d", result.Selections[1].CandidateIndex)
	}
}

func TestOpenAIClient_APIErrorIsReturned(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`)) //nolint:errcheck
	})

	_, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "")
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}

func TestOpenAIClient_ContextCancellationReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Recognize(ctx, []byte{1}, "image/jpeg", "")
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}
