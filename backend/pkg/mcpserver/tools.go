package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
)

// typeTimeCol maps type name to [2]string{table, timeCol}.
// Keep in sync with typeRegistry in server/api.go.
var typeTimeCol = map[string][2]string{
	"steps":                  {"steps", "start_time"},
	"heart_rate":             {"heart_rates", "time"},
	"heart_rate_variability": {"heart_rate_variabilities", "time"},
	"sleep":                  {"sleeps", "start_time"},
	"distance":               {"distances", "start_time"},
	"active_calories":        {"active_calories", "start_time"},
	"total_calories":         {"total_calories", "start_time"},
	"weight":                 {"weights", "time"},
	"height":                 {"heights", "time"},
	"blood_pressure":         {"blood_pressures", "time"},
	"blood_glucose":          {"blood_glucoses", "time"},
	"oxygen_saturation":      {"oxygen_saturations", "time"},
	"body_temperature":       {"body_temperatures", "time"},
	"skin_temperature":       {"skin_temperatures", "time"},
	"respiratory_rate":       {"respiratory_rates", "time"},
	"resting_heart_rate":     {"resting_heart_rates", "time"},
	"exercise":               {"exercises", "start_time"},
	"hydration":              {"hydrations", "start_time"},
	"nutrition":              {"nutritions", "start_time"},
	"basal_metabolic_rate":   {"basal_metabolic_rates", "time"},
	"body_fat":               {"body_fats", "time"},
	"lean_body_mass":         {"lean_body_masses", "time"},
	"vo2_max":                {"vo2_maxes", "time"},
	"bone_mass":              {"bone_masses", "time"},
}

type queryInput struct {
	User string `json:"user"`
	Type string `json:"type"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type summaryInput struct {
	User string `json:"user"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

func toolText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func toolError(msg string) *mcp.CallToolResult {
	r := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
	return r
}

func parseTR(from, to string) database.TimeRange {
	f, _ := time.Parse(time.RFC3339, from)
	t, _ := time.Parse(time.RFC3339, to)
	if f.IsZero() {
		f = time.Now().UTC().AddDate(0, 0, -7)
	}
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return database.TimeRange{From: f, To: t}
}

func registerTools(server *mcp.Server, storage database.Storage) {
	// list_users — returns all usernames
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_users",
		Description: "List all users in the system.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		users, err := storage.AllUsers()
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		type userInfo struct {
			Username string `json:"username"`
			ID       string `json:"id"`
		}
		out := make([]userInfo, len(users))
		for i, u := range users {
			out[i] = userInfo{Username: u.Username, ID: u.ID.String()}
		}
		b, _ := json.Marshal(out)
		return toolText(string(b)), nil, nil
	})

	// query_data — generic query by type name
	mcp.AddTool(server, &mcp.Tool{
		Name: "query_data",
		Description: "Query health records for a user and data type. " +
			"type must be one of: steps, heart_rate, heart_rate_variability, sleep, distance, " +
			"active_calories, total_calories, weight, height, blood_pressure, blood_glucose, " +
			"oxygen_saturation, body_temperature, skin_temperature, respiratory_rate, " +
			"resting_heart_rate, exercise, hydration, nutrition, basal_metabolic_rate, " +
			"body_fat, lean_body_mass, vo2_max, bone_mass.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user": map[string]any{"type": "string", "description": "username"},
				"type": map[string]any{"type": "string", "description": "data type name"},
				"from": map[string]any{"type": "string", "description": "RFC3339 start time"},
				"to":   map[string]any{"type": "string", "description": "RFC3339 end time"},
			},
			"required": []string{"user", "type"},
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input queryInput) (*mcp.CallToolResult, any, error) {
		cols, ok := typeTimeCol[input.Type]
		if !ok {
			return toolError(fmt.Sprintf("unknown type %q", input.Type)), nil, nil
		}
		user, err := storage.FindUserByName(input.User)
		if err != nil {
			return toolError(fmt.Sprintf("user %q not found", input.User)), nil, nil
		}
		records, err := storage.QueryRecords(cols[0], cols[1], user.ID, parseTR(input.From, input.To))
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		if records == nil {
			records = []map[string]any{}
		}
		b, _ := json.Marshal(records)
		return toolText(string(b)), nil, nil
	})

	// summary — aggregate steps, heart rate, sleep
	mcp.AddTool(server, &mcp.Tool{
		Name:        "summary",
		Description: "Get a health summary for a user: total steps, average heart rate, total sleep seconds.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user": map[string]any{"type": "string", "description": "username"},
				"from": map[string]any{"type": "string", "description": "RFC3339 start time"},
				"to":   map[string]any{"type": "string", "description": "RFC3339 end time"},
			},
			"required": []string{"user"},
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input summaryInput) (*mcp.CallToolResult, any, error) {
		user, err := storage.FindUserByName(input.User)
		if err != nil {
			return toolError(fmt.Sprintf("user %q not found", input.User)), nil, nil
		}
		tr := parseTR(input.From, input.To)
		steps, _ := storage.SummarySteps(user.ID, tr)
		avgHR, _ := storage.SummaryAvgHeartRate(user.ID, tr)
		sleepSec, _ := storage.SummarySleepSeconds(user.ID, tr)
		b, _ := json.Marshal(map[string]any{
			"user":           input.User,
			"from":           tr.From,
			"to":             tr.To,
			"steps":          steps,
			"avg_heart_rate": avgHR,
			"sleep_seconds":  sleepSec,
		})
		return toolText(string(b)), nil, nil
	})
}
