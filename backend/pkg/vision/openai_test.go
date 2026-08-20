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
			`{"items":[{"display_name":"chicken breast","canonical_name":"","preparation":"roasted","state":"cooked","weight_grams":180,"confidence":0.9}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{0xFF, 0xD8, 0xFF}, "image/jpeg", "", "en")
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

// Regression for a code-review finding: display_name went straight into
// Item.Name with no TrimSpace, unlike its CanonicalName/Brand siblings on the
// same struct literal — so incidental model whitespace could flow all the
// way to a save-as-custom-food CustomFood.Name, later tripping
// UpdateCustomFood's trimmed-comparison rename check as a false-positive.
func TestOpenAIClient_Recognize_TrimsDisplayNameWhitespace(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"display_name":"  chicken breast  ","canonical_name":"","preparation":"roasted","state":"cooked","weight_grams":180,"confidence":0.9}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{0xFF, 0xD8, 0xFF}, "image/jpeg", "", "en")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "chicken breast" {
		t.Fatalf("expected trimmed name %q, got %+v", "chicken breast", result.Items)
	}
}

func TestOpenAIClient_Recognize_EstimatedProfileParsed(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"display_name":"Mexican vegetable mix","canonical_name":"","preparation":"unknown","state":"unknown",`+
				`"weight_grams":200,"confidence":0.6,`+
				`"estimated_profile":{"calories_per_100g":90,"protein_per_100g":3,"carbs_per_100g":15,`+
				`"fat_per_100g":2,"sugar_per_100g":4,"sodium_per_100g":0.3,"dietary_fiber_per_100g":3}}],`+
				`"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	// The request's JSON schema must require estimated_profile (nullable, not
	// omittable) on every item — OpenAI's strict structured-output mode
	// requires every property to be listed in "required".
	schema := capturedBody["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
	itemSchema := schema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	required, _ := itemSchema["required"].([]any)
	var sawEstimatedProfile bool
	for _, r := range required {
		if r == "estimated_profile" {
			sawEstimatedProfile = true
		}
	}
	if !sawEstimatedProfile {
		t.Errorf("expected estimated_profile listed as required in the item schema, got %+v", required)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	p := result.Items[0].EstimatedProfile
	if p == nil {
		t.Fatal("expected a non-nil estimated profile")
	}
	if p.CaloriesPer100g != 90 || p.ProteinPer100g != 3 || p.CarbsPer100g != 15 ||
		p.FatPer100g != 2 || p.SugarPer100g != 4 || p.SodiumPer100g != 0.3 || p.DietaryFiberPer100g != 3 {
		t.Errorf("unexpected estimated profile: %+v", p)
	}
}

func TestOpenAIClient_Recognize_NullEstimatedProfileParsedAsNil(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"display_name":"unidentifiable item","canonical_name":"","preparation":"unknown","state":"unknown",`+
				`"weight_grams":50,"confidence":0.2,"estimated_profile":null}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if result.Items[0].EstimatedProfile != nil {
		t.Errorf("expected nil estimated profile for a null response, got %+v", result.Items[0].EstimatedProfile)
	}
}

func TestOpenAIClient_Recognize_UnknownMapsToEmptyString(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"display_name":"mystery sauce","canonical_name":"","preparation":"unknown","state":"unknown","weight_grams":30,"confidence":0.4}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
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

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
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
			`{"items":[{"display_name":"chicken breast","canonical_name":"","preparation":"roasted","state":"cooked","weight_grams":180,"confidence":0.9}],"clarification_questions":[]}`)))
	})

	_, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "this is chicken and rice, not berries", "en")
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

	_, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
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

func TestOpenAIClient_Recognize_NonEnglishKeepsDisplayAndCanonicalNames(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
		w.Write([]byte(chatResponse(t,                //nolint:errcheck
			`{"items":[{"display_name":"вареники","canonical_name":"dumplings","preparation":"boiled","state":"cooked","weight_grams":150,"confidence":0.8}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "ru")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if result.Items[0].Name != "вареники" || result.Items[0].CanonicalName != "dumplings" {
		t.Errorf("expected distinct display/canonical names, got %+v", result.Items[0])
	}

	b, _ := json.Marshal(capturedBody)
	if !strings.Contains(string(b), `\"ru\"`) {
		t.Errorf("expected the target language code in the request, got %s", b)
	}
}

func TestOpenAIClient_Recognize_EnglishDropsCanonicalNameEvenIfModelReturnsOne(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, //nolint:errcheck
			`{"items":[{"display_name":"dumplings","canonical_name":"dumplings","preparation":"boiled","state":"cooked","weight_grams":150,"confidence":0.8}],"clarification_questions":[]}`)))
	})

	result, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if result.Items[0].Name != "dumplings" || result.Items[0].CanonicalName != "" {
		t.Errorf("expected empty CanonicalName for English, got %+v", result.Items[0])
	}
}

