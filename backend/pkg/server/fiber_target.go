package server

import (
	"encoding/json"
	"errors"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"time"
)

func fiberTarget(settings string, now time.Time) *int {
	grams, _ := resolveFiberTarget(settings, now)
	return grams
}

// resolveFiberTarget also identifies the source without changing the shared rules.
func resolveFiberTarget(settings string, now time.Time) (*int, string) {
	var obj map[string]json.RawMessage
	_ = json.Unmarshal([]byte(settings), &obj)
	var n int
	if json.Unmarshal(obj["fiber_target_grams"], &n) == nil && n >= 1 && n <= 200 {
		return &n, "configured"
	}
	p := parseUserProfile(settings)
	if p.HasBirthdate && calendarAge(p.Birthdate, now) < 18 {
		return nil, "unavailable_under_18"
	}
	n = 25
	return &n, "default"
}

// Patch only the fiber setting inside a transaction, retaining other preferences.
func FiberTargetHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		var input struct {
			Grams *int `json:"grams"`
		}
		if !decodeBoundedJSON(w, r, 1024, &input) {
			return
		}
		if input.Grams != nil && (*input.Grams < 1 || *input.Grams > 200) {
			http.Error(w, "grams must be 1..200 or null", 400)
			return
		}
		var resolved *int
		err := storage.DB().Transaction(func(tx *gorm.DB) error {
			var row database.UserSettings
			err := tx.Where("user_id = ?", claims.UserID).First(&row).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			obj := map[string]any{}
			if row.SettingsJSON != "" {
				if err := json.Unmarshal([]byte(row.SettingsJSON), &obj); err != nil {
					return err
				}
			}
			if obj == nil {
				obj = map[string]any{}
			}
			if input.Grams == nil {
				delete(obj, "fiber_target_grams")
			} else {
				obj["fiber_target_grams"] = *input.Grams
			}
			body, err := json.Marshal(obj)
			if err != nil {
				return err
			}
			row.UserID = claims.UserID
			row.FamilyID = FamilyIDFromCtx(r)
			row.SettingsJSON = string(body)
			resolved = fiberTarget(row.SettingsJSON, time.Now().UTC())
			return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"settings_json"})}).Create(&row).Error
		})
		if err != nil {
			http.Error(w, "save error", 500)
			return
		}
		writeJSON(w, map[string]any{"grams": resolved})
	}
}
