package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/kin-core/auth"
)

type phoneDescribeInput struct {
	RequestID      string                       `json:"request_id" jsonschema:"Stable unique ID for this meal request. Reuse on transport retries; use a new ID for a different meal."`
	Description    string                       `json:"description"`
	Name           string                       `json:"name,omitempty"`
	LoggedAt       *time.Time                   `json:"logged_at,omitempty"`
	CookingContext *database.MealCookingContext `json:"cooking_context,omitempty"`
}

type phoneMealInput struct {
	MealID string `json:"meal_id"`
}
type phoneClarifyInput struct {
	MealID          string   `json:"meal_id"`
	ExpectedRound   int      `json:"expected_round"`
	ExpectedVersion string   `json:"expected_version"`
	Answers         []string `json:"answers"`
}
type phoneConfirmInput struct {
	MealID   string     `json:"meal_id"`
	Confirm  bool       `json:"confirm" jsonschema:"True only when the user authorizes saving the described meal and its portions."`
	LoggedAt *time.Time `json:"logged_at,omitempty"`
}

type phoneItem struct {
	Name        string  `json:"name"`
	WeightGrams float64 `json:"weight_grams"`
	MacroSource string  `json:"macro_source"`
}
type phoneMealResult struct {
	MealID          string             `json:"meal_id"`
	Status          string             `json:"status"`
	Saved           bool               `json:"saved"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	LoggedAt        time.Time          `json:"logged_at"`
	Items           []phoneItem        `json:"items"`
	Questions       []string           `json:"questions"`
	ExpectedRound   int                `json:"expected_round"`
	ExpectedVersion string             `json:"expected_version"`
	Totals          map[string]float64 `json:"totals"`
}

func phoneResult(meal *database.FoodMeal) phoneMealResult {
	// Draft totals are a preview of analyzed items, not committed daily totals.
	preview := *meal
	if meal.Status != database.MealStatusConfirmed {
		preview.Aggregate(meal.Items)
	}
	out := phoneMealResult{
		MealID: meal.ID.String(), Status: meal.Status, Saved: meal.Status == database.MealStatusConfirmed,
		Name: meal.Name, Description: meal.Description, LoggedAt: meal.LoggedAt,
		Items: []phoneItem{}, Questions: []string{}, ExpectedRound: meal.ClarifyRound + 1,
		ExpectedVersion: meal.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Totals: map[string]float64{"calories": preview.Calories, "protein_grams": preview.ProteinGrams,
			"carbs_grams": preview.CarbsGrams, "fat_grams": preview.FatGrams, "sodium_grams": preview.SodiumGrams,
			"dietary_fiber_grams": preview.DietaryFiberGrams},
	}
	for _, it := range meal.Items {
		out.Items = append(out.Items, phoneItem{it.Name, it.WeightGrams, it.MacroSource})
	}
	var entries []database.ClarifyEntry
	if json.Unmarshal([]byte(meal.ClarifyLog), &entries) == nil && meal.Status == database.MealStatusPendingClarification {
		for _, e := range entries {
			if e.Round == out.ExpectedRound && e.Answer == "" {
				out.Questions = append(out.Questions, e.Question)
			}
		}
	}
	return out
}

func phoneError(err error) (*mcp.CallToolResult, phoneMealResult, error) {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, phoneMealResult{Items: []phoneItem{}, Questions: []string{}, Totals: map[string]float64{}}, nil
}

// phoneResponse adapts existing HTTP food handlers without loopback requests.
type phoneResponse struct {
	headers http.Header
	status  int
	bytes.Buffer
}

func (w *phoneResponse) Header() http.Header { return w.headers }
func (w *phoneResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *phoneResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.Buffer.Write(p)
}

func (h *foodHandlers) invokePhoneFood(ctx context.Context, claims *auth.Claims, method, id string, body any, handler http.HandlerFunc) (*database.FoodMeal, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("invalid meal input")
	}
	r, err := http.NewRequestWithContext(context.WithValue(ctx, claimsKey, claims), method, "/api/food/meals", bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("invalid meal request")
	}
	r = mux.SetURLVars(r, map[string]string{"id": id})
	w := &phoneResponse{headers: make(http.Header)}
	handler(w, r)
	if w.status < 200 || w.status >= 300 {
		return nil, fmt.Errorf("meal operation HTTP %d: %s", w.status, strings.TrimSpace(w.String()))
	}
	var meal database.FoodMeal
	if err := json.Unmarshal(w.Bytes(), &meal); err != nil {
		return nil, errors.New("invalid meal response")
	}
	return &meal, nil
}

// PhoneMCPHandler grants a private connection access to one configured UUID.
// It never accepts a user selector or exposes the administrative MCP tools.
func (h *foodHandlers) PhoneMCPHandler(token, userID string, barrier *sync.RWMutex) http.Handler {
	id, err := uuid.Parse(userID)
	if token == "" || err != nil || id == uuid.Nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "phone MCP not configured", http.StatusServiceUnavailable)
		})
	}
	if barrier == nil {
		barrier = &sync.RWMutex{}
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "healthvault-food", Version: "1.1.0"}, &mcp.ServerOptions{Instructions: "Record meals only for the connected HealthVault user. describe_food_meal creates a draft. " +
		"Keep request_id unchanged on retries. Read saved and status; only confirmed means saved in daily totals. Draft totals preview analyzed items, and missing macros contribute no known nutrients. " +
		"Ask the returned clarification questions and pass expected_round and expected_version unchanged. Show names and portions before confirming when they are uncertain. " +
		"An explicit user request to record a clearly specified meal authorizes confirmation; never invent missing answers. " +
		"Use get_food_meal after interrupted calls; retry_food_meal recovers failed or stale processing meals without creating another row. " +
		"For questions about yesterday or a period, use list_food_meals and get_food_daily_totals instead of chat memory. Follow next_cursor until empty for a complete meal list. " +
		"Report resolved dates and timezone, confirmed totals and coverage. These are recorded-food totals, not proof of everything eaten. Unknown nutrients are not measured zeros; estimates are not measurements. " +
		"An empty day means no recorded food, not fasting. Today is partial. Completeness is a logging heuristic or owner assertion, never proof of complete intake. Sodium grams are elemental sodium, not added salt."})
	run := func(ctx context.Context, operation func(*auth.Claims) (*database.FoodMeal, error)) (*mcp.CallToolResult, phoneMealResult, error) {
		// The outer stream is exempt from capture locking. Every tool, including
		// identity resolution and replay reads, takes the shared lock here.
		barrier.RLock()
		defer barrier.RUnlock()
		user, err := h.storage.FindUserByID(id)
		if err != nil {
			return phoneError(errors.New("connected user unavailable"))
		}
		claims := &auth.Claims{UserID: user.ID, FamilyID: &user.FamilyID}
		meal, err := operation(claims)
		if err != nil {
			return phoneError(err)
		}
		// Shared HTTP handlers may return their pre-update timestamp after
		// analysis. Read the committed row for the next question version.
		meal, err = h.loadOwnedMeal(meal.ID, claims.UserID)
		if err != nil {
			return phoneError(errors.New("meal no longer available; reload its state"))
		}
		out := phoneResult(meal)
		encoded, err := json.Marshal(out)
		if err != nil || len(encoded) > 64*1024 {
			return phoneError(errors.New("meal result too large; review this meal in HealthVault"))
		}
		return nil, out, nil
	}
	additive, closed := false, false
	write := &mcp.ToolAnnotations{DestructiveHint: &additive, OpenWorldHint: &closed}
	mcp.AddTool(s, &mcp.Tool{Name: "describe_food_meal", Description: "Create a meal draft from the user's description. request_id is required and must stay identical on retries. Returns the actual status and questions.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &additive, OpenWorldHint: &closed, IdempotentHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneDescribeInput) (*mcp.CallToolResult, phoneMealResult, error) {
		if strings.TrimSpace(in.RequestID) == "" {
			return phoneError(errors.New("request_id is required"))
		}
		return run(ctx, func(c *auth.Claims) (*database.FoodMeal, error) {
			return h.invokePhoneFood(ctx, c, http.MethodPost, "", in, h.CreateDescribedMeal)
		})
	})
	mcp.AddTool(s, &mcp.Tool{Name: "get_food_meal", Description: "Read an owned meal's current status, portions and pending questions.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneMealInput) (*mcp.CallToolResult, phoneMealResult, error) {
		return run(ctx, func(c *auth.Claims) (*database.FoodMeal, error) {
			return h.invokePhoneFood(ctx, c, http.MethodGet, in.MealID, nil, h.GetMeal)
		})
	})
	mcp.AddTool(s, &mcp.Tool{Name: "clarify_food_meal", Description: "Answer returned questions in order with their expected_round and expected_version. Replays of an already answered version return the current meal; changed answers require review.", Annotations: write}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneClarifyInput) (*mcp.CallToolResult, phoneMealResult, error) {
		return run(ctx, func(c *auth.Claims) (*database.FoodMeal, error) {
			if in.ExpectedRound < 1 || in.ExpectedVersion == "" {
				return nil, errors.New("expected_round must be positive and expected_version is required")
			}
			meal, err := h.ownedPhoneMeal(in.MealID, c.UserID)
			if err != nil {
				return nil, err
			}
			if phoneAnswersApplied(meal, in.ExpectedRound, in.ExpectedVersion, in.Answers) {
				return meal, nil
			}
			meal, err = h.invokePhoneFood(ctx, c, http.MethodPost, in.MealID, in, h.ClarifyMeal)
			if err != nil {
				current, loadErr := h.ownedPhoneMeal(in.MealID, c.UserID)
				if loadErr == nil && phoneAnswersApplied(current, in.ExpectedRound, in.ExpectedVersion, in.Answers) {
					return current, nil
				}
			}
			return meal, err
		})
	})
	mcp.AddTool(s, &mcp.Tool{Name: "confirm_food_meal", Description: "Confirm an owned reviewed meal after the user authorizes saving. confirm must be true. An identical confirmation replay returns the saved meal.", Annotations: &mcp.ToolAnnotations{DestructiveHint: &additive, OpenWorldHint: &closed, IdempotentHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneConfirmInput) (*mcp.CallToolResult, phoneMealResult, error) {
		if !in.Confirm {
			return phoneError(errors.New("confirm must be true after user authorization"))
		}
		return run(ctx, func(c *auth.Claims) (*database.FoodMeal, error) {
			meal, err := h.ownedPhoneMeal(in.MealID, c.UserID)
			if err != nil {
				return nil, err
			}
			if meal.Status == database.MealStatusConfirmed {
				return phoneConfirmedReplay(meal, in.LoggedAt)
			}
			meal, err = h.invokePhoneFood(ctx, c, http.MethodPut, in.MealID, in, h.ConfirmMeal)
			if err != nil {
				current, loadErr := h.ownedPhoneMeal(in.MealID, c.UserID)
				if loadErr == nil && current.Status == database.MealStatusConfirmed {
					return phoneConfirmedReplay(current, in.LoggedAt)
				}
			}
			return meal, err
		})
	})
	mcp.AddTool(s, &mcp.Tool{Name: "retry_food_meal", Description: "Recover failed or stale processing meal analysis in the existing row. Never creates a second meal. Read the meal first.", Annotations: write}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneMealInput) (*mcp.CallToolResult, phoneMealResult, error) {
		return run(ctx, func(c *auth.Claims) (*database.FoodMeal, error) {
			return h.invokePhoneFood(ctx, c, http.MethodPost, in.MealID, nil, h.RetryMeal)
		})
	})
	h.addPhoneHistoryTools(s, id, barrier)
	stream := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true})
	return requireBearerToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		stream.ServeHTTP(w, r)
	}))
}

func (h *foodHandlers) ownedPhoneMeal(id string, userID uuid.UUID) (*database.FoodMeal, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.New("invalid meal_id")
	}
	meal, err := h.loadOwnedMeal(parsed, userID)
	if err != nil {
		return nil, errors.New("meal not found or unavailable")
	}
	return meal, nil
}

func phoneAnswersApplied(meal *database.FoodMeal, round int, version string, answers []string) bool {
	if meal.ClarifyRound < round || len(answers) == 0 {
		return false
	}
	var entries []database.ClarifyEntry
	if json.Unmarshal([]byte(meal.ClarifyLog), &entries) != nil {
		return false
	}
	var applied []string
	for _, e := range entries {
		if e.Round == round {
			if e.RequestVersion != version {
				return false
			}
			applied = append(applied, e.Answer)
		}
	}
	if len(applied) != len(answers) {
		return false
	}
	for i, a := range applied {
		if a == "" || a != answers[i] {
			return false
		}
	}
	return true
}

func phoneConfirmedReplay(meal *database.FoodMeal, loggedAt *time.Time) (*database.FoodMeal, error) {
	if loggedAt != nil && !meal.LoggedAt.Equal(*loggedAt) {
		return nil, errors.New("meal already confirmed with a different timestamp")
	}
	return meal, nil
}
