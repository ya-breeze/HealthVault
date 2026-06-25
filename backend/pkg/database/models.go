package database

import (
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

// WebhookPayload is the raw audit log of every POST received.
type WebhookPayload struct {
	models.TenantModel
	UserID      uuid.UUID `gorm:"type:uuid;not null;index"`
	ReceivedAt  time.Time `gorm:"not null"`
	AppVersion  string    `gorm:"not null"`
	PayloadTs   time.Time `gorm:"not null"`
	Raw         string    `gorm:"type:text;not null"`
}

// --- interval types (start_time + end_time) ---

type Steps struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_steps_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_steps_user_time"`
	EndTime         time.Time `gorm:"not null"`
	Count           int       `gorm:"not null"`
}

type Distance struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_distance_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_distance_user_time"`
	EndTime         time.Time `gorm:"not null"`
	Meters          float64   `gorm:"not null"`
}

type ActiveCalories struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_active_cal_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_active_cal_user_time"`
	EndTime         time.Time `gorm:"not null"`
	Calories        float64   `gorm:"not null"`
}

type TotalCalories struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_total_cal_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_total_cal_user_time"`
	EndTime         time.Time `gorm:"not null"`
	Calories        float64   `gorm:"not null"`
}

type Hydration struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_hydration_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_hydration_user_time"`
	EndTime         time.Time `gorm:"not null"`
	Liters          float64   `gorm:"not null"`
}

// --- point-in-time types (single time field) ---

type HeartRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_hr_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_hr_user_time"`
	BPM             int       `gorm:"not null"`
}

type HeartRateVariability struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_hrv_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_hrv_user_time"`
	RmssdMillis     float64   `gorm:"not null"`
}

type Weight struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_weight_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_weight_user_time"`
	Kilograms       float64   `gorm:"not null"`
}

type Height struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_height_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_height_user_time"`
	Meters          float64   `gorm:"not null"`
}

type BloodGlucose struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_bg_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bg_user_time"`
	MmolPerLiter    float64   `gorm:"not null"`
}

type OxygenSaturation struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_spo2_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_spo2_user_time"`
	Percentage      float64   `gorm:"not null"`
}

type BodyTemperature struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_bodytemp_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bodytemp_user_time"`
	Celsius         float64   `gorm:"not null"`
}

type RespiratoryRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_rr_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_rr_user_time"`
	Rate            float64   `gorm:"not null"`
}

type RestingHeartRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_rhr_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_rhr_user_time"`
	BPM             int       `gorm:"not null"`
}

type BasalMetabolicRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_bmr_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bmr_user_time"`
	Watts           float64   `gorm:"not null"`
}

type BodyFat struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_bodyfat_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bodyfat_user_time"`
	Percentage      float64   `gorm:"not null"`
}

type LeanBodyMass struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_lbm_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_lbm_user_time"`
	Kilograms       float64   `gorm:"not null"`
}

type VO2Max struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_vo2_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_vo2_user_time"`
	MlPerKgPerMin   float64   `gorm:"not null"`
}

type BoneMass struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_bonemass_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bonemass_user_time"`
	Kilograms       float64   `gorm:"not null"`
}

// --- multi-value types ---

type BloodPressure struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_bp_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bp_user_time"`
	Systolic        float64   `gorm:"not null"`
	Diastolic       float64   `gorm:"not null"`
}

type SkinTemperature struct {
	models.TenantModel
	UserID              uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_skintemp_user_time"`
	SourcePayloadID     uuid.UUID `gorm:"type:uuid;not null"`
	Time                time.Time `gorm:"not null;uniqueIndex:idx_skintemp_user_time"`
	DeltaCelsius        float64   `gorm:"not null"`
	BaselineCelsius     *float64
	MeasurementLocation int       `gorm:"not null"`
}

// --- sleep (derived start_time, unique anchor is session_end_time) ---

type Sleep struct {
	models.TenantModel
	UserID          uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:idx_sleeps_user_time"`
	SourcePayloadID uuid.UUID    `gorm:"type:uuid;not null"`
	StartTime       time.Time    `gorm:"not null"`
	SessionEndTime  time.Time    `gorm:"not null;uniqueIndex:idx_sleeps_user_time"`
	DurationSeconds int          `gorm:"not null"`
	Stages          []SleepStage `gorm:"foreignKey:SleepID"`
}

type SleepStage struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	SleepID         uuid.UUID `gorm:"type:uuid;not null;index"`
	Stage           string    `gorm:"not null"`
	StartTime       time.Time `gorm:"not null"`
	EndTime         time.Time `gorm:"not null"`
	DurationSeconds int       `gorm:"not null"`
}

func (s *SleepStage) BeforeCreate(tx *gorm.DB) error {
	s.ID = uuid.New()
	return nil
}

// --- exercise ---

type Exercise struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_exercise_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_exercise_user_time"`
	EndTime         time.Time `gorm:"not null"`
	DurationSeconds int       `gorm:"not null"`
	ExerciseType    string    `gorm:"not null"`
	DistanceMeters  *float64
	Steps           *int
	AvgCadenceSpm   *float64
	MaxCadenceSpm   *float64
	StrideLengthM   *float64
}

// --- speed (point-in-time series, like HeartRate) ---

type Speed struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_speed_user_time"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_speed_user_time"`
	MetersPerSecond float64   `gorm:"not null"`
}

// --- nutrition ---

type Nutrition struct {
	models.TenantModel
	UserID            uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_nutrition_user_time"`
	SourcePayloadID   uuid.UUID `gorm:"type:uuid;not null"`
	StartTime         time.Time `gorm:"not null;uniqueIndex:idx_nutrition_user_time"`
	EndTime           time.Time `gorm:"not null"`
	Calories          *float64
	ProteinGrams      *float64
	CarbsGrams        *float64
	FatGrams          *float64
	SugarGrams        *float64
	SodiumGrams       *float64
	DietaryFiberGrams *float64
	Name              *string
}
