package server

import (
	"context"
	"testing"
	"time"

	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
	"github.com/ya-breeze/healthvault/pkg/weather"
)

func TestDisposableProvidersNeverSelectNetworkClients(t *testing.T) {
	visionClient, weatherProvider := selectProviders(&config.Config{
		DisposableE2E: true,
		OpenAIAPIKey:  "a-key-must-not-enable-network-calls",
	})
	if _, ok := visionClient.(disposableVision); !ok {
		t.Fatalf("disposable vision client: %T", visionClient)
	}
	if _, ok := weatherProvider.(disposableWeather); !ok {
		t.Fatalf("disposable weather provider: %T", weatherProvider)
	}

	english, err := visionClient.Describe(context.Background(), "soup", "en")
	if err != nil || len(english.Items) != 1 || english.Items[0].Name == "" {
		t.Fatalf("English description: %+v, %v", english, err)
	}
	russian, err := visionClient.Describe(context.Background(), "борщ", "ru")
	if err != nil || len(russian.Items) != 1 || russian.Items[0].CanonicalName == "" || russian.Items[0].Name == russian.Items[0].CanonicalName {
		t.Fatalf("Russian description: %+v, %v", russian, err)
	}
	photo, err := visionClient.Recognize(context.Background(), []byte("photo"), "image/jpeg", "", "en")
	if err != nil || len(photo.Items) != 1 {
		t.Fatalf("photo recognition: %+v, %v", photo, err)
	}
	clarified, err := visionClient.Clarify(context.Background(), "soup", nil, nil, "ru")
	if err != nil || len(clarified.Items) != 1 || clarified.Items[0].CanonicalName == "" {
		t.Fatalf("clarification: %+v, %v", clarified, err)
	}
	weights, err := visionClient.EstimateWeights(context.Background(), nil, "image/jpeg", []vision.WeightEstimateInput{{ComponentIndex: 2, Name: "rice"}})
	if err != nil || len(weights.Estimates) != 1 || weights.Estimates[0].ComponentIndex != 2 {
		t.Fatalf("weight estimate: %+v, %v", weights, err)
	}
	selected, err := visionClient.Select(context.Background(), []vision.ItemCandidates{{ItemIndex: 3}})
	if err != nil || len(selected.Selections) != 1 || selected.Selections[0].CandidateIndex != -1 {
		t.Fatalf("candidate selection: %+v, %v", selected, err)
	}
	translated, err := visionClient.Translate(context.Background(), "soup")
	if err != nil || translated != "soup" {
		t.Fatalf("translation: %q, %v", translated, err)
	}
	advice, err := visionClient.Advise(context.Background(), vision.AdviceInput{})
	if err != nil || len(advice) == 0 {
		t.Fatalf("advice: %v, %v", advice, err)
	}
	chat, err := visionClient.NutritionChat(context.Background(), vision.NutritionChatInput{Question: "Why?"})
	if err != nil || chat.Answer == "" {
		t.Fatalf("nutrition chat: %+v, %v", chat, err)
	}

	hour := database.WeatherHour{Hour: time.Now().UTC().Truncate(time.Hour)}
	weatherResult, err := weatherProvider.Fetch(context.Background(), hour)
	if err != nil || weatherResult.Status != "ready" || weatherResult.Source == "" || weatherResult.Model == "" || weatherResult.FetchedAt.IsZero() {
		t.Fatalf("weather enrichment: %+v, %v", weatherResult, err)
	}
}

func TestOrdinaryProvidersRemainUnchanged(t *testing.T) {
	withoutKey, ordinaryWeather := selectProviders(&config.Config{})
	if _, ok := withoutKey.(vision.Unconfigured); !ok {
		t.Fatalf("ordinary vision without key: %T", withoutKey)
	}
	if _, ok := ordinaryWeather.(*weather.OpenMeteo); !ok {
		t.Fatalf("ordinary weather provider: %T", ordinaryWeather)
	}
	withKey, _ := selectProviders(&config.Config{OpenAIAPIKey: "not-used-by-this-test"})
	if _, ok := withKey.(*vision.OpenAIClient); !ok {
		t.Fatalf("ordinary vision with key: %T", withKey)
	}
}
