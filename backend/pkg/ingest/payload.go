package ingest

// PayloadJSON mirrors the HC Webhook JSON shape. All type arrays are optional.
type PayloadJSON struct {
	Timestamp  string `json:"timestamp"`
	AppVersion string `json:"app_version"`

	Steps                []StepsJSON                `json:"steps,omitempty"`
	HeartRate            []HeartRateJSON            `json:"heart_rate,omitempty"`
	HeartRateVariability []HRVJson                  `json:"heart_rate_variability,omitempty"`
	Sleep                []SleepJSON                `json:"sleep,omitempty"`
	Distance             []DistanceJSON             `json:"distance,omitempty"`
	ActiveCalories       []CaloriesJSON             `json:"active_calories,omitempty"`
	TotalCalories        []CaloriesJSON             `json:"total_calories,omitempty"`
	Weight               []WeightJSON               `json:"weight,omitempty"`
	Height               []HeightJSON               `json:"height,omitempty"`
	BloodPressure        []BloodPressureJSON        `json:"blood_pressure,omitempty"`
	BloodGlucose         []BloodGlucoseJSON         `json:"blood_glucose,omitempty"`
	OxygenSaturation     []OxygenSaturationJSON     `json:"oxygen_saturation,omitempty"`
	BodyTemperature      []BodyTemperatureJSON      `json:"body_temperature,omitempty"`
	SkinTemperature      []SkinTemperatureJSON      `json:"skin_temperature,omitempty"`
	RespiratoryRate      []RespiratoryRateJSON      `json:"respiratory_rate,omitempty"`
	RestingHeartRate     []RestingHeartRateJSON     `json:"resting_heart_rate,omitempty"`
	Exercise             []ExerciseJSON             `json:"exercise,omitempty"`
	Hydration            []HydrationJSON            `json:"hydration,omitempty"`
	Nutrition            []NutritionJSON            `json:"nutrition,omitempty"`
	BasalMetabolicRate   []BasalMetabolicRateJSON   `json:"basal_metabolic_rate,omitempty"`
	BodyFat              []BodyFatJSON              `json:"body_fat,omitempty"`
	LeanBodyMass         []LeanBodyMassJSON         `json:"lean_body_mass,omitempty"`
	VO2Max               []VO2MaxJSON               `json:"vo2_max,omitempty"`
	BoneMass             []BoneMassJSON             `json:"bone_mass,omitempty"`
}

type StepsJSON struct {
	Count     int    `json:"count"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}
type HeartRateJSON struct {
	BPM  int    `json:"bpm"`
	Time string `json:"time"`
}
type HRVJson struct {
	RmssdMillis float64 `json:"rmssd_millis"`
	Time        string  `json:"time"`
}
type SleepStageJSON struct {
	Stage           string `json:"stage"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	DurationSeconds int    `json:"duration_seconds"`
}
type SleepJSON struct {
	SessionEndTime  string           `json:"session_end_time"`
	DurationSeconds int              `json:"duration_seconds"`
	Stages          []SleepStageJSON `json:"stages"`
}
type DistanceJSON struct {
	Meters    float64 `json:"meters"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
}
type CaloriesJSON struct {
	Calories  float64 `json:"calories"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
}
type WeightJSON struct {
	Kilograms float64 `json:"kilograms"`
	Time      string  `json:"time"`
}
type HeightJSON struct {
	Meters float64 `json:"meters"`
	Time   string  `json:"time"`
}
type BloodPressureJSON struct {
	Systolic  float64 `json:"systolic"`
	Diastolic float64 `json:"diastolic"`
	Time      string  `json:"time"`
}
type BloodGlucoseJSON struct {
	MmolPerLiter float64 `json:"mmol_per_liter"`
	Time         string  `json:"time"`
}
type OxygenSaturationJSON struct {
	Percentage float64 `json:"percentage"`
	Time       string  `json:"time"`
}
type BodyTemperatureJSON struct {
	Celsius float64 `json:"celsius"`
	Time    string  `json:"time"`
}
type SkinTemperatureJSON struct {
	Time                string   `json:"time"`
	DeltaCelsius        float64  `json:"delta_celsius"`
	BaselineCelsius     *float64 `json:"baseline_celsius,omitempty"`
	MeasurementLocation int      `json:"measurement_location"`
}
type RespiratoryRateJSON struct {
	Rate float64 `json:"rate"`
	Time string  `json:"time"`
}
type RestingHeartRateJSON struct {
	BPM  int    `json:"bpm"`
	Time string `json:"time"`
}
type ExerciseJSON struct {
	Type            string   `json:"type"`
	StartTime       string   `json:"start_time"`
	EndTime         string   `json:"end_time"`
	DurationSeconds int      `json:"duration_seconds"`
	DistanceMeters  *float64 `json:"distance_meters,omitempty"`
	Steps           *int     `json:"steps,omitempty"`
	AvgCadenceSpm   *float64 `json:"avg_cadence_spm,omitempty"`
	MaxCadenceSpm   *float64 `json:"max_cadence_spm,omitempty"`
	StrideLengthM   *float64 `json:"stride_length_m,omitempty"`
}
type HydrationJSON struct {
	Liters    float64 `json:"liters"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
}
type NutritionJSON struct {
	StartTime         string   `json:"start_time"`
	EndTime           string   `json:"end_time"`
	Calories          *float64 `json:"calories,omitempty"`
	ProteinGrams      *float64 `json:"protein_grams,omitempty"`
	CarbsGrams        *float64 `json:"carbs_grams,omitempty"`
	FatGrams          *float64 `json:"fat_grams,omitempty"`
	SugarGrams        *float64 `json:"sugar_grams,omitempty"`
	SodiumGrams       *float64 `json:"sodium_grams,omitempty"`
	DietaryFiberGrams *float64 `json:"dietary_fiber_grams,omitempty"`
	Name              *string  `json:"name,omitempty"`
}
type BasalMetabolicRateJSON struct {
	Watts float64 `json:"watts"`
	Time  string  `json:"time"`
}
type BodyFatJSON struct {
	Percentage float64 `json:"percentage"`
	Time       string  `json:"time"`
}
type LeanBodyMassJSON struct {
	Kilograms float64 `json:"kilograms"`
	Time      string  `json:"time"`
}
type VO2MaxJSON struct {
	MlPerKgPerMin float64 `json:"ml_per_kg_per_min"`
	Time          string  `json:"time"`
}
type BoneMassJSON struct {
	Kilograms float64 `json:"kilograms"`
	Time      string  `json:"time"`
}
