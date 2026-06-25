package database

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/auth"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

// SeedUsers parses "FamilyName:username:password,..." and creates missing families/users.
// It is idempotent: existing families and users are left unchanged.
func SeedUsers(db *gorm.DB, spec string) error {
	if spec == "" {
		return nil
	}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid seed entry %q (want FamilyName:username:password)", entry)
		}
		familyName, username, password := parts[0], parts[1], parts[2]

		// Find or create the family. TenantModel/Family has no BeforeCreate hook,
		// so we must supply the ID ourselves. We use an explicit find-then-create
		// rather than FirstOrCreate to avoid GORM applying attrs to the result struct.
		var family kinmodels.Family
		if err := db.Where("name = ?", familyName).First(&family).Error; err != nil {
			// Not found — create it with an explicit ID.
			family = kinmodels.Family{
				ID:   uuid.New(),
				Name: familyName,
			}
			if err := db.Create(&family).Error; err != nil {
				return fmt.Errorf("seed family %q: %w", familyName, err)
			}
		}

		// Check if user already exists — do NOT update password if so.
		var existing kinmodels.User
		if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
			continue // already exists, skip
		}

		hash, err := auth.HashPassword(password)
		if err != nil {
			return fmt.Errorf("hash password for %q: %w", username, err)
		}

		// Must set ID and FamilyID explicitly — User has no BeforeCreate hook.
		user := kinmodels.User{
			ID:           uuid.New(),
			Username:     username,
			PasswordHash: hash,
			FamilyID:     family.ID,
		}
		if err := db.Create(&user).Error; err != nil {
			return fmt.Errorf("create user %q: %w", username, err)
		}
	}
	return nil
}
