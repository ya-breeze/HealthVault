// Package backupapi implements HealthVault's private, durable backup jobs API.
package backupapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ContractVersion = 1

type Request struct {
	Trigger        string            `json:"trigger"`
	TargetRevision string            `json:"target_revision"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Evidence struct {
	BackupSetType       string `json:"backup_set_type"`
	ManifestSHA256      string `json:"manifest_sha256"`
	ReadyFileID         string `json:"ready_file_id"`
	CiphertextSHA256    string `json:"ciphertext_sha256"`
	CiphertextSizeBytes int64  `json:"ciphertext_size_bytes"`
	EncryptionKeyID     string `json:"encryption_key_id"`
}

type Job struct {
	ContractVersion int        `json:"contract_version"`
	JobID           string     `json:"job_id"`
	Trigger         string     `json:"trigger"`
	TargetRevision  string     `json:"target_revision"`
	State           string     `json:"state"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	Evidence        *Evidence  `json:"evidence"`
	Error           *Error     `json:"error"`
}

type JobReference struct {
	ContractVersion int    `json:"contract_version"`
	JobID           string `json:"job_id"`
	State           string `json:"state"`
	StatusURL       string `json:"status_url"`
}

type Status struct {
	ContractVersion int  `json:"contract_version"`
	LastAttempt     *Job `json:"last_attempt"`
	LastSuccess     *Job `json:"last_success"`
}

type row struct {
	Sequence       int64     `gorm:"primaryKey;autoIncrement"`
	ID             string    `gorm:"uniqueIndex;size:130;not null"`
	IdempotencyKey string    `gorm:"uniqueIndex;size:128;not null"`
	RequestSHA256  string    `gorm:"size:64;not null"`
	Trigger        string    `gorm:"size:32;not null"`
	TargetRevision string    `gorm:"size:200;not null"`
	State          string    `gorm:"size:16;not null;index"`
	CreatedAt      time.Time `gorm:"not null;index"`
	CompletedAt    *time.Time
	EvidenceJSON   *string
	ErrorJSON      *string
}

// Runner performs the project-owned snapshot operation. Task 1 deliberately ships
// an unconfigured implementation; Task 2 supplies the storage pipeline.
type Runner interface {
	Run(context.Context, Job) (*Evidence, *Error)
}

type RunnerFunc func(context.Context, Job) (*Evidence, *Error)

func (f RunnerFunc) Run(ctx context.Context, job Job) (*Evidence, *Error) { return f(ctx, job) }

type Service struct {
	db     *gorm.DB
	runner Runner
	now    func() time.Time
	runMu  sync.Mutex // one backup set at a time bounds scratch-disk use
}

func New(db *gorm.DB, runner Runner) (*Service, error) {
	if db == nil {
		return nil, errors.New("backup jobs database is required")
	}
	if err := db.AutoMigrate(&row{}); err != nil {
		return nil, fmt.Errorf("initialize backup job ledger: %w", err)
	}
	if runner == nil {
		runner = RunnerFunc(UnconfiguredRunner)
	}
	s := &Service{db: db, runner: runner, now: func() time.Time { return time.Now().UTC() }}
	if err := s.recoverInterrupted(); err != nil {
		return nil, fmt.Errorf("recover backup jobs: %w", err)
	}
	return s, nil
}

func UnconfiguredRunner(context.Context, Job) (*Evidence, *Error) {
	return nil, &Error{Code: "storage_unconfigured", Message: "backup storage is not configured"}
}

func (s *Service) Create(ctx context.Context, key string, req Request) (JobReference, bool, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return JobReference{}, false, err
	}
	digest := sha256.Sum256(payload)
	jobID, err := newJobID()
	if err != nil {
		return JobReference{}, false, err
	}
	created := s.now()
	newRow := row{ID: jobID, IdempotencyKey: key, RequestSHA256: hex.EncodeToString(digest[:]),
		Trigger: req.Trigger, TargetRevision: req.TargetRevision, State: "accepted", CreatedAt: created}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&newRow)
	if result.Error != nil {
		return JobReference{}, false, result.Error
	}
	wasCreated := result.RowsAffected == 1
	if !wasCreated {
		var existing row
		if err := s.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&existing).Error; err != nil {
			return JobReference{}, false, err
		}
		if existing.RequestSHA256 != newRow.RequestSHA256 {
			return JobReference{}, false, ErrIdempotencyConflict
		}
		newRow = existing
	}
	if wasCreated {
		go s.run(jobID)
	}
	return JobReference{ContractVersion: ContractVersion, JobID: newRow.ID, State: newRow.State,
		StatusURL: "/internal/backups/v1/jobs/" + newRow.ID}, wasCreated, nil
}

var ErrIdempotencyConflict = errors.New("idempotency conflict")

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	var item row
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return Job{}, err
	}
	return decodeJob(item)
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	result := Status{ContractVersion: ContractVersion}
	var lastAttempt row
	if err := s.db.WithContext(ctx).Order("sequence DESC").First(&lastAttempt).Error; err == nil {
		job, decodeErr := decodeJob(lastAttempt)
		if decodeErr != nil {
			return Status{}, decodeErr
		}
		result.LastAttempt = &job
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Status{}, err
	}
	var lastSuccess row
	if err := s.db.WithContext(ctx).Where("state = ?", "succeeded").Order("completed_at DESC, sequence DESC").First(&lastSuccess).Error; err == nil {
		job, decodeErr := decodeJob(lastSuccess)
		if decodeErr != nil {
			return Status{}, decodeErr
		}
		result.LastSuccess = &job
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Status{}, err
	}
	return result, nil
}

