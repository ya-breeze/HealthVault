package server

import (
	"context"
	"strings"
	"time"

	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
	"github.com/ya-breeze/healthvault/pkg/weather"
)

// selectProviders keeps disposable E2E completely local, including when a
// stray API key is present. Ordinary runtime selection is unchanged.
func selectProviders(cfg *config.Config) (vision.Client, weather.Provider) {
	if cfg.DisposableE2E {
		return disposableVision{}, disposableWeather{}
	}
	var client vision.Client = vision.Unconfigured{}
	if cfg.OpenAIAPIKey != "" {
		client = vision.NewOpenAIClient(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	}
	return client, weather.NewOpenMeteo()
}

type disposableWeather struct{}

func (disposableWeather) Fetch(_ context.Context, hour database.WeatherHour) (database.WeatherHour, error) {
	hour.TemperatureC = 18
	hour.ApparentTemperatureC = 17
	hour.RelativeHumidityPercent = 60
	hour.SurfacePressureHpa = 1013
	hour.MeanSeaLevelPressureHpa = 1015
	hour.PrecipitationMm = 0
	hour.WindSpeedKmh = 8
	hour.Source = "open-meteo historical-forecast (synthetic)"
	hour.Model = "synthetic-ecmwf-ifs025"
	hour.FetchedAt = time.Now().UTC()
	hour.Status = "ready"
	return hour, nil
}

type disposableVision struct{}

func disposableRecognize(language string) *vision.RecognizeResult {
	name, canonical := "Soup", ""
	if strings.HasPrefix(strings.ToLower(language), "ru") {
		name, canonical = "борщ", "borscht"
	}
	return &vision.RecognizeResult{Items: []vision.Item{{
		Name: name, CanonicalName: canonical, WeightGrams: 250, Confidence: 0.9,
		EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 60, ProteinPer100g: 3, CarbsPer100g: 7, FatPer100g: 2},
	}}, Model: "synthetic-local"}
}

func (disposableVision) Recognize(_ context.Context, _ []byte, _, _, language string) (*vision.RecognizeResult, error) {
	return disposableRecognize(language), nil
}

func (disposableVision) Describe(_ context.Context, _, language string) (*vision.RecognizeResult, error) {
	return disposableRecognize(language), nil
}

func (disposableVision) Clarify(_ context.Context, _ string, _ []vision.Item, _ []vision.ClarifyTurn, language string) (*vision.RecognizeResult, error) {
	return disposableRecognize(language), nil
}

func (disposableVision) EstimateWeights(_ context.Context, _ []byte, _ string, components []vision.WeightEstimateInput) (*vision.WeightEstimateResult, error) {
	result := &vision.WeightEstimateResult{Model: "synthetic-local"}
	for _, component := range components {
		result.Estimates = append(result.Estimates, vision.WeightEstimate{ComponentIndex: component.ComponentIndex, WeightGrams: 100})
	}
	return result, nil
}

func (disposableVision) Select(_ context.Context, candidates []vision.ItemCandidates) (*vision.SelectResult, error) {
	result := &vision.SelectResult{Model: "synthetic-local"}
	for _, item := range candidates {
		result.Selections = append(result.Selections, vision.Selection{ItemIndex: item.ItemIndex, CandidateIndex: -1})
	}
	return result, nil
}

func (disposableVision) Translate(_ context.Context, query string) (string, error) {
	return query, nil
}

func (disposableVision) Advise(_ context.Context, _ vision.AdviceInput) ([]string, error) {
	return []string{"Synthetic nutrition advice."}, nil
}

func (disposableVision) NutritionChat(_ context.Context, in vision.NutritionChatInput) (*vision.NutritionChatResult, error) {
	return &vision.NutritionChatResult{Answer: "Synthetic answer to: " + in.Question, Model: "synthetic-local"}, nil
}

var _ vision.Client = disposableVision{}
var _ weather.Provider = disposableWeather{}
