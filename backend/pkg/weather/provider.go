package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

var variables = []string{"temperature_2m", "apparent_temperature", "relative_humidity_2m", "surface_pressure", "pressure_msl", "precipitation", "wind_speed_10m"}

func Units() map[string]string {
	return map[string]string{"temperature_c": "°C", "apparent_temperature_c": "°C", "relative_humidity_percent": "%", "surface_pressure_hpa": "hPa", "mean_sea_level_pressure_hpa": "hPa", "precipitation_mm": "mm", "wind_speed_kmh": "km/h"}
}

type Provider interface {
	Fetch(context.Context, database.WeatherHour) (database.WeatherHour, error)
}
type OpenMeteo struct {
	Client *http.Client
	URL    string
}

func NewOpenMeteo() *OpenMeteo {
	return &OpenMeteo{Client: &http.Client{Timeout: 15 * time.Second}, URL: "https://historical-forecast-api.open-meteo.com/v1/forecast"}
}

func (p *OpenMeteo) Fetch(ctx context.Context, h database.WeatherHour) (database.WeatherHour, error) {
	// Only archived model output for elapsed UTC intervals is requested. It is
	// labelled historical forecast, never direct weather-station observation.
	endpoint, err := url.Parse(p.URL)
	if err != nil {
		return h, err
	}
	q := endpoint.Query()
	q.Set("latitude", strconv.FormatFloat(h.Latitude, 'f', 1, 64))
	q.Set("longitude", strconv.FormatFloat(h.Longitude, 'f', 1, 64))
	q.Set("start_hour", h.Hour.Format("2006-01-02T15:04"))
	q.Set("end_hour", h.Hour.Add(time.Hour).Format("2006-01-02T15:04"))
	q.Set("hourly", strings.Join(variables, ","))
	q.Set("models", "ecmwf_ifs025")
	q.Set("timezone", "UTC")
	q.Set("temperature_unit", "celsius")
	q.Set("wind_speed_unit", "kmh")
	q.Set("precipitation_unit", "mm")
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return h, err
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return h, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return h, fmt.Errorf("weather provider status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return h, err
	}
	if len(raw) > 256*1024 {
		return h, fmt.Errorf("provider response too large")
	}
	var data struct {
		Hourly    map[string]json.RawMessage `json:"hourly"`
		Units     map[string]string          `json:"hourly_units"`
		UTCOffset int                        `json:"utc_offset_seconds"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return h, err
	}
	if data.UTCOffset != 0 {
		return h, fmt.Errorf("provider timezone mismatch")
	}
	var times []string
	if err := json.Unmarshal(data.Hourly["time"], &times); err != nil {
		return h, err
	}
	if len(times) == 0 || len(times) > 48 {
		return h, fmt.Errorf("provider time size")
	}
	index := -1
	precipitationIndex := -1
	for i, v := range times {
		ts, e := time.Parse("2006-01-02T15:04", v)
		if e != nil {
			return h, e
		}
		if i > 0 {
			prev, _ := time.Parse("2006-01-02T15:04", times[i-1])
			if ts.Sub(prev) != time.Hour {
				return h, fmt.Errorf("provider times misaligned")
			}
		}
		if ts.Equal(h.Hour) {
			index = i
		}
		if ts.Equal(h.Hour.Add(time.Hour)) {
			precipitationIndex = i
		}
	}
	if index < 0 || precipitationIndex < 0 {
		return h, fmt.Errorf("provider hour missing")
	}
	expectedUnits := []string{"°C", "°C", "%", "hPa", "hPa", "mm", "km/h"}
	values := make([]float64, 7)
	for j, key := range variables {
		if data.Units[key] != expectedUnits[j] {
			return h, fmt.Errorf("provider unit mismatch for %s", key)
		}
		var arr []*float64
		if err := json.Unmarshal(data.Hourly[key], &arr); err != nil {
			return h, err
		}
		if len(arr) != len(times) {
			return h, fmt.Errorf("provider array misaligned")
		}
		// Reject invalid data anywhere in the supplied arrays; null must never
		// silently turn into a measured zero. Precipitation is the preceding-hour
		// accumulation, so use the value at the interval's end.
		for _, v := range arr {
			if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
				return h, fmt.Errorf("provider missing or nonfinite value")
			}
		}
		k := index
		if key == "precipitation" {
			k = precipitationIndex
		}
		values[j] = *arr[k]
	}
	h.TemperatureC = values[0]
	h.ApparentTemperatureC = values[1]
	h.RelativeHumidityPercent = values[2]
	h.SurfacePressureHpa = values[3]
	h.MeanSeaLevelPressureHpa = values[4]
	h.PrecipitationMm = values[5]
	h.WindSpeedKmh = values[6]
	h.Source = "open-meteo historical-forecast"
	h.Model = "ecmwf_ifs025"
	h.FetchedAt = time.Now().UTC()
	h.Status = "ready"
	return h, nil
}

// ProcessBatch reads at most twelve due jobs, then persists each result under
// the capture barrier. Evidence may change during HTTP; conditional ID writes
// prevent a removed interval from being resurrected by an in-flight request.
func (s *Store) ProcessBatch(ctx context.Context, p Provider, barrier *sync.RWMutex, now time.Time) error {
	var jobs []database.WeatherHour
	if err := s.DB.WithContext(ctx).Where("status = ? AND next_attempt <= ? AND hour <= ?", "pending", now, now.Add(-time.Hour)).Order("next_attempt, hour").Limit(12).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		result, fetchErr := p.Fetch(callCtx, job)
		cancel()
		barrier.RLock()
		s.mu.Lock()
		var writeErr error
		if fetchErr == nil {
			// Keep identity/provenance from the durable job, not the provider.
			result.ID = job.ID
			result.UserID = job.UserID
			result.FamilyID = job.FamilyID
			result.EvidenceRevision = job.EvidenceRevision
			result.Hour = job.Hour
			result.FirstObservationID = job.FirstObservationID
			result.LastObservationID = job.LastObservationID
			result.Latitude = job.Latitude
			result.Longitude = job.Longitude
			result.Status = "ready"
			writeErr = s.DB.Model(&database.WeatherHour{}).Where("id = ? AND evidence_revision = ? AND status = ?", job.ID, job.EvidenceRevision, "pending").Select("temperature_c", "apparent_temperature_c", "relative_humidity_percent", "surface_pressure_hpa", "mean_sea_level_pressure_hpa", "precipitation_mm", "wind_speed_kmh", "source", "model", "fetched_at", "status").Updates(&result).Error
		} else {
			attempts := job.Attempts + 1
			delay := time.Duration(1<<min(attempts, 8)) * time.Minute
			writeErr = s.DB.Model(&database.WeatherHour{}).Where("id = ? AND evidence_revision = ? AND status = ?", job.ID, job.EvidenceRevision, "pending").Updates(map[string]any{"attempts": attempts, "next_attempt": now.Add(delay)}).Error
		}
		s.mu.Unlock()
		barrier.RUnlock()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (s *Store) Run(ctx context.Context, p Provider, barrier *sync.RWMutex, logger *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.ProcessBatch(ctx, p, barrier, time.Now().UTC()); err != nil && ctx.Err() == nil {
			logger.Warn("weather persistence failed; pending work will retry")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
