package database

import (
	"time"

	"github.com/google/uuid"
)

// WeatherLocation stores only coordinates already rounded by the phone.
type WeatherLocation struct {
	ID         uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`
	UserID     uuid.UUID `gorm:"type:char(36);uniqueIndex:weather_location_time;index" json:"-"`
	FamilyID   uuid.UUID `gorm:"type:char(36);index" json:"-"`
	ObservedAt time.Time `gorm:"uniqueIndex:weather_location_time;index" json:"observed_at"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	AccuracyM  float64   `json:"accuracy_m"`
}

// WeatherCoverage records adjacent evidence even when it cannot establish weather.
type WeatherCoverage struct {
	ID                 uuid.UUID `gorm:"type:char(36);primaryKey"`
	UserID             uuid.UUID `gorm:"type:char(36);index"`
	FamilyID           uuid.UUID `gorm:"type:char(36);index"`
	FirstObservationID uuid.UUID `gorm:"type:char(36);index"`
	LastObservationID  uuid.UUID `gorm:"type:char(36);index"`
	From               time.Time `gorm:"index"`
	To                 time.Time `gorm:"index"`
	Reason             string
}

// WeatherHour doubles as a durable provider job until Status becomes ready.
type WeatherHour struct {
	ID                      uuid.UUID `gorm:"type:char(36);primaryKey" json:"-"`
	UserID                  uuid.UUID `gorm:"type:char(36);uniqueIndex:weather_user_hour;index" json:"-"`
	FamilyID                uuid.UUID `gorm:"type:char(36);index" json:"-"`
	EvidenceRevision        uuid.UUID `gorm:"type:char(36);index" json:"-"`
	ObservationIDs          string    `json:"-"`
	Hour                    time.Time `gorm:"uniqueIndex:weather_user_hour;index" json:"hour"`
	FirstObservationID      uuid.UUID `gorm:"type:char(36)" json:"first_observation_id"`
	LastObservationID       uuid.UUID `gorm:"type:char(36)" json:"last_observation_id"`
	Latitude                float64   `json:"latitude"`
	Longitude               float64   `json:"longitude"`
	Status                  string    `gorm:"index" json:"-"`
	NextAttempt             time.Time `gorm:"index" json:"-"`
	Attempts                int       `json:"-"`
	TemperatureC            float64   `json:"temperature_c"`
	ApparentTemperatureC    float64   `json:"apparent_temperature_c"`
	RelativeHumidityPercent float64   `json:"relative_humidity_percent"`
	SurfacePressureHpa      float64   `json:"surface_pressure_hpa"`
	MeanSeaLevelPressureHpa float64   `json:"mean_sea_level_pressure_hpa"`
	PrecipitationMm         float64   `json:"precipitation_mm"`
	WindSpeedKmh            float64   `json:"wind_speed_kmh"`
	Source                  string    `json:"source"`
	Model                   string    `json:"model"`
	FetchedAt               time.Time `json:"fetched_at"`
}