func TestOpenAIClient_Clarify_SendsNoImageContent(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
		w.Write([]byte(chatResponse(t,                //nolint:errcheck
			`{"items":[{"display_name":"sauce","canonical_name":"","preparation":"unknown","state":"unknown","weight_grams":30,"confidence":0.6}],"clarification_questions":[]}`)))
	})

	_, err := c.Clarify(context.Background(),
		[]vision.Item{{Name: "sauce", WeightGrams: 30}},
		[]vision.ClarifyTurn{{Question: "Cream or tomato based?", Answer: "Tomato-based"}}, "en")
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

// Regression (design.md "Selection is offered the recognized item's own
// name and brand" / tasks.md 6.7): Select's payload previously carried only
// item_index and anonymous candidate descriptions — the recognized item's
// own name/brand was never sent, so the model had no idea what it was
// actually comparing candidates against. Confirms the marshaled request
// body carries item_name/item_brand, not just that the Go struct has the
// fields — a future refactor could still drop them from the JSON tags.
func TestOpenAIClient_Select_PayloadCarriesItemNameAndBrand(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
		w.Write([]byte(chatResponse(t,                //nolint:errcheck
			`{"selections":[{"item_index":0,"candidate_index":0}]}`)))
	})

	_, err := c.Select(context.Background(), []vision.ItemCandidates{
		{
			ItemIndex: 0, ItemName: "Bílý jogurt", ItemBrand: "Olma",
			Candidates: []vision.Candidate{{Description: "Bílý jogurt", Brands: "Olma"}},
		},
	})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	messages, _ := capturedBody["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %+v", messages)
	}
	userMsg, _ := messages[1].(map[string]any)
	content, _ := userMsg["content"].(string)
	if !strings.Contains(content, `"item_name":"Bílý jogurt"`) {
		t.Errorf("expected item_name in the Select payload, got %s", content)
	}
	if !strings.Contains(content, `"item_brand":"Olma"`) {
		t.Errorf("expected item_brand in the Select payload, got %s", content)
	}
}

func TestOpenAIClient_APIErrorIsReturned(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`)) //nolint:errcheck
	})

	_, err := c.Recognize(context.Background(), []byte{1}, "image/jpeg", "", "en")
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
	_, err := c.Recognize(ctx, []byte{1}, "image/jpeg", "", "en")
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestOpenAIClient_Translate_SendsQueryAndReturnsTerm(t *testing.T) {
	var capturedBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody) //nolint:errcheck
		w.Write([]byte(chatResponse(t,                //nolint:errcheck
			`{"translated_query":"oatmeal"}`)))
	})

	result, err := c.Translate(context.Background(), "овсянка")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if result != "oatmeal" {
		t.Errorf("expected \"oatmeal\", got %q", result)
	}

	messages, _ := capturedBody["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %+v", messages)
	}
	userMsg, _ := messages[1].(map[string]any)
	content, _ := userMsg["content"].(string)
	if content != "овсянка" {
		t.Errorf("expected the raw query as the user message, got %q", content)
	}
	if strings.Contains(string(mustMarshal(t, capturedBody)), "image_url") {
		t.Error("expected no image_url content in a Translate request")
	}
	if store, _ := capturedBody["store"].(bool); store {
		t.Error("expected store=false")
	}
}

func TestOpenAIClient_Translate_APIErrorIsReturned(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`)) //nolint:errcheck
	})

	_, err := c.Translate(context.Background(), "porridge")
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}

func TestOpenAIClient_Translate_MalformedResponseIsReturned(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatResponse(t, `not valid json`))) //nolint:errcheck
	})

	_, err := c.Translate(context.Background(), "porridge")
	if err == nil {
		t.Fatal("expected an error for malformed structured content")
	}
}

// Regression for a code-review finding: display_language is an unvalidated,
// caller-supplied opaque-settings string (only the frontend's own dropdown
// is constrained to exact "en"/"ru"), so a full BCP-47 tag like "en-US" from
// some other caller must still be recognized as English rather than
// silently gating out USDA/OFF matching and requesting a redundant
// canonical_name for a functionally-English user.
func TestIsEnglishDisplayLanguage(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"en", true},
		{"En", true},
		{"EN", true},
		{"en-US", true},
		{"en_GB", true},
		{"ru", false},
		{"ru-RU", false},
		{"english", false}, // not a valid BCP-47 primary subtag match for "en"
	}
	for _, c := range cases {
		if got := vision.IsEnglishDisplayLanguage(c.in); got != c.want {
			t.Errorf("IsEnglishDisplayLanguage(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
