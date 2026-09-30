package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Nutrition Chat has its own model policy, independent of photo recognition.
const nutritionChatModel = "gpt-6-luna"
const openAIResponsesURL = "https://api.openai.com/v1/responses"

type nutritionResponseTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Strict      bool   `json:"strict"`
	Parameters  any    `json:"parameters"`
}

type nutritionResponseRequest struct {
	Model     string `json:"model"`
	Input     []any  `json:"input"`
	Store     bool   `json:"store"`
	Reasoning struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
	Include []string `json:"include"`
	Text    struct {
		Format struct {
			Type   string `json:"type"`
			Name   string `json:"name"`
			Strict bool   `json:"strict"`
			Schema any    `json:"schema"`
		} `json:"format"`
	} `json:"text"`
	Tools      []nutritionResponseTool `json:"tools,omitempty"`
	ToolChoice string                  `json:"tool_choice,omitempty"`
}

type nutritionResponse struct {
	Model  string            `json:"model"`
	Status string            `json:"status"`
	Output []json.RawMessage `json:"output"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Code string `json:"code"`
		Type string `json:"type"`
	} `json:"error"`
}

type nutritionOutputItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// NutritionChat replays the ephemeral conversation and executes only the
// caller-scoped history interface. Reasoning and tool output stay request-local.
func (c *OpenAIClient) NutritionChat(ctx context.Context, in NutritionChatInput) (*NutritionChatResult, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal nutrition chat input: %w", err)
	}
	req := nutritionResponseRequest{
		Model: nutritionChatModel,
		Input: []any{
			chatMessage{Role: "system", Content: nutritionChatSystemPrompt},
			chatMessage{Role: "user", Content: string(payload)},
		},
		Store: false, Include: []string{"reasoning.encrypted_content"},
	}
	req.Reasoning.Effort = "high"
	req.Text.Format.Type = "json_schema"
	req.Text.Format.Name = "nutrition_chat"
	req.Text.Format.Strict = true
	req.Text.Format.Schema = nutritionChatJSONSchema
	if in.HistoryTools != nil {
		for _, tool := range nutritionChatTools {
			req.Tools = append(req.Tools, nutritionResponseTool{
				Type: tool.Type, Name: tool.Function.Name, Description: tool.Function.Description,
				Strict: tool.Function.Strict, Parameters: tool.Function.Parameters,
			})
		}
		req.ToolChoice = "auto"
	}
	totalInput, totalOutput := 0, 0
	totalLatency := time.Duration(0)
	callsUsed := 0
	callIDs := make(map[string]bool, nutritionChatMaxToolCalls)
	for {
		resp, latency, err := c.callNutritionResponse(ctx, req)
		if err != nil {
			return nil, err
		}
		totalInput += resp.Usage.InputTokens
		totalOutput += resp.Usage.OutputTokens
		totalLatency += latency

		var calls []nutritionOutputItem
		var answer strings.Builder
		for _, raw := range resp.Output {
			var item nutritionOutputItem
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, fmt.Errorf("decode nutrition chat output: %w", err)
			}
			switch item.Type {
			case "function_call":
				calls = append(calls, item)
			case "message":
				for _, content := range item.Content {
					if content.Type == "refusal" {
						return nil, fmt.Errorf("nutrition chat response refused")
					}
					if content.Type == "output_text" {
						answer.WriteString(content.Text)
					}
				}
			}
		}
		if len(calls) == 0 {
			return parseNutritionChatAnswer(answer.String(), resp.Model, totalInput, totalOutput, totalLatency)
		}
		if in.HistoryTools == nil || callsUsed+len(calls) > nutritionChatMaxToolCalls {
			return nil, fmt.Errorf("nutrition chat exceeded tool call limit")
		}
		// Preserve the whole output, including encrypted reasoning, before
		// appending function results. Never retain it across user requests.
		for _, raw := range resp.Output {
			req.Input = append(req.Input, raw)
		}
		for _, call := range calls {
			if call.CallID == "" || callIDs[call.CallID] || call.Name == "" {
				return nil, fmt.Errorf("nutrition chat returned an invalid tool call")
			}
			callIDs[call.CallID] = true
			result, execErr := in.HistoryTools.Execute(ctx, call.Name, json.RawMessage(call.Arguments))
			if execErr != nil {
				if !errors.Is(execErr, ErrInvalidNutritionChatToolCall) {
					return nil, fmt.Errorf("nutrition chat history tool failed: %w", execErr)
				}
				result = json.RawMessage(`{"available":false,"reason":"invalid_request"}`)
			}
			if !json.Valid(result) {
				return nil, fmt.Errorf("nutrition chat history tool returned invalid JSON")
			}
			req.Input = append(req.Input, map[string]string{
				"type": "function_call_output", "call_id": call.CallID, "output": string(result),
			})
			callsUsed++
		}
	}
}

func (c *OpenAIClient) callNutritionResponse(ctx context.Context, input nutritionResponseRequest) (*nutritionResponse, time.Duration, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal nutrition chat request: %w", err)
	}
	url := c.ResponsesURL
	if url == "" {
		url = openAIResponsesURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build nutrition chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	// HTTP status remains authoritative even if the provider sends HTML,
	// malformed JSON, or a body that cannot be read completely.
	if resp.StatusCode != http.StatusOK {
		var failure nutritionResponse
		_ = json.Unmarshal(body, &failure)
		code, errorType := "", ""
		if failure.Error != nil {
			code, errorType = failure.Error.Code, failure.Error.Type
		}
		return nil, latency, nutritionChatProviderError(resp.StatusCode, code, errorType)
	}
	if err != nil {
		return nil, latency, fmt.Errorf("read nutrition chat response: %w", err)
	}
	var result nutritionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, latency, fmt.Errorf("decode nutrition chat response: %w", err)
	}
	if result.Error != nil {
		return nil, latency, nutritionChatProviderError(resp.StatusCode, result.Error.Code, result.Error.Type)
	}
	if result.Status != "completed" {
		return nil, latency, fmt.Errorf("nutrition chat response did not complete")
	}
	return &result, latency, nil
}