func (s *Service) run(id string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	started := s.db.Model(&row{}).Where("id = ? AND state = ?", id, "accepted").Updates(map[string]any{"state": "running"})
	if started.Error != nil || started.RowsAffected != 1 {
		return
	}
	job, err := s.Get(context.Background(), id)
	if err != nil {
		return
	}
	evidence, failure := s.executeRunner(job)
	completed := s.now()
	updates := map[string]any{"state": "failed", "completed_at": completed,
		"error_json": `{"code":"internal","message":"backup job failed"}`, "evidence_json": nil}
	if completed.Before(job.CreatedAt) {
		completed = job.CreatedAt
		updates["completed_at"] = completed
	}
	if failure != nil {
		if !validJobError(failure.Code) {
			failure = &Error{Code: "internal", Message: "backup job failed"}
		}
		failure.Message = safeMessage(failure.Code)
		encoded, marshalErr := json.Marshal(failure)
		if marshalErr == nil {
			updates["error_json"] = string(encoded)
		}
	} else if evidence != nil {
		if validateEvidence(*evidence) == nil {
			encoded, marshalErr := json.Marshal(evidence)
			if marshalErr == nil {
				updates["state"] = "succeeded"
				updates["error_json"] = nil
				updates["evidence_json"] = string(encoded)
			}
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		finished := s.db.Model(&row{}).Where("id = ? AND state = ?", id, "running").Updates(updates)
		if finished.Error == nil && finished.RowsAffected == 1 {
			return
		}
		if finished.Error == nil && finished.RowsAffected == 0 {
			return // another owner already moved this job; never overwrite it
		}
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	// A failed SQLite write cannot be reported as success. The deployer will time
	// out, and startup recovery marks the interrupted job failed after restart.
	slog.Error("backup job terminal state could not be persisted", "job_id", id)
}

func (s *Service) executeRunner(job Job) (evidence *Evidence, failure *Error) {
	defer func() {
		if recover() != nil {
			evidence = nil
			failure = &Error{Code: "internal", Message: "backup job failed"}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return s.runner.Run(ctx, job)
}

func (s *Service) recoverInterrupted() error {
	completed := s.now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var interrupted []row
		if err := tx.Where("state IN ?", []string{"accepted", "running"}).Find(&interrupted).Error; err != nil {
			return err
		}
		for _, item := range interrupted {
			finished := completed
			if finished.Before(item.CreatedAt) {
				finished = item.CreatedAt
			}
			if err := tx.Model(&row{}).Where("id = ? AND state IN ?", item.ID, []string{"accepted", "running"}).Updates(map[string]any{
				"state": "failed", "completed_at": finished,
				"error_json":    `{"code":"internal","message":"job interrupted by service restart"}`,
				"evidence_json": nil,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func decodeJob(item row) (Job, error) {
	job := Job{ContractVersion: ContractVersion, JobID: item.ID, Trigger: item.Trigger,
		TargetRevision: item.TargetRevision, State: item.State, CreatedAt: item.CreatedAt.UTC(), CompletedAt: item.CompletedAt}
	if item.CompletedAt != nil {
		v := item.CompletedAt.UTC()
		job.CompletedAt = &v
	}
	if item.EvidenceJSON != nil {
		var value Evidence
		if err := json.Unmarshal([]byte(*item.EvidenceJSON), &value); err != nil {
			return Job{}, err
		}
		job.Evidence = &value
	}
	if item.ErrorJSON != nil {
		var value Error
		if err := json.Unmarshal([]byte(*item.ErrorJSON), &value); err != nil {
			return Job{}, err
		}
		job.Error = &value
	}
	return job, nil
}

func newJobID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "j_" + hex.EncodeToString(b[:]), nil
}

func validJobError(code string) bool {
	switch code {
	case "snapshot_failed", "local_verification_failed", "encryption_failed", "publish_failed", "storage_unconfigured", "timeout", "internal":
		return true
	default:
		return false
	}
}

func safeMessage(code string) string {
	switch code {
	case "snapshot_failed":
		return "backup snapshot failed"
	case "local_verification_failed":
		return "backup verification failed"
	case "encryption_failed":
		return "backup encryption failed"
	case "publish_failed":
		return "backup artifact publication failed"
	case "storage_unconfigured":
		return "backup storage is not configured"
	case "timeout":
		return "backup job timed out"
	default:
		return "backup job failed"
	}
}

func validateEvidence(value Evidence) error {
	if value.BackupSetType != "full" || !isDigest(value.ManifestSHA256) || !isDigest(value.CiphertextSHA256) ||
		!readyFileIDPattern.MatchString(value.ReadyFileID) ||
		value.CiphertextSizeBytes <= 0 || value.EncryptionKeyID == "" || len(value.EncryptionKeyID) > 200 ||
		containsLeak(value.EncryptionKeyID) {
		return errors.New("invalid evidence")
	}
	return nil
}

var readyFileIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.age$`)

func isDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func containsUnsafe(value string) bool {
	for _, r := range value {
		if r == '?' || r < 0x20 || r > 0x7e {
			return true
		}
	}
	return false
}

var (
	localPathPattern = regexp.MustCompile(`(?:^|[\s"'=(:,;])(?:/(?:[\w.-]+/)*[\w.-]+|~/[\w./-]+)|[A-Za-z]:\\`)
	dsnPattern       = regexp.MustCompile(`\b[a-z][a-z0-9+.-]*://[^\s/@]*@`)
	secretPattern    = regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|secret|token|api_key)\s*[=:]`)
	presignedPattern = regexp.MustCompile(`(?i)\bx-(?:amz|goog)-(?:credential|signature|security-token)\b|[?&](?:sig|signature|se|sv|token|access_token)=`)
)

func containsLeak(value string) bool {
	return localPathPattern.MatchString(value) || dsnPattern.MatchString(value) ||
		secretPattern.MatchString(value) || presignedPattern.MatchString(value)
}
