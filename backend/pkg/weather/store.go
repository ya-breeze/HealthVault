// Package weather keeps location evidence and model weather separate from medical records.
package weather

import (
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

var ErrConflict = errors.New("observation identity conflicts")

// Store serializes evidence repair with provider writes in this single-replica service.
type Store struct {
	DB *gorm.DB
	mu sync.Mutex
}

func (s *Store) Accept(o database.WeatherLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var old database.WeatherLocation
		err := tx.Where("id = ?", o.ID).Take(&old).Error
		if err == nil {
			if old.UserID != o.UserID || old.FamilyID != o.FamilyID || !old.ObservedAt.Equal(o.ObservedAt) || old.Latitude != o.Latitude || old.Longitude != o.Longitude || old.AccuracyM != o.AccuracyM {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Model(&database.WeatherLocation{}).Where("user_id = ? AND observed_at = ?", o.UserID, o.ObservedAt).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrConflict
		}
		var prev, next database.WeatherLocation
		scope := func() *gorm.DB { return tx.Where("user_id = ? AND family_id = ?", o.UserID, o.FamilyID) }
		pe := scope().Where("observed_at < ?", o.ObservedAt).Order("observed_at DESC").Take(&prev).Error
		ne := scope().Where("observed_at > ?", o.ObservedAt).Order("observed_at ASC").Take(&next).Error
		for _, e := range []error{pe, ne} {
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		if err := tx.Create(&o).Error; err != nil {
			return err
		}
		if pe == nil && ne == nil {
			if err := scope().Where("first_observation_id = ? AND last_observation_id = ?", prev.ID, next.ID).Delete(&database.WeatherCoverage{}).Error; err != nil {
				return err
			}
		}
		if pe == nil {
			if err := createPair(tx, prev, o); err != nil {
				return err
			}
		}
		if ne == nil {
			if err := createPair(tx, o, next); err != nil {
				return err
			}
		}
		return rebuildHours(tx, o)
	})
}

func pairReason(a, b database.WeatherLocation) string {
	if b.ObservedAt.Sub(a.ObservedAt) > 6*time.Hour {
		return "long_gap"
	}
	if a.AccuracyM > 10000 || b.AccuracyM > 10000 {
		return "poor_accuracy"
	}
	const rad = math.Pi / 180
	dlat, dlon := (b.Latitude-a.Latitude)*rad, (b.Longitude-a.Longitude)*rad
	h := math.Pow(math.Sin(dlat/2), 2) + math.Cos(a.Latitude*rad)*math.Cos(b.Latitude*rad)*math.Pow(math.Sin(dlon/2), 2)
	distance := 6371000 * 2 * math.Asin(math.Sqrt(math.Min(1, h)))
	if distance+a.AccuracyM+b.AccuracyM > 25000 {
		return "moving"
	}
	return ""
}

func createPair(tx *gorm.DB, a, b database.WeatherLocation) error {
	c := database.WeatherCoverage{ID: uuid.New(), UserID: a.UserID, FamilyID: a.FamilyID, FirstObservationID: a.ID, LastObservationID: b.ID, From: a.ObservedAt, To: b.ObservedAt, Reason: pairReason(a, b)}
	if err := tx.Create(&c).Error; err != nil {
		return err
	}
	return nil
}

type Gap struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Reason string    `json:"reason"`
}
type History struct {
	Hours []database.WeatherHour `json:"hours"`
	Gaps  []Gap                  `json:"gaps"`
	Units map[string]string      `json:"units"`
}

