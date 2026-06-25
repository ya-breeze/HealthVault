package database

import (
	"fmt"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

type storageImpl struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) Storage {
	return &storageImpl{db: db}
}

func (s *storageImpl) DB() *gorm.DB { return s.db }

func (s *storageImpl) FindUserByName(username string) (*kinmodels.User, error) {
	var u kinmodels.User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *storageImpl) FindUserByID(id uuid.UUID) (*kinmodels.User, error) {
	var u kinmodels.User
	if err := s.db.Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *storageImpl) FindUsersByFamilyID(familyID uuid.UUID) ([]kinmodels.User, error) {
	var users []kinmodels.User
	return users, s.db.Where("family_id = ?", familyID).Find(&users).Error
}

func (s *storageImpl) AllUsers() ([]kinmodels.User, error) {
	var users []kinmodels.User
	return users, s.db.Find(&users).Error
}

func (s *storageImpl) SaveWebhookPayload(p *WebhookPayload) error {
	return s.db.Create(p).Error
}

func (s *storageImpl) DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error {
	result := s.db.Exec(
		"DELETE FROM "+tableName+" WHERE id = ? AND user_id = ?",
		id, userID,
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *storageImpl) QueryRecords(tableName string, timeCol string, userID uuid.UUID, tr TimeRange) ([]map[string]any, error) {
	var results []map[string]any
	query := fmt.Sprintf("user_id = ? AND %s >= ? AND %s <= ?", timeCol, timeCol)
	err := s.db.Table(tableName).
		Where(query, userID, tr.From, tr.To).
		Find(&results).Error
	return results, err
}

func (s *storageImpl) SummarySteps(userID uuid.UUID, tr TimeRange) (int, error) {
	var total int
	err := s.db.Model(&Steps{}).
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Select("COALESCE(SUM(count), 0)").Scan(&total).Error
	return total, err
}

func (s *storageImpl) SummaryAvgHeartRate(userID uuid.UUID, tr TimeRange) (float64, error) {
	var avg float64
	err := s.db.Model(&HeartRate{}).
		Where("user_id = ? AND time >= ? AND time <= ?", userID, tr.From, tr.To).
		Select("COALESCE(AVG(bpm), 0)").Scan(&avg).Error
	return avg, err
}

func (s *storageImpl) SummarySleepSeconds(userID uuid.UUID, tr TimeRange) (int, error) {
	var total int
	err := s.db.Model(&Sleep{}).
		Where("user_id = ? AND start_time >= ? AND start_time <= ?", userID, tr.From, tr.To).
		Select("COALESCE(SUM(duration_seconds), 0)").Scan(&total).Error
	return total, err
}

var _ Storage = (*storageImpl)(nil) // compile-time interface check
