package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func healthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Open(slog.New(slog.NewTextHandler(io.Discard, nil)), filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

var healthFixtureTime = time.Date(2026, 11, 2, 12, 0, 0, 123456789, time.UTC)

// Seed independent model fixtures, rather than generating tables or records from
// the catalog under test. Every projected column must exist in the actual schema.
func healthFixture(t *testing.T, db *gorm.DB, row any, user, family uuid.UUID) uuid.UUID {
	t.Helper()
	v := reflect.ValueOf(row).Elem()
	id := uuid.New()
	if f := v.FieldByName("ID"); f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(id))
	}
	if f := v.FieldByName("UserID"); f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(user))
	}
	if f := v.FieldByName("FamilyID"); f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(family))
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		if f.Type() == reflect.TypeOf(time.Time{}) {
			f.Set(reflect.ValueOf(healthFixtureTime))
		}
	}
	if f := v.FieldByName("CreatedAt"); f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(healthFixtureTime))
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return v.FieldByName("ID").Interface().(uuid.UUID)
}

func TestHealthCatalogEveryDatasetAndExtension(t *testing.T) {
	db := healthTestDB(t)
	ctx := context.Background()
	user, family := uuid.New(), uuid.New()
	fixtures := map[string]any{
		"steps": &Steps{}, "distance": &Distance{}, "active_calories": &ActiveCalories{}, "total_calories": &TotalCalories{}, "hydration": &Hydration{},
		"heart_rate": &HeartRate{}, "heart_rate_variability": &HeartRateVariability{}, "weight": &Weight{}, "height": &Height{}, "weight_goal": &WeightGoal{},
		"blood_pressure": &BloodPressure{}, "blood_glucose": &BloodGlucose{}, "oxygen_saturation": &OxygenSaturation{}, "body_temperature": &BodyTemperature{}, "skin_temperature": &SkinTemperature{},
		"respiratory_rate": &RespiratoryRate{}, "resting_heart_rate": &RestingHeartRate{}, "basal_metabolic_rate": &BasalMetabolicRate{}, "body_fat": &BodyFat{}, "lean_body_mass": &LeanBodyMass{}, "vo2_max": &VO2Max{}, "bone_mass": &BoneMass{}, "speed": &Speed{}, "exercise": &Exercise{}, "nutrition": &Nutrition{},
		"custom_food": &CustomFood{Name: "fixture"}, "weather_location": &WeatherLocation{}, "weather_hour": &WeatherHour{Status: "ready"}, "weather_coverage": &WeatherCoverage{}, "webhook_import": &WebhookPayload{Raw: `{"future_type":123}`},
	}
	for _, row := range fixtures {
		healthFixture(t, db, row, user, family)
	}
	sleepID := healthFixture(t, db, &Sleep{}, user, family)
	healthFixture(t, db, &SleepStage{SleepID: sleepID, Stage: "REM"}, user, family)
	mealID := healthFixture(t, db, &FoodMeal{Name: "meal", Description: "description", CookingContext: &MealCookingContext{LowAddedSalt: true}, RawResponse: "SECRET_RAW_MODEL", PhotoPath: "SECRET_PATH", RequestFingerprint: "SECRET_FINGERPRINT", ClarifyLog: "SECRET_LOG"}, user, family)
	healthFixture(t, db, &FoodItem{MealID: mealID, Name: "item", MacroSource: "estimated", HasEstimate: true, EstimatedCaloriesPer100g: 123.4, HistoryOriginalSource: "reference", HistoryOriginalConfidence: 0.8}, user, family)
	fixtures["sleep"], fixtures["sleep_stage"], fixtures["food_meal"], fixtures["food_item"] = nil, nil, nil, nil
	catalog, err := HealthDataTypes(ctx, db, user, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(fixtures) {
		t.Fatalf("catalog=%d fixture types=%d", len(catalog), len(fixtures))
	}
	encoded, _ := json.Marshal(catalog)
	if len(encoded) > 48*1024 {
		t.Fatalf("catalog too large: %d", len(encoded))
	}
	for _, d := range catalog {
		if d.RecordCount != 1 || d.FirstTime == nil || d.LastTime == nil {
			t.Fatalf("%s availability: %+v", d.Name, d)
		}
		p, err := QueryHealthData(ctx, db, user, family, HealthDataQuery{Type: d.Name})
		if err != nil || len(p.Records) != 1 {
			t.Fatalf("%s query: %+v %v", d.Name, p, err)
		}
		b, _ := json.Marshal(p)
		if strings.Contains(string(b), "SECRET_") {
			t.Fatalf("%s leaked excluded field", d.Name)
		}
		if d.Name == "food_meal" && !strings.Contains(string(b), "low_added_salt") {
			t.Fatal("meal cooking context missing")
		}
		if d.Name == "food_item" && (!strings.Contains(string(b), "123.4") || !strings.Contains(string(b), "reference")) {
			t.Fatal("stored nutrition estimate or source missing")
		}
		if _, ok := fixtures[d.Name]; !ok {
			t.Fatalf("unexpected dataset %s", d.Name)
		}
	}
	// One new registration serves both discovery and reads without another tool.
	if err := db.Exec("CREATE TABLE future_health (id TEXT PRIMARY KEY,user_id TEXT,family_id TEXT,time DATETIME,value REAL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO future_health VALUES (?,?,?,?,?)", uuid.New(), user, family, healthFixtureTime, 42).Error; err != nil {
		t.Fatal(err)
	}
	healthDatasets["future_health"] = newHealthDataset("future_health", "future_health", "time", "value:unit", "Synthetic extension", false)
	defer delete(healthDatasets, "future_health")
	catalog, err = HealthDataTypes(ctx, db, user, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(fixtures)+1 {
		t.Fatal("extension absent from discovery")
	}
	p, err := QueryHealthData(ctx, db, user, family, HealthDataQuery{Type: "future_health"})
	if err != nil || len(p.Records) != 1 {
		t.Fatalf("extension unreadable: %+v %v", p, err)
	}
}

func TestHealthImportLargeStoredPayloadBoundary(t *testing.T) {
	db := healthTestDB(t)
	u, f := uuid.New(), uuid.New()
	ctx := context.Background()
	prefix, suffix := `{"future": "`, `"}`
	raw := prefix + strings.Repeat("x", (20<<20)-len(prefix)-len(suffix)) + suffix
	id := healthFixture(t, db, &WebhookPayload{Raw: raw}, u, f)
	first, err := GetHealthImport(ctx, db, u, f, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	last, err := GetHealthImport(ctx, db, u, f, id, len(raw)-healthChunkBytes)
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalBytes != 20<<20 || !first.HasMore || last.HasMore || last.NextOffset != len(raw) || first.SHA256 != last.SHA256 || !strings.HasSuffix(last.Data, suffix) {
		t.Fatal("maximum payload boundary incomplete")
	}
	if err := db.Model(&WebhookPayload{}).Where("id = ?", id).Update("raw", raw+" ").Error; err != nil {
		t.Fatal(err)
	}
	legacy, err := GetHealthImport(ctx, db, u, f, id, len(raw))
	if err != nil || legacy.TotalBytes != len(raw)+1 || legacy.Data != " " || legacy.HasMore {
		t.Fatal("stored import beyond current upload limit must remain completely readable")
	}
}

func TestHealthReadsExactChronologyCursorAndIdentity(t *testing.T) {
	db := healthTestDB(t)
	ctx := context.Background()
	user, family := uuid.New(), uuid.New()
	base := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	// Lexical ordering and SQLite fractional-second rounding both fail this set.
	stamps := []time.Time{base.Add(999999999), base.Add(1), base.Add(999999998), base.Add(-time.Nanosecond).In(time.FixedZone("offset", 7200))}
	for i, stamp := range stamps {
		r := &HeartRate{Time: stamp, BPM: i + 1}
		r.ID = uuid.New()
		r.UserID = user
		r.FamilyID = family
		if err := db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	r := &HeartRate{Time: base.Add(time.Hour), BPM: 999}
	r.ID = uuid.New()
	r.UserID = uuid.New()
	r.FamilyID = family
	if err := db.Create(r).Error; err != nil {
		t.Fatal(err)
	}
	r = &HeartRate{Time: base.Add(2 * time.Hour), BPM: 999}
	r.ID = uuid.New()
	r.UserID = user
	r.FamilyID = uuid.New()
	if err := db.Create(r).Error; err != nil {
		t.Fatal(err)
	}
	r = &HeartRate{Time: base.Add(3 * time.Hour), BPM: 999}
	r.ID = uuid.New()
	r.UserID = user
	r.FamilyID = family
	if err := db.Create(r).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(r).Error; err != nil {
		t.Fatal(err)
	}
	in := HealthDataQuery{Type: "heart_rate", Limit: 1}
	var values []int64
	for {
		p, err := QueryHealthData(ctx, db, user, family, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Records) != 1 {
			t.Fatalf("unexpected records %+v", p)
		}
		values = append(values, p.Records[0]["bpm"].(int64))
		if !p.HasMore {
			break
		}
		in.Cursor = p.NextCursor
		if len(values) > 4 {
			t.Fatal("pagination repeated records")
		}
	}
	if fmt.Sprint(values) != "[4 2 3 1]" {
		t.Fatalf("chronology %v", values)
	}
	first, err := QueryHealthData(ctx, db, user, family, HealthDataQuery{Type: "heart_rate", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		u, f uuid.UUID
		q    HealthDataQuery
	}{
		{uuid.New(), family, HealthDataQuery{Type: "heart_rate", Cursor: first.NextCursor}},
		{user, uuid.New(), HealthDataQuery{Type: "heart_rate", Cursor: first.NextCursor}},
		{user, family, HealthDataQuery{Type: "weight", Cursor: first.NextCursor}},
		{user, family, HealthDataQuery{Type: "heart_rate", From: base, Cursor: first.NextCursor}},
	} {
		if _, err := QueryHealthData(ctx, db, tc.u, tc.f, tc.q); err == nil {
			t.Fatal("accepted cross-scope cursor")
		}
	}
	p, err := QueryHealthData(ctx, db, user, family, HealthDataQuery{Type: "heart_rate", From: base.Add(time.Nanosecond), To: base.Add(999999999)})
	if err != nil || len(p.Records) != 2 {
		t.Fatalf("nanosecond half-open range %+v %v", p, err)
	}
}

func TestHealthChildOwnershipAndReadyWeather(t *testing.T) {
	db := healthTestDB(t)
	ctx := context.Background()
	u, f := uuid.New(), uuid.New()
	for _, owner := range []uuid.UUID{u, uuid.New()} {
		s := &Sleep{}
		id := healthFixture(t, db, s, owner, f)
		healthFixture(t, db, &SleepStage{SleepID: id}, u, f)
		if owner == u {
			if err := db.Delete(s).Error; err != nil {
				t.Fatal(err)
			}
		}
		m := &FoodMeal{}
		mid := healthFixture(t, db, m, owner, f)
		healthFixture(t, db, &FoodItem{MealID: mid}, u, f)
		if owner == u {
			if err := db.Delete(m).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	healthFixture(t, db, &WeatherHour{Status: "pending"}, u, f)
	for _, name := range []string{"sleep_stage", "food_item", "weather_hour"} {
		p, err := QueryHealthData(ctx, db, u, f, HealthDataQuery{Type: name})
		if err != nil || len(p.Records) != 0 {
			t.Fatalf("%s leaked %+v %v", name, p, err)
		}
	}
}

func TestHealthChunksPreserveLargeImportsAndRecords(t *testing.T) {
	db := healthTestDB(t)
	ctx := context.Background()
	u, f := uuid.New(), uuid.New()
	raw := `{"unknown_future": "` + strings.Repeat("Ж🌍", 20000) + `"}`
	id := healthFixture(t, db, &WebhookPayload{Raw: raw}, u, f)
	var got strings.Builder
	offset := 0
	expectedHash := sha256.Sum256([]byte(raw))
	for {
		c, err := GetHealthImport(ctx, db, u, f, id, offset)
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(c.Data) || len(c.Data) > healthChunkBytes || c.TotalBytes != len(raw) || c.SHA256 != hex.EncodeToString(expectedHash[:]) {
			t.Fatalf("invalid chunk %+v", c)
		}
		got.WriteString(c.Data)
		offset = c.NextOffset
		if !c.HasMore {
			break
		}
	}
	if got.String() != raw {
		t.Fatal("raw import lost data")
	}
	if _, err := GetHealthImport(ctx, db, uuid.New(), f, id, 0); err == nil {
		t.Fatal("cross-user raw import exposed")
	}
	if _, err := GetHealthImport(ctx, db, u, uuid.New(), id, 0); err == nil {
		t.Fatal("cross-family raw import exposed")
	}
	if _, err := GetHealthImport(ctx, db, u, f, id, 21); err == nil {
		t.Fatal("split UTF8 offset accepted")
	}
	mealID := healthFixture(t, db, &FoodMeal{Name: "oversized", Description: raw, PhotoPath: "SECRET_PATH"}, u, f)
	p, err := QueryHealthData(ctx, db, u, f, HealthDataQuery{Type: "food_meal"})
	if err != nil || len(p.Records) != 1 || p.Records[0]["record_chunk_required"] != true {
		t.Fatalf("oversized reference %+v %v", p, err)
	}
	got.Reset()
	offset = 0
	for {
		c, err := GetHealthRecord(ctx, db, u, f, "food_meal", mealID, offset)
		if err != nil {
			t.Fatal(err)
		}
		got.WriteString(c.Data)
		offset = c.NextOffset
		if !c.HasMore {
			break
		}
	}
	var meal map[string]any
	if err := json.Unmarshal([]byte(got.String()), &meal); err != nil {
		t.Fatal(err)
	}
	if meal["description"] != raw || strings.Contains(got.String(), "SECRET_PATH") {
		t.Fatal("record chunk projection lost or leaked data")
	}
	if _, err := GetHealthRecord(ctx, db, uuid.New(), f, "food_meal", mealID, 0); err == nil {
		t.Fatal("cross-user record chunk")
	}
	if _, err := QueryHealthData(ctx, db, u, f, HealthDataQuery{Type: "users"}); err == nil {
		t.Fatal("operational table exposed")
	}
}