func (s *Store) History(user, family uuid.UUID, from, to time.Time) (History, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := History{Hours: []database.WeatherHour{}, Gaps: []Gap{}, Units: Units()}
	var hours []database.WeatherHour
	var spans []database.WeatherCoverage
	if err := s.DB.Where("user_id = ? AND family_id = ? AND hour >= ? AND hour < ?", user, family, from.Truncate(time.Hour), to).Order("hour").Limit(746).Find(&hours).Error; err != nil {
		return out, err
	}
	if err := s.DB.Where("user_id = ? AND family_id = ? AND \"from\" < ? AND \"to\" > ?", user, family, to, from).Order("\"from\"").Limit(10001).Find(&spans).Error; err != nil {
		return out, err
	}
	if len(spans) > 10000 {
		return out, errors.New("too many coverage intervals")
	}
	// Split gaps at actual evidence boundaries. UTC bucketing must not shift
	// travel or missing-location reasons to the following whole hour.
	cursor := from
	for cursor.Before(to) {
		end := cursor.Truncate(time.Hour).Add(time.Hour)
		if end.After(to) {
			end = to
		}
		reason := "missing_location"
		for _, c := range spans {
			if !c.From.After(cursor) && c.To.After(cursor) {
				reason = c.Reason
				if reason == "" {
					reason = "incomplete_hour"
				}
			}
			if c.From.After(cursor) && c.From.Before(end) {
				end = c.From
			}
			if c.To.After(cursor) && c.To.Before(end) {
				end = c.To
			}
		}
		ready := false
		for _, h := range hours {
			if h.Hour.Equal(cursor) && !h.Hour.Add(time.Hour).After(to) && h.Status == "ready" {
				out.Hours = append(out.Hours, h)
				cursor = h.Hour.Add(time.Hour)
				ready = true
				break
			}
			if !h.Hour.After(cursor) && h.Hour.Add(time.Hour).After(cursor) && h.Status == "pending" {
				reason = "pending_weather"
			}
		}
		if ready {
			continue
		}
		n := len(out.Gaps)
		if n > 0 && out.Gaps[n-1].Reason == reason && out.Gaps[n-1].To.Equal(cursor) {
			out.Gaps[n-1].To = end
		} else {
			out.Gaps = append(out.Gaps, Gap{cursor, end, reason})
		}
		cursor = end
	}

	return out, nil
}

// rebuildHours handles jittered phone samples by requiring every adjacent pair
// in a bracketing chain to be compatible. Only hours within six hours of the
// newly inserted evidence can change; older ready rows retain their provenance.
func rebuildHours(tx *gorm.DB, o database.WeatherLocation) error {
	start := o.ObservedAt.Truncate(time.Hour).Add(-6 * time.Hour)
	end := o.ObservedAt.Truncate(time.Hour).Add(6 * time.Hour)
	var evidence []database.WeatherLocation
	if err := tx.Where("user_id = ? AND family_id = ? AND observed_at >= ? AND observed_at <= ?", o.UserID, o.FamilyID, start.Add(-6*time.Hour), end.Add(7*time.Hour)).Order("observed_at").Limit(10001).Find(&evidence).Error; err != nil {
		return err
	}
	if len(evidence) > 10000 {
		return errors.New("too many weather observations in repair window")
	}
	for h := start; !h.After(end); h = h.Add(time.Hour) {
		first, last := -1, -1
		for i, e := range evidence {
			if !e.ObservedAt.After(h) {
				first = i
			}
			if last < 0 && !e.ObservedAt.Before(h.Add(time.Hour)) {
				last = i
			}
		}
		supported := first >= 0 && last > first && pairReason(evidence[first], evidence[last]) == ""
		if supported {
			for i := first; i < last; i++ {
				if pairReason(evidence[i], evidence[i+1]) != "" || pairReason(evidence[first], evidence[i+1]) != "" {
					supported = false
					break
				}
			}
		}
		var existing database.WeatherHour
		err := tx.Where("user_id = ? AND hour = ?", o.UserID, h).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		ids := ""
		if supported {
			chain := make([]uuid.UUID, 0, last-first+1)
			for i := first; i <= last; i++ {
				chain = append(chain, evidence[i].ID)
			}
			raw, _ := json.Marshal(chain)
			ids = string(raw)
		}
		if err == nil && supported && existing.ObservationIDs == ids {
			continue
		}
		if err == nil {
			if e := tx.Delete(&existing).Error; e != nil {
				return e
			}
		}
		if supported {
			a, b := evidence[first], evidence[last]
			job := database.WeatherHour{ID: uuid.New(), UserID: o.UserID, FamilyID: o.FamilyID, EvidenceRevision: uuid.New(), Hour: h, FirstObservationID: a.ID, LastObservationID: b.ID, ObservationIDs: ids, Latitude: a.Latitude, Longitude: a.Longitude, Status: "pending", NextAttempt: time.Now().UTC()}
			if e := tx.Create(&job).Error; e != nil {
				return e
			}
		}
	}
	return nil
}
