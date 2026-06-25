package ingest

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const timeLayout = time.RFC3339Nano

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}

// Process fans out all type arrays from p into their respective tables.
// Records are upserted: on conflict with the (user_id, time) unique key the
// data values are overwritten with the incoming values, so re-sending the
// same payload is always idempotent.
// Sleep uses DO NOTHING because re-inserting its child stages is not safe
// without a unique key on the stage rows.
func Process(db *gorm.DB, userID, familyID, payloadID uuid.UUID, p *PayloadJSON) error {
	// Interval types keyed on (user_id, start_time)
	upsertInterval := clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "start_time"}},
		UpdateAll: true,
	}
	// Point-in-time types keyed on (user_id, time)
	upsertPoint := clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "time"}},
		UpdateAll: true,
	}
	// Sleep: skip duplicates rather than upsert (stages have no unique key)
	doNothing := clause.OnConflict{DoNothing: true}

	for _, r := range p.Steps {
		rec := &database.Steps{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Count: r.Count,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := db.Clauses(upsertInterval).Create(rec).Error; err != nil {
			return fmt.Errorf("upsert steps: %w", err)
		}
	}
	for _, r := range p.HeartRate {
		rec := &database.HeartRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), BPM: r.BPM,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := db.Clauses(upsertPoint).Create(rec).Error; err != nil {
			return fmt.Errorf("upsert heart_rate: %w", err)
		}
	}
	for _, r := range p.HeartRateVariability {
		rec := &database.HeartRateVariability{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), RmssdMillis: r.RmssdMillis,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		if err := db.Clauses(upsertPoint).Create(rec).Error; err != nil {
			return fmt.Errorf("upsert hrv: %w", err)
		}
	}
	for _, r := range p.Sleep {
		end := parseTime(r.SessionEndTime)
		start := end.Add(-time.Duration(r.DurationSeconds) * time.Second)
		sleep := &database.Sleep{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: start, SessionEndTime: end,
			DurationSeconds: r.DurationSeconds,
		}
		sleep.ID = uuid.New()
		sleep.FamilyID = familyID
		res := db.Clauses(doNothing).Create(sleep)
		if res.Error != nil {
			return fmt.Errorf("insert sleep: %w", res.Error)
		}
		// Skip stages if sleep was a duplicate (RowsAffected == 0)
		if res.RowsAffected == 0 {
			continue
		}
		for _, st := range r.Stages {
			stage := &database.SleepStage{
				SleepID:         sleep.ID,
				Stage:           st.Stage,
				StartTime:       parseTime(st.StartTime),
				EndTime:         parseTime(st.EndTime),
				DurationSeconds: st.DurationSeconds,
			}
			db.Clauses(doNothing).Create(stage) //nolint:errcheck
		}
	}
	for _, r := range p.Distance {
		rec := &database.Distance{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Meters: r.Meters,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertInterval).Create(rec) //nolint:errcheck
	}
	for _, r := range p.ActiveCalories {
		rec := &database.ActiveCalories{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Calories: r.Calories,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertInterval).Create(rec) //nolint:errcheck
	}
	for _, r := range p.TotalCalories {
		rec := &database.TotalCalories{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Calories: r.Calories,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertInterval).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Weight {
		rec := &database.Weight{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Kilograms: r.Kilograms,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Height {
		rec := &database.Height{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Meters: r.Meters,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BloodPressure {
		rec := &database.BloodPressure{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Systolic: r.Systolic, Diastolic: r.Diastolic,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BloodGlucose {
		rec := &database.BloodGlucose{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), MmolPerLiter: r.MmolPerLiter,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.OxygenSaturation {
		rec := &database.OxygenSaturation{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Percentage: r.Percentage,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BodyTemperature {
		rec := &database.BodyTemperature{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Celsius: r.Celsius,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.SkinTemperature {
		rec := &database.SkinTemperature{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), DeltaCelsius: r.DeltaCelsius,
			BaselineCelsius: r.BaselineCelsius, MeasurementLocation: r.MeasurementLocation,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.RespiratoryRate {
		rec := &database.RespiratoryRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Rate: r.Rate,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.RestingHeartRate {
		rec := &database.RestingHeartRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), BPM: r.BPM,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Exercise {
		rec := &database.Exercise{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			DurationSeconds: r.DurationSeconds, ExerciseType: r.Type,
			DistanceMeters: r.DistanceMeters, Steps: r.Steps,
			AvgCadenceSpm: r.AvgCadenceSpm, MaxCadenceSpm: r.MaxCadenceSpm,
			StrideLengthM: r.StrideLengthM,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertInterval).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Hydration {
		rec := &database.Hydration{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Liters: r.Liters,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertInterval).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Nutrition {
		rec := &database.Nutrition{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Calories: r.Calories, ProteinGrams: r.ProteinGrams,
			CarbsGrams: r.CarbsGrams, FatGrams: r.FatGrams,
			SugarGrams: r.SugarGrams, SodiumGrams: r.SodiumGrams,
			DietaryFiberGrams: r.DietaryFiberGrams, Name: r.Name,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertInterval).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BasalMetabolicRate {
		rec := &database.BasalMetabolicRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Watts: r.Watts,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BodyFat {
		rec := &database.BodyFat{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Percentage: r.Percentage,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.LeanBodyMass {
		rec := &database.LeanBodyMass{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Kilograms: r.Kilograms,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.VO2Max {
		rec := &database.VO2Max{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), MlPerKgPerMin: r.MlPerKgPerMin,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BoneMass {
		rec := &database.BoneMass{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Kilograms: r.Kilograms,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Speed {
		rec := &database.Speed{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), MetersPerSecond: r.MetersPerSecond,
		}
		rec.ID = uuid.New()
		rec.FamilyID = familyID
		db.Clauses(upsertPoint).Create(rec) //nolint:errcheck
	}
	return nil
}
