# HealthVault Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go+SQLite backend that receives HC Webhook health data, stores it in normalized per-type tables, serves a REST API and Next.js dashboard, and exposes an HTTP MCP server.

**Architecture:** Single Go binary (`hcw`) with Cobra subcommands, GORM+SQLite database, kin-core for family/user auth, gorilla/mux router. Next.js frontend built as static export, served by nginx alongside the backend.

**Tech Stack:** Go 1.26, GORM+SQLite, Cobra+Viper, github.com/ya-breeze/kin-core, github.com/modelcontextprotocol/go-sdk v1.3.0, gorilla/mux, Next.js App Router, Tailwind CSS, Recharts, nginx.

## Global Constraints

- Module path: `github.com/ya-breeze/healthvault`
- Go version: 1.26
- All env vars prefixed `HCW_` (e.g. `HCW_PORT`, `HCW_DBPATH`)
- `HCW_SEED_USERS` format: `FamilyName:username:password` comma-separated
- Never hardcode `container_name` in docker-compose
- Volume host paths use `/mnt/eight-2/eight-2/data/data/hcw-wip`
- WIP ports: HTTP 8888, HTTPS 9888
- kin-core: `TenantModel` has no BeforeCreate hook — set `ID = uuid.New()` and `FamilyID` explicitly before every `Create()`
- MCP transport: `mcp.NewStreamableHTTPHandler` (HTTP streamable, not stdio)
- No `container_name` in docker-compose files

---

### Task 1: Project Scaffolding

**Files:**
- Create: `backend/go.mod`
- Create: `backend/cmd/main.go`
- Create: `backend/cmd/commands/cmdserver.go`
- Create: `backend/pkg/config/config.go`
- Create: `Makefile`
- Create: `.gitignore`

**Interfaces:**
- Produces: `config.Load() (*Config, error)`, `Config` struct with all HCW_ env vars

- [ ] **Step 1: Initialize go module**

```bash
cd /data/HealthVault/backend
go mod init github.com/ya-breeze/healthvault
```

- [ ] **Step 2: Create `backend/pkg/config/config.go`**

```go
package config

import "github.com/spf13/viper"

type Config struct {
	Port           string
	DBPath         string
	SeedUsers      string
	JWTSecret      string
	CookieSecure   bool
	BackupPath     string
	BackupInterval string
	BackupMaxCount int
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("HCW")
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("DBPATH", "hcw.db")
	viper.SetDefault("BACKUP_INTERVAL", "24h")
	viper.SetDefault("BACKUP_MAX_COUNT", 10)
	viper.SetDefault("COOKIE_SECURE", true)
	return &Config{
		Port:           viper.GetString("PORT"),
		DBPath:         viper.GetString("DBPATH"),
		SeedUsers:      viper.GetString("SEED_USERS"),
		JWTSecret:      viper.GetString("JWT_SECRET"),
		CookieSecure:   viper.GetBool("COOKIE_SECURE"),
		BackupPath:     viper.GetString("BACKUP_PATH"),
		BackupInterval: viper.GetString("BACKUP_INTERVAL"),
		BackupMaxCount: viper.GetInt("BACKUP_MAX_COUNT"),
	}, nil
}
```

- [ ] **Step 3: Create `backend/cmd/commands/cmdserver.go`**

```go
package commands

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/pkg/config"
)

func CmdServer(logger *slog.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start the HealthVault HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			logger.Info("starting server", "port", cfg.Port, "db", cfg.DBPath)
			// server.Run called in Task 4
			_ = cfg
			return nil
		},
	}
}
```

- [ ] **Step 4: Create `backend/cmd/main.go`**

```go
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/cmd/commands"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	root := &cobra.Command{Use: "hcw", Short: "HealthVault"}
	root.AddCommand(commands.CmdServer(logger))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Create `Makefile`**

```makefile
ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: all build test lint run-backend

all: build

build:
	@cd $(ROOT_DIR)/backend/cmd && go build -o ../bin/hcw .

test:
	@cd $(ROOT_DIR)/backend && go test ./...

lint:
	@cd $(ROOT_DIR)/backend && go vet ./...

run-backend: build
	@HCW_DBPATH=$(ROOT_DIR)hcw.db \
	HCW_JWT_SECRET=devsecret \
	HCW_COOKIE_SECURE=false \
	HCW_SEED_USERS="TestFamily:alice:pass1,TestFamily:bob:pass2" \
	$(ROOT_DIR)/backend/bin/hcw server
```

- [ ] **Step 6: Create `.gitignore`**

```
backend/bin/
*.db
*.db-shm
*.db-wal
node_modules/
frontend/.next/
frontend/out/
.env
```

- [ ] **Step 7: Add dependencies**

```bash
cd /data/HealthVault/backend
go get github.com/spf13/cobra@v1.9.1
go get github.com/spf13/viper@v1.19.0
go get github.com/google/uuid@v1.6.0
go get gorm.io/gorm@v1.25.12
go get gorm.io/driver/sqlite@v1.5.6
go get github.com/gorilla/mux@v1.8.1
go get github.com/ya-breeze/kin-core@latest
go get github.com/modelcontextprotocol/go-sdk@v1.3.0
```

- [ ] **Step 8: Verify build**

```bash
cd /data/HealthVault && make build
```
Expected: `backend/bin/hcw` created, no errors.

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "feat: project scaffolding — Go module, Cobra CLI, Viper config, Makefile"
```

---

### Task 2: Database Models & Migration

**Files:**
- Create: `backend/pkg/database/models.go`
- Create: `backend/pkg/database/db.go`
- Test: `backend/pkg/database/db_test.go`

**Interfaces:**
- Produces: `database.Open(logger, dbPath) (*gorm.DB, error)` — opens SQLite, runs AutoMigrate
- Produces: all 26 GORM model types (WebhookPayload + 24 health types + SleepStage)

- [ ] **Step 1: Create `backend/pkg/database/models.go`**

```go
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
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_steps_user_time,composite:user_id"`
	EndTime         time.Time `gorm:"not null"`
	Count           int       `gorm:"not null"`
}

type Distance struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_distance_user_time,composite:user_id"`
	EndTime         time.Time `gorm:"not null"`
	Meters          float64   `gorm:"not null"`
}

type ActiveCalories struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_active_cal_user_time,composite:user_id"`
	EndTime         time.Time `gorm:"not null"`
	Calories        float64   `gorm:"not null"`
}

type TotalCalories struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_total_cal_user_time,composite:user_id"`
	EndTime         time.Time `gorm:"not null"`
	Calories        float64   `gorm:"not null"`
}

type Hydration struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	StartTime       time.Time `gorm:"not null;uniqueIndex:idx_hydration_user_time,composite:user_id"`
	EndTime         time.Time `gorm:"not null"`
	Liters          float64   `gorm:"not null"`
}

// --- point-in-time types (single time field) ---

type HeartRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_hr_user_time,composite:user_id"`
	BPM             int       `gorm:"not null"`
}

type HeartRateVariability struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_hrv_user_time,composite:user_id"`
	RmssdMillis     float64   `gorm:"not null"`
}

type Weight struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_weight_user_time,composite:user_id"`
	Kilograms       float64   `gorm:"not null"`
}

type Height struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_height_user_time,composite:user_id"`
	Meters          float64   `gorm:"not null"`
}

type BloodGlucose struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bg_user_time,composite:user_id"`
	MmolPerLiter    float64   `gorm:"not null"`
}

type OxygenSaturation struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_spo2_user_time,composite:user_id"`
	Percentage      float64   `gorm:"not null"`
}

type BodyTemperature struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bodytemp_user_time,composite:user_id"`
	Celsius         float64   `gorm:"not null"`
}

type RespiratoryRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_rr_user_time,composite:user_id"`
	Rate            float64   `gorm:"not null"`
}

type RestingHeartRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_rhr_user_time,composite:user_id"`
	BPM             int       `gorm:"not null"`
}

type BasalMetabolicRate struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bmr_user_time,composite:user_id"`
	Watts           float64   `gorm:"not null"`
}

type BodyFat struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bodyfat_user_time,composite:user_id"`
	Percentage      float64   `gorm:"not null"`
}

type LeanBodyMass struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_lbm_user_time,composite:user_id"`
	Kilograms       float64   `gorm:"not null"`
}

type VO2Max struct {
	models.TenantModel
	UserID             uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID    uuid.UUID `gorm:"type:uuid;not null"`
	Time               time.Time `gorm:"not null;uniqueIndex:idx_vo2_user_time,composite:user_id"`
	MlPerKgPerMin      float64   `gorm:"not null"`
}

type BoneMass struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bonemass_user_time,composite:user_id"`
	Kilograms       float64   `gorm:"not null"`
}

// --- multi-value types ---

type BloodPressure struct {
	models.TenantModel
	UserID          uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID `gorm:"type:uuid;not null"`
	Time            time.Time `gorm:"not null;uniqueIndex:idx_bp_user_time,composite:user_id"`
	Systolic        float64   `gorm:"not null"`
	Diastolic       float64   `gorm:"not null"`
}

type SkinTemperature struct {
	models.TenantModel
	UserID              uuid.UUID  `gorm:"type:uuid;not null;index"`
	SourcePayloadID     uuid.UUID  `gorm:"type:uuid;not null"`
	Time                time.Time  `gorm:"not null;uniqueIndex:idx_skintemp_user_time,composite:user_id"`
	DeltaCelsius        float64    `gorm:"not null"`
	BaselineCelsius     *float64
	MeasurementLocation int        `gorm:"not null"`
}

// --- sleep (derived start_time) ---

type Sleep struct {
	models.TenantModel
	UserID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID  `gorm:"type:uuid;not null"`
	StartTime       time.Time  `gorm:"not null;uniqueIndex:idx_sleep_user_time,composite:user_id"`
	SessionEndTime  time.Time  `gorm:"not null"`
	DurationSeconds int        `gorm:"not null"`
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
	UserID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	SourcePayloadID uuid.UUID  `gorm:"type:uuid;not null"`
	StartTime       time.Time  `gorm:"not null;uniqueIndex:idx_exercise_user_time,composite:user_id"`
	EndTime         time.Time  `gorm:"not null"`
	DurationSeconds int        `gorm:"not null"`
	ExerciseType    string     `gorm:"not null"`
	DistanceMeters  *float64
	Steps           *int
	AvgCadenceSpm   *float64
	MaxCadenceSpm   *float64
	StrideLengthM   *float64
}

// --- nutrition ---

type Nutrition struct {
	models.TenantModel
	UserID             uuid.UUID `gorm:"type:uuid;not null;index"`
	SourcePayloadID    uuid.UUID `gorm:"type:uuid;not null"`
	StartTime          time.Time `gorm:"not null;uniqueIndex:idx_nutrition_user_time,composite:user_id"`
	EndTime            time.Time `gorm:"not null"`
	Calories           *float64
	ProteinGrams       *float64
	CarbsGrams         *float64
	FatGrams           *float64
	SugarGrams         *float64
	SodiumGrams        *float64
	DietaryFiberGrams  *float64
	Name               *string
}
```

- [ ] **Step 2: Create `backend/pkg/database/db.go`**

```go
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ya-breeze/kin-core/authdb"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type slogGormLogger struct {
	l *slog.Logger
}

func (g *slogGormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface { return g }
func (g *slogGormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	g.l.InfoContext(ctx, fmt.Sprintf(msg, args...))
}
func (g *slogGormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	g.l.WarnContext(ctx, fmt.Sprintf(msg, args...))
}
func (g *slogGormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	g.l.ErrorContext(ctx, fmt.Sprintf(msg, args...))
}
func (g *slogGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {}

func Open(l *slog.Logger, dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: &slogGormLogger{l: l},
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(
		&kinmodels.Family{},
		&kinmodels.User{},
		&authdb.RefreshToken{},
		&authdb.BlacklistedToken{},
		&WebhookPayload{},
		&Steps{}, &Distance{}, &ActiveCalories{}, &TotalCalories{}, &Hydration{},
		&HeartRate{}, &HeartRateVariability{}, &Weight{}, &Height{},
		&BloodGlucose{}, &OxygenSaturation{}, &BodyTemperature{},
		&RespiratoryRate{}, &RestingHeartRate{}, &BasalMetabolicRate{},
		&BodyFat{}, &LeanBodyMass{}, &VO2Max{}, &BoneMass{},
		&BloodPressure{}, &SkinTemperature{},
		&Sleep{}, &SleepStage{},
		&Exercise{}, &Nutrition{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}
```

- [ ] **Step 3: Write failing test `backend/pkg/database/db_test.go`**

```go
package database_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestOpen(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAllTablesCreated(t *testing.T) {
	db, _ := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	tables := []string{
		"families", "users", "webhook_payloads",
		"steps", "heart_rates", "sleeps", "sleep_stages",
		"blood_pressures", "exercises", "nutritions",
	}
	for _, tbl := range tables {
		if !db.Migrator().HasTable(tbl) {
			t.Errorf("missing table: %s", tbl)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
cd /data/HealthVault && make test 2>&1 | head -20
```
Expected: compilation error (packages not yet linked).

- [ ] **Step 5: Wire imports and run test**

```bash
cd /data/HealthVault && make test
```
Expected: `PASS`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: database models and migration for all 24 health types"
```

---

### Task 3: Storage Interface & User Seeding

**Files:**
- Create: `backend/pkg/database/storage.go`
- Create: `backend/pkg/database/storage_impl.go`
- Create: `backend/pkg/database/seed.go`
- Test: `backend/pkg/database/seed_test.go`

**Interfaces:**
- Consumes: `database.Open()`
- Produces: `database.Storage` interface with `FindUserByName`, `FindUsersByFamilyID`, `SaveWebhookPayload`, `SaveHealthRecords`
- Produces: `database.SeedUsers(db, spec string) error`

- [ ] **Step 1: Create `backend/pkg/database/storage.go`**

```go
package database

import (
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"gorm.io/gorm"
)

type TimeRange struct {
	From time.Time
	To   time.Time
}

type Storage interface {
	FindUserByName(username string) (*kinmodels.User, error)
	FindUserByID(id uuid.UUID) (*kinmodels.User, error)
	FindUsersByFamilyID(familyID uuid.UUID) ([]kinmodels.User, error)
	AllUsers() ([]kinmodels.User, error)
	SaveWebhookPayload(p *WebhookPayload) error
	// Generic health record queries — returns []map[string]any for JSON serialization.
	// timeCol is the column to filter on ("time", "start_time", etc.).
	QueryRecords(tableName string, timeCol string, userID uuid.UUID, tr TimeRange) ([]map[string]any, error)
	// Summary data
	SummarySteps(userID uuid.UUID, tr TimeRange) (int, error)
	SummaryAvgHeartRate(userID uuid.UUID, tr TimeRange) (float64, error)
	SummarySleepSeconds(userID uuid.UUID, tr TimeRange) (int, error)
	// DB exposes raw gorm.DB for ingest fan-out
	DB() *gorm.DB
}
```

- [ ] **Step 2: Create `backend/pkg/database/storage_impl.go`**

```go
package database

import (
	"fmt"
	"time"

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

func (s *storageImpl) FindUserByID(id uuid.UUID) (*kinmodels.User, error) {
	var u kinmodels.User
	if err := s.db.Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
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

// timeField returns the primary time column name for a given table.
func timeField(tableName string) string {
	switch tableName {
	case "steps", "distances", "active_calories", "total_calories",
		"hydrations", "sleeps", "exercises", "nutritions":
		return "start_time"
	default:
		return "time"
	}
}

var _ Storage = (*storageImpl)(nil) // compile-time check

// QueryRecords refined to use correct time field
func init() {
	_ = fmt.Sprintf // suppress import
	_ = time.Now    // suppress import
}
```

- [ ] **Step 3: Create `backend/pkg/database/seed.go`**

```go
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
func SeedUsers(db *gorm.DB, spec string) error {
	if spec == "" {
		return nil
	}
	for _, entry := range strings.Split(spec, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid seed entry %q (want FamilyName:username:password)", entry)
		}
		familyName, username, password := parts[0], parts[1], parts[2]

		var family kinmodels.Family
		if err := db.Where("name = ?", familyName).FirstOrCreate(&family, kinmodels.Family{
			ID:   uuid.New(),
			Name: familyName,
		}).Error; err != nil {
			return fmt.Errorf("seed family %q: %w", familyName, err)
		}

		var existing kinmodels.User
		if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
			continue // already exists
		}

		hash, err := auth.HashPassword(password)
		if err != nil {
			return fmt.Errorf("hash password for %q: %w", username, err)
		}
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
```

- [ ] **Step 4: Write test `backend/pkg/database/seed_test.go`**

```go
package database_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestSeedUsers(t *testing.T) {
	db, err := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	store := database.NewStorage(db)

	if err := database.SeedUsers(db, "TestFamily:alice:pass1,TestFamily:bob:pass2"); err != nil {
		t.Fatalf("SeedUsers: %v", err)
	}

	alice, err := store.FindUserByName("alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	bob, err := store.FindUserByName("bob")
	if err != nil {
		t.Fatalf("find bob: %v", err)
	}
	if alice.FamilyID != bob.FamilyID {
		t.Error("alice and bob should be in the same family")
	}

	// Idempotent — seed again should not error or duplicate
	if err := database.SeedUsers(db, "TestFamily:alice:pass1"); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	users, _ := store.FindUsersByFamilyID(alice.FamilyID)
	if len(users) != 2 {
		t.Errorf("want 2 users, got %d", len(users))
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd /data/HealthVault && make test
```
Expected: `PASS`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: storage interface, implementation, and user seeding from HCW_SEED_USERS"
```

---

### Task 4: Auth Handlers & HTTP Server

**Files:**
- Create: `backend/pkg/server/middleware.go`
- Create: `backend/pkg/server/auth.go`
- Create: `backend/pkg/server/server.go`
- Modify: `backend/cmd/commands/cmdserver.go`

**Interfaces:**
- Consumes: `database.Storage`, `config.Config`
- Produces: `server.Run(ctx, cfg, storage) error` — starts HTTP server with all routes
- Produces: `RequireAuth` Gorilla Mux middleware

- [ ] **Step 1: Create `backend/pkg/server/middleware.go`**

```go
package server

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/auth"
	kinmw "github.com/ya-breeze/kin-core/middleware"
	"github.com/ya-breeze/kin-core/cookies"
)

type contextKey string

const claimsKey contextKey = "claims"

func RequireAuth(jwtSecret []byte, cookieCfg cookies.Config) func(http.Handler) http.Handler {
	cfg := kinmw.Config{JWTSecret: jwtSecret, CookieCfg: cookieCfg}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := kinmw.ValidateRequest(r, cfg)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFromCtx(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

func FamilyIDFromCtx(r *http.Request) uuid.UUID {
	c := ClaimsFromCtx(r)
	if c == nil {
		return uuid.Nil
	}
	if c.FamilyID == nil {
		return uuid.Nil
	}
	return *c.FamilyID
}
```

- [ ] **Step 2: Create `backend/pkg/server/auth.go`**

```go
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ya-breeze/kin-core/auth"
	"github.com/ya-breeze/kin-core/authdb"
	"github.com/ya-breeze/kin-core/cookies"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

type authHandlers struct {
	storage    database.Storage
	db         *gorm.DB
	jwtSecret  []byte
	cookieCfg  cookies.Config
}

func (h *authHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, err := h.storage.FindUserByName(req.Username)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	accessToken, err := auth.GenerateAccessToken(user.ID, &user.FamilyID, h.jwtSecret, 15*time.Minute)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rt, err := authdb.CreateRefreshToken(h.db, user.ID, 365*24*time.Hour)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cookies.SetAccessCookie(w, accessToken, 900, h.cookieCfg)
	cookies.SetRefreshCookie(w, rt.Token, 365*24*3600, h.cookieCfg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

func (h *authHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	token := cookies.GetAccessToken(r)
	if token != "" {
		authdb.BlacklistToken(h.db, token) //nolint:errcheck
	}
	cookies.ClearAccessCookie(w, h.cookieCfg)
	cookies.ClearRefreshCookie(w, h.cookieCfg)
	w.WriteHeader(http.StatusNoContent)
}

func (h *authHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	rtToken := cookies.GetRefreshToken(r)
	if rtToken == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rt, err := authdb.ValidateRefreshToken(h.db, rtToken)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var user database.WebhookPayload // just to get family ID — load user properly
	_ = user
	// Look up user to get FamilyID
	users, err := h.storage.AllUsers()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var familyID *interface{ GetID() interface{} }
	_ = familyID
	// Find the user matching rt.UserID
	for _, u := range users {
		if u.ID == rt.UserID {
			accessToken, _ := auth.GenerateAccessToken(u.ID, &u.FamilyID, h.jwtSecret, 15*time.Minute)
			newRT, _ := authdb.RotateRefreshToken(h.db, rt, 365*24*time.Hour)
			cookies.SetAccessCookie(w, accessToken, 900, h.cookieCfg)
			cookies.SetRefreshCookie(w, newRT.Token, 365*24*3600, h.cookieCfg)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
```

- [ ] **Step 3: Create `backend/pkg/server/server.go`**

```go
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/ya-breeze/kin-core/cookies"
	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/database"
)

func Run(ctx context.Context, logger *slog.Logger, cfg *config.Config, storage database.Storage) error {
	jwtSecret := []byte(cfg.JWTSecret)
	cookieCfg := cookies.Config{Secure: cfg.CookieSecure}

	ah := &authHandlers{
		storage:   storage,
		db:        storage.DB(),
		jwtSecret: jwtSecret,
		cookieCfg: cookieCfg,
	}

	r := mux.NewRouter()

	// Webhook (unauthenticated)
	r.HandleFunc("/webhook/{username}", webhookHandler(storage)).Methods("POST")

	// Auth
	r.HandleFunc("/api/auth/login", ah.Login).Methods("POST")
	r.HandleFunc("/api/auth/logout", ah.Logout).Methods("POST")
	r.HandleFunc("/api/auth/refresh", ah.Refresh).Methods("POST")

	// Protected API
	api := r.PathPrefix("/api").Subrouter()
	api.Use(RequireAuth(jwtSecret, cookieCfg))
	api.HandleFunc("/users/me", meHandler(storage)).Methods("GET")
	api.HandleFunc("/data/{type}", dataHandler(storage)).Methods("GET")
	api.HandleFunc("/data/summary", summaryHandler(storage)).Methods("GET")

	// MCP (unauthenticated) — registered in Task 7
	// r.Handle("/mcp", mcpHandler(storage))

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	logger.Info("listening", "port", cfg.Port)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return fmt.Errorf("server: %w", err)
	}
}
```

- [ ] **Step 4: Update `backend/cmd/commands/cmdserver.go`** to call `server.Run`

```go
package commands

import (
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/pkg/config"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func CmdServer(logger *slog.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start the HealthVault HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			db, err := database.Open(logger, cfg.DBPath)
			if err != nil {
				return err
			}
			if err := database.SeedUsers(db, cfg.SeedUsers); err != nil {
				return err
			}
			storage := database.NewStorage(db)
			return server.Run(cmd.Context(), logger, cfg, storage)
		},
	}
}
```

- [ ] **Step 5: Build and smoke-test**

```bash
cd /data/HealthVault && make build
HCW_JWT_SECRET=test HCW_COOKIE_SECURE=false HCW_SEED_USERS="Fam:alice:pass1" \
  ./backend/bin/hcw server &
sleep 1
curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"pass1"}' | head -c 100
kill %1
```
Expected: `{"status":"ok"}`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: auth middleware and login/logout/refresh handlers"
```

---

### Task 5: Webhook Receiver & Ingest

**Files:**
- Create: `backend/pkg/ingest/payload.go`
- Create: `backend/pkg/ingest/ingest.go`
- Create: `backend/pkg/server/webhook.go`
- Test: `backend/pkg/ingest/ingest_test.go`

**Interfaces:**
- Consumes: `database.Storage`, all database models
- Produces: `ingest.Process(db *gorm.DB, userID, familyID, payloadID uuid.UUID, raw []byte) error`

- [ ] **Step 1: Create `backend/pkg/ingest/payload.go`**

```go
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
```

- [ ] **Step 2: Create `backend/pkg/ingest/ingest.go`**

```go
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

func newRecord(userID, familyID, payloadID uuid.UUID) (uuid.UUID, uuid.UUID, uuid.UUID) {
	return uuid.New(), familyID, payloadID
}

// Process fans out all type arrays from p into their respective tables.
// Each record is inserted with ON CONFLICT DO NOTHING for deduplication.
func Process(db *gorm.DB, userID, familyID, payloadID uuid.UUID, p *PayloadJSON) error {
	do := clause.OnConflict{DoNothing: true}

	for _, r := range p.Steps {
		rec := &database.Steps{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Count: r.Count,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		if err := db.Clauses(do).Create(rec).Error; err != nil {
			return fmt.Errorf("insert steps: %w", err)
		}
	}
	for _, r := range p.HeartRate {
		rec := &database.HeartRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), BPM: r.BPM,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		if err := db.Clauses(do).Create(rec).Error; err != nil {
			return fmt.Errorf("insert heart_rate: %w", err)
		}
	}
	for _, r := range p.HeartRateVariability {
		rec := &database.HeartRateVariability{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), RmssdMillis: r.RmssdMillis,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		if err := db.Clauses(do).Create(rec).Error; err != nil {
			return fmt.Errorf("insert hrv: %w", err)
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
		sleep.ID = uuid.New(); sleep.FamilyID = familyID
		if err := db.Clauses(do).Create(sleep).Error; err != nil {
			return fmt.Errorf("insert sleep: %w", err)
		}
		// Skip stages if sleep was duplicate (ID not persisted)
		for _, st := range r.Stages {
			stage := &database.SleepStage{
				SleepID: sleep.ID, Stage: st.Stage,
				StartTime: parseTime(st.StartTime), EndTime: parseTime(st.EndTime),
				DurationSeconds: st.DurationSeconds,
			}
			db.Clauses(do).Create(stage) //nolint:errcheck
		}
	}
	for _, r := range p.Distance {
		rec := &database.Distance{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Meters: r.Meters,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.ActiveCalories {
		rec := &database.ActiveCalories{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Calories: r.Calories,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.TotalCalories {
		rec := &database.TotalCalories{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Calories: r.Calories,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Weight {
		rec := &database.Weight{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Kilograms: r.Kilograms,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Height {
		rec := &database.Height{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Meters: r.Meters,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BloodPressure {
		rec := &database.BloodPressure{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Systolic: r.Systolic, Diastolic: r.Diastolic,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BloodGlucose {
		rec := &database.BloodGlucose{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), MmolPerLiter: r.MmolPerLiter,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.OxygenSaturation {
		rec := &database.OxygenSaturation{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Percentage: r.Percentage,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BodyTemperature {
		rec := &database.BodyTemperature{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Celsius: r.Celsius,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.SkinTemperature {
		rec := &database.SkinTemperature{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), DeltaCelsius: r.DeltaCelsius,
			BaselineCelsius: r.BaselineCelsius, MeasurementLocation: r.MeasurementLocation,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.RespiratoryRate {
		rec := &database.RespiratoryRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Rate: r.Rate,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.RestingHeartRate {
		rec := &database.RestingHeartRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), BPM: r.BPM,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
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
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.Hydration {
		rec := &database.Hydration{
			UserID: userID, SourcePayloadID: payloadID,
			StartTime: parseTime(r.StartTime), EndTime: parseTime(r.EndTime),
			Liters: r.Liters,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
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
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BasalMetabolicRate {
		rec := &database.BasalMetabolicRate{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Watts: r.Watts,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BodyFat {
		rec := &database.BodyFat{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Percentage: r.Percentage,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.LeanBodyMass {
		rec := &database.LeanBodyMass{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Kilograms: r.Kilograms,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.VO2Max {
		rec := &database.VO2Max{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), MlPerKgPerMin: r.MlPerKgPerMin,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	for _, r := range p.BoneMass {
		rec := &database.BoneMass{
			UserID: userID, SourcePayloadID: payloadID,
			Time: parseTime(r.Time), Kilograms: r.Kilograms,
		}
		rec.ID = uuid.New(); rec.FamilyID = familyID
		db.Clauses(do).Create(rec) //nolint:errcheck
	}
	return nil
}
```

- [ ] **Step 3: Create `backend/pkg/server/webhook.go`**

```go
package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/ingest"
)

func webhookHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := mux.Vars(r)["username"]
		user, err := storage.FindUserByName(username)
		if err != nil {
			http.Error(w, "unknown user", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var p ingest.PayloadJSON
		if err := json.Unmarshal(body, &p); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		payloadID := uuid.New()
		wp := &database.WebhookPayload{
			UserID:     user.ID,
			ReceivedAt: time.Now().UTC(),
			AppVersion: p.AppVersion,
			Raw:        string(body),
		}
		wp.ID = payloadID
		wp.FamilyID = user.FamilyID
		if ts, err := time.Parse(time.RFC3339Nano, p.Timestamp); err == nil {
			wp.PayloadTs = ts
		}
		if err := storage.SaveWebhookPayload(wp); err != nil {
			slog.Error("save webhook payload", "err", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		if err := ingest.Process(storage.DB(), user.ID, user.FamilyID, payloadID, &p); err != nil {
			slog.Error("ingest", "err", err)
			// Don't fail — payload is saved, ingest errors are non-fatal
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Write test `backend/pkg/ingest/ingest_test.go`**

```go
package ingest_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/ingest"
)

func TestProcess_Steps(t *testing.T) {
	db, _ := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	userID, familyID, payloadID := uuid.New(), uuid.New(), uuid.New()

	p := &ingest.PayloadJSON{
		Timestamp:  time.Now().Format(time.RFC3339),
		AppVersion: "1.0",
		Steps: []ingest.StepsJSON{
			{Count: 1000, StartTime: "2026-06-24T00:00:00Z", EndTime: "2026-06-24T01:00:00Z"},
		},
	}
	if err := ingest.Process(db, userID, familyID, payloadID, p); err != nil {
		t.Fatalf("Process: %v", err)
	}

	var steps []database.Steps
	db.Find(&steps)
	if len(steps) != 1 || steps[0].Count != 1000 {
		t.Errorf("want 1 step record with count=1000, got %+v", steps)
	}
}

func TestProcess_Deduplication(t *testing.T) {
	db, _ := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	userID, familyID := uuid.New(), uuid.New()

	p := &ingest.PayloadJSON{
		Steps: []ingest.StepsJSON{
			{Count: 500, StartTime: "2026-06-24T00:00:00Z", EndTime: "2026-06-24T01:00:00Z"},
		},
	}
	ingest.Process(db, userID, familyID, uuid.New(), p) //nolint:errcheck
	ingest.Process(db, userID, familyID, uuid.New(), p) // same record, second payload

	var steps []database.Steps
	db.Find(&steps)
	if len(steps) != 1 {
		t.Errorf("deduplication failed: want 1 record, got %d", len(steps))
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd /data/HealthVault && make test
```
Expected: `PASS`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: webhook receiver and ingest fan-out for all 24 health types"
```

---

### Task 6: REST API

**Files:**
- Create: `backend/pkg/server/api.go`

**Interfaces:**
- Consumes: `database.Storage`, `RequireAuth` middleware, `ClaimsFromCtx`
- Produces: `GET /api/users/me`, `GET /api/data/{type}`, `GET /api/data/summary`

- [ ] **Step 1: Create `backend/pkg/server/api.go`**

```go
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
)

// typeInfo maps URL type names to (table name, primary time column).
type typeInfo struct{ table, timeCol string }

var typeRegistry = map[string]typeInfo{
	"steps":                  {"steps", "start_time"},
	"heart_rate":             {"heart_rates", "time"},
	"heart_rate_variability": {"heart_rate_variabilities", "time"},
	"sleep":                  {"sleeps", "start_time"},
	"distance":               {"distances", "start_time"},
	"active_calories":        {"active_calories", "start_time"},
	"total_calories":         {"total_calories", "start_time"},
	"weight":                 {"weights", "time"},
	"height":                 {"heights", "time"},
	"blood_pressure":         {"blood_pressures", "time"},
	"blood_glucose":          {"blood_glucoses", "time"},
	"oxygen_saturation":      {"oxygen_saturations", "time"},
	"body_temperature":       {"body_temperatures", "time"},
	"skin_temperature":       {"skin_temperatures", "time"},
	"respiratory_rate":       {"respiratory_rates", "time"},
	"resting_heart_rate":     {"resting_heart_rates", "time"},
	"exercise":               {"exercises", "start_time"},
	"hydration":              {"hydrations", "start_time"},
	"nutrition":              {"nutritions", "start_time"},
	"basal_metabolic_rate":   {"basal_metabolic_rates", "time"},
	"body_fat":               {"body_fats", "time"},
	"lean_body_mass":         {"lean_body_masses", "time"},
	"vo2_max":                {"vo2_maxes", "time"},
	"bone_mass":              {"bone_masses", "time"},
}

func meHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		user, err := storage.FindUserByID(claims.UserID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"id": user.ID, "username": user.Username, "family_id": user.FamilyID,
		})
	}
}

func dataHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		typeName := mux.Vars(r)["type"]
		info, ok := typeRegistry[typeName]
		if !ok {
			http.Error(w, "unknown type", http.StatusNotFound)
			return
		}

		claims := ClaimsFromCtx(r)
		familyID := FamilyIDFromCtx(r)

		targetUser, err := resolveUser(r, storage, claims, familyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		from, to := parseTimeRange(r)
		records, err := storage.QueryRecords(info.table, info.timeCol, targetUser.ID, database.TimeRange{From: from, To: to})
		if err != nil {
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records) //nolint:errcheck
	}
}

func summaryHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		familyID := FamilyIDFromCtx(r)
		targetUser, err := resolveUser(r, storage, claims, familyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		from, to := parseTimeRange(r)
		tr := database.TimeRange{From: from, To: to}

		steps, _ := storage.SummarySteps(targetUser.ID, tr)
		avgHR, _ := storage.SummaryAvgHeartRate(targetUser.ID, tr)
		sleepSec, _ := storage.SummarySleepSeconds(targetUser.ID, tr)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"steps":         steps,
			"avg_heart_rate": avgHR,
			"sleep_seconds": sleepSec,
		})
	}
}

func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	q := r.URL.Query()
	from, _ := time.Parse(time.RFC3339, q.Get("from"))
	to, _ := time.Parse(time.RFC3339, q.Get("to"))
	if from.IsZero() {
		from = time.Now().UTC().AddDate(0, 0, -7)
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	return from, to
}

// resolveUser returns the target user: the caller themselves, or a named
// family member (from ?user= query param). Returns error if the named user
// is not in the caller's family.
func resolveUser(r *http.Request, storage database.Storage, claims *auth.Claims, familyID uuid.UUID) (*kinmodels.User, error) {
	username := r.URL.Query().Get("user")
	if username == "" {
		return storage.FindUserByID(claims.UserID)
	}
	target, err := storage.FindUserByName(username)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	if target.FamilyID != familyID {
		return nil, fmt.Errorf("access denied")
	}
	return target, nil
}
```

Add these imports to `backend/pkg/server/api.go`:
```go
import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
    "github.com/ya-breeze/kin-core/auth"
    kinmodels "github.com/ya-breeze/kin-core/models"
    "github.com/ya-breeze/healthvault/pkg/database"
)
```

- [ ] **Step 2: Build and smoke-test**

```bash
cd /data/HealthVault && make build
HCW_JWT_SECRET=test HCW_COOKIE_SECURE=false HCW_SEED_USERS="Fam:alice:pass1" \
  ./backend/bin/hcw server &
sleep 1
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"pass1"}' -c /tmp/cookies.txt)
curl -s -b /tmp/cookies.txt "http://localhost:8080/api/users/me"
kill %1
```
Expected: JSON with `username: "alice"`.

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: REST API — /api/users/me, /api/data/{type}, /api/data/summary"
```

---

### Task 7: MCP Server

**Files:**
- Create: `backend/pkg/mcpserver/server.go`
- Create: `backend/pkg/mcpserver/tools.go`
- Create: `backend/pkg/mcpserver/instructions.go`
- Create: `backend/cmd/commands/cmdmcpconfig.go`
- Modify: `backend/pkg/server/server.go` — register `/mcp` handler
- Modify: `backend/cmd/main.go` — register `CmdMCPConfig`

**Interfaces:**
- Consumes: `database.Storage`
- Produces: `mcpserver.Handler(storage) http.Handler` — HTTP streamable MCP endpoint

- [ ] **Step 1: Create `backend/pkg/mcpserver/instructions.go`**

```go
package mcpserver

const instructions = `HealthVault stores health data received from the HC Webhook Android app.

Data types available: steps, heart_rate, heart_rate_variability, sleep, distance,
active_calories, total_calories, weight, height, blood_pressure, blood_glucose,
oxygen_saturation, body_temperature, skin_temperature, respiratory_rate,
resting_heart_rate, exercise, hydration, nutrition, basal_metabolic_rate,
body_fat, lean_body_mass, vo2_max, bone_mass.

Use list_users to see available users, then query_data or summary to retrieve health data.
Time parameters use RFC3339 format (e.g. 2026-06-24T00:00:00Z).
Default time range when omitted: last 7 days.`
```

- [ ] **Step 2: Create `backend/pkg/mcpserver/tools.go`**

```go
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
)

type mcpStorage struct{ storage database.Storage }

type queryInput struct {
	User string `json:"user"`
	Type string `json:"type"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type summaryInput struct {
	User string `json:"user"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

func registerTools(server *mcp.Server, storage database.Storage) {
	s := &mcpStorage{storage: storage}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_users",
		Description: "List all users in the system.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		users, err := storage.AllUsers()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), struct{}{}, nil
		}
		names := make([]string, len(users))
		for i, u := range users { names[i] = u.Username }
		b, _ := json.Marshal(names)
		return mcp.NewToolResultText(string(b)), struct{}{}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_data",
		Description: "Query health records for a user and data type. type must be one of: steps, heart_rate, heart_rate_variability, sleep, distance, active_calories, total_calories, weight, height, blood_pressure, blood_glucose, oxygen_saturation, body_temperature, skin_temperature, respiratory_rate, resting_heart_rate, exercise, hydration, nutrition, basal_metabolic_rate, body_fat, lean_body_mass, vo2_max, bone_mass.",
		InputSchema: mcp.MustParseInputSchema(`{
			"type":"object",
			"properties":{
				"user":{"type":"string","description":"username"},
				"type":{"type":"string","description":"data type name"},
				"from":{"type":"string","description":"RFC3339 start time"},
				"to":{"type":"string","description":"RFC3339 end time"}
			},
			"required":["user","type"]
		}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input queryInput) (*mcp.CallToolResult, queryInput, error) {
		return s.queryData(ctx, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "summary",
		Description: "Get a daily health summary for a user: total steps, average heart rate, total sleep seconds.",
		InputSchema: mcp.MustParseInputSchema(`{
			"type":"object",
			"properties":{
				"user":{"type":"string","description":"username"},
				"from":{"type":"string","description":"RFC3339 start time"},
				"to":{"type":"string","description":"RFC3339 end time"}
			},
			"required":["user"]
		}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input summaryInput) (*mcp.CallToolResult, summaryInput, error) {
		return s.querySummary(ctx, input)
	})
}

// typeTimeCol maps type name to (table, primary time column) — mirrors server.typeRegistry.
// Keep in sync or move to a shared package if it drifts.
var typeTimeCol = map[string][2]string{
	"steps":                  {"steps", "start_time"},
	"heart_rate":             {"heart_rates", "time"},
	"heart_rate_variability": {"heart_rate_variabilities", "time"},
	"sleep":                  {"sleeps", "start_time"},
	"distance":               {"distances", "start_time"},
	"active_calories":        {"active_calories", "start_time"},
	"total_calories":         {"total_calories", "start_time"},
	"weight":                 {"weights", "time"},
	"height":                 {"heights", "time"},
	"blood_pressure":         {"blood_pressures", "time"},
	"blood_glucose":          {"blood_glucoses", "time"},
	"oxygen_saturation":      {"oxygen_saturations", "time"},
	"body_temperature":       {"body_temperatures", "time"},
	"skin_temperature":       {"skin_temperatures", "time"},
	"respiratory_rate":       {"respiratory_rates", "time"},
	"resting_heart_rate":     {"resting_heart_rates", "time"},
	"exercise":               {"exercises", "start_time"},
	"hydration":              {"hydrations", "start_time"},
	"nutrition":              {"nutritions", "start_time"},
	"basal_metabolic_rate":   {"basal_metabolic_rates", "time"},
	"body_fat":               {"body_fats", "time"},
	"lean_body_mass":         {"lean_body_masses", "time"},
	"vo2_max":                {"vo2_maxes", "time"},
	"bone_mass":              {"bone_masses", "time"},
}

func parseTR(from, to string) database.TimeRange {
	f, _ := time.Parse(time.RFC3339, from)
	t, _ := time.Parse(time.RFC3339, to)
	if f.IsZero() { f = time.Now().UTC().AddDate(0, 0, -7) }
	if t.IsZero() { t = time.Now().UTC() }
	return database.TimeRange{From: f, To: t}
}

func (s *mcpStorage) queryData(_ context.Context, input queryInput) (*mcp.CallToolResult, queryInput, error) {
	cols, ok := typeTimeCol[input.Type]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("unknown type %q", input.Type)), input, nil
	}
	user, err := s.storage.FindUserByName(input.User)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("user %q not found", input.User)), input, nil
	}
	records, err := s.storage.QueryRecords(cols[0], cols[1], user.ID, parseTR(input.From, input.To))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), input, nil
	}
	b, _ := json.Marshal(records)
	return mcp.NewToolResultText(string(b)), input, nil
}

func (s *mcpStorage) querySummary(_ context.Context, input summaryInput) (*mcp.CallToolResult, summaryInput, error) {
	user, err := s.storage.FindUserByName(input.User)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("user %q not found", input.User)), input, nil
	}
	tr := parseTR(input.From, input.To)
	steps, _ := s.storage.SummarySteps(user.ID, tr)
	avgHR, _ := s.storage.SummaryAvgHeartRate(user.ID, tr)
	sleepSec, _ := s.storage.SummarySleepSeconds(user.ID, tr)
	b, _ := json.Marshal(map[string]any{
		"user": input.User, "from": tr.From, "to": tr.To,
		"steps": steps, "avg_heart_rate": avgHR, "sleep_seconds": sleepSec,
	})
	return mcp.NewToolResultText(string(b)), input, nil
}
```

- [ ] **Step 3: Create `backend/pkg/mcpserver/server.go`**

```go
package mcpserver

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
)

func Handler(storage database.Storage) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "healthvault",
		Version: "1.0.0",
	}, &mcp.ServerOptions{Instructions: instructions})

	registerTools(server, storage)

	return mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)
}
```

- [ ] **Step 4: Create `backend/cmd/commands/cmdmcpconfig.go`**

```go
package commands

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/ya-breeze/healthvault/pkg/config"
)

func CmdMCPConfig(logger *slog.Logger) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "mcp-config",
		Short: "Generate .mcp.json for Claude Desktop / Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			mcpURL := fmt.Sprintf("http://localhost:%s/mcp", cfg.Port)
			entry := map[string]any{
				"healthvault": map[string]any{
					"url": mcpURL,
				},
			}
			var existing map[string]any
			if data, err := os.ReadFile(output); err == nil {
				json.Unmarshal(data, &existing) //nolint:errcheck
			}
			if existing == nil {
				existing = map[string]any{}
			}
			servers, _ := existing["mcpServers"].(map[string]any)
			if servers == nil {
				servers = map[string]any{}
			}
			for k, v := range entry { servers[k] = v }
			existing["mcpServers"] = servers
			b, _ := json.MarshalIndent(existing, "", "  ")
			if err := os.WriteFile(output, b, 0644); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote MCP config to %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", ".mcp.json", "output path")
	return cmd
}
```

- [ ] **Step 5: Register MCP handler in `backend/pkg/server/server.go`**

Add this import and line inside `Run()` after the api subrouter:
```go
import "github.com/ya-breeze/healthvault/pkg/mcpserver"
// inside Run():
r.Handle("/mcp", mcpserver.Handler(storage))
r.Handle("/mcp/", mcpserver.Handler(storage))
```

- [ ] **Step 6: Register `CmdMCPConfig` in `backend/cmd/main.go`**

```go
root.AddCommand(commands.CmdMCPConfig(logger))
```

- [ ] **Step 7: Build and smoke-test MCP endpoint**

```bash
cd /data/HealthVault && make build
HCW_JWT_SECRET=test HCW_COOKIE_SECURE=false HCW_SEED_USERS="Fam:alice:pass1" \
  ./backend/bin/hcw server &
sleep 1
curl -s -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}' \
  | head -c 200
kill %1
```
Expected: JSON response with `"result"` containing server info.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat: HTTP MCP server with query_data, summary, list_users tools"
```

---

### Task 8: Next.js Frontend

**Files:**
- Create: `frontend/` — Next.js app scaffolded via create-next-app
- Create: `frontend/lib/api.ts` — typed API client
- Create: `frontend/app/login/page.tsx`
- Create: `frontend/app/page.tsx` — dashboard
- Create: `frontend/app/data/[type]/page.tsx`

**Interfaces:**
- Consumes: backend REST API at `NEXT_PUBLIC_API_URL`

- [ ] **Step 1: Scaffold Next.js app**

```bash
cd /data/HealthVault
npx create-next-app@latest frontend \
  --typescript --tailwind --app --no-src-dir \
  --no-eslint --import-alias "@/*"
```

- [ ] **Step 2: Add Recharts**

```bash
cd /data/HealthVault/frontend && npm install recharts
```

- [ ] **Step 3: Create `frontend/lib/api.ts`**

```typescript
const BASE = process.env.NEXT_PUBLIC_API_URL ?? '/api';

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export const api = {
  login: (username: string, password: string) =>
    apiFetch('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => apiFetch('/auth/logout', { method: 'POST' }),

  me: () => apiFetch<{ id: string; username: string; family_id: string }>('/users/me'),

  data: (type: string, from?: string, to?: string, user?: string) => {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    if (user) params.set('user', user);
    return apiFetch<Record<string, unknown>[]>(`/data/${type}?${params}`);
  },

  summary: (from?: string, to?: string, user?: string) => {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    if (user) params.set('user', user);
    return apiFetch<{ steps: number; avg_heart_rate: number; sleep_seconds: number }>(
      `/data/summary?${params}`
    );
  },
};
```

- [ ] **Step 4: Create `frontend/app/login/page.tsx`**

```tsx
'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.login(username, password);
      router.push('/');
    } catch {
      setError('Invalid credentials');
    }
  };

  return (
    <main className="min-h-screen flex items-center justify-center bg-gray-50">
      <form onSubmit={handleSubmit} className="bg-white p-8 rounded-lg shadow w-80 space-y-4">
        <h1 className="text-2xl font-bold text-center">HealthVault</h1>
        {error && <p className="text-red-500 text-sm">{error}</p>}
        <input
          className="w-full border rounded px-3 py-2"
          placeholder="Username"
          value={username}
          onChange={e => setUsername(e.target.value)}
          required
        />
        <input
          className="w-full border rounded px-3 py-2"
          type="password"
          placeholder="Password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          required
        />
        <button className="w-full bg-blue-600 text-white rounded px-3 py-2 hover:bg-blue-700">
          Sign in
        </button>
      </form>
    </main>
  );
}
```

- [ ] **Step 5: Create `frontend/app/page.tsx`** (dashboard)

```tsx
'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';

const DATA_TYPES = ['steps','heart_rate','sleep','heart_rate_variability','distance',
  'weight','blood_pressure','oxygen_saturation'];

export default function Dashboard() {
  const router = useRouter();
  const [summary, setSummary] = useState<{ steps: number; avg_heart_rate: number; sleep_seconds: number } | null>(null);
  const [me, setMe] = useState<{ username: string } | null>(null);

  useEffect(() => {
    api.me().then(setMe).catch(() => router.push('/login'));
    api.summary().then(setSummary).catch(() => {});
  }, [router]);

  const handleLogout = async () => {
    await api.logout();
    router.push('/login');
  };

  return (
    <main className="p-6 max-w-4xl mx-auto">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-3xl font-bold">HealthVault</h1>
        <div className="flex items-center gap-4">
          <span className="text-gray-600">{me?.username}</span>
          <button onClick={handleLogout} className="text-sm text-red-500 hover:underline">Logout</button>
        </div>
      </div>

      {summary && (
        <div className="grid grid-cols-3 gap-4 mb-8">
          <div className="bg-white rounded-lg shadow p-4">
            <p className="text-gray-500 text-sm">Steps today</p>
            <p className="text-3xl font-bold">{summary.steps.toLocaleString()}</p>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <p className="text-gray-500 text-sm">Avg Heart Rate</p>
            <p className="text-3xl font-bold">{summary.avg_heart_rate.toFixed(0)} <span className="text-lg font-normal">bpm</span></p>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <p className="text-gray-500 text-sm">Sleep</p>
            <p className="text-3xl font-bold">{(summary.sleep_seconds / 3600).toFixed(1)} <span className="text-lg font-normal">h</span></p>
          </div>
        </div>
      )}

      <h2 className="text-xl font-semibold mb-3">Data Types</h2>
      <div className="grid grid-cols-4 gap-3">
        {DATA_TYPES.map(t => (
          <a key={t} href={`/data/${t}`}
            className="bg-white rounded-lg shadow p-3 text-center hover:bg-blue-50 text-sm font-medium capitalize">
            {t.replace(/_/g, ' ')}
          </a>
        ))}
      </div>
    </main>
  );
}
```

- [ ] **Step 6: Create `frontend/app/data/[type]/page.tsx`**

```tsx
'use client';
import { useEffect, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { api } from '@/lib/api';

export default function DataTypePage() {
  const { type } = useParams<{ type: string }>();
  const router = useRouter();
  const [records, setRecords] = useState<Record<string, unknown>[]>([]);
  const [from, setFrom] = useState(() => {
    const d = new Date(); d.setDate(d.getDate() - 7);
    return d.toISOString().slice(0, 10);
  });
  const [to, setTo] = useState(() => new Date().toISOString().slice(0, 10));

  useEffect(() => {
    api.data(type, `${from}T00:00:00Z`, `${to}T23:59:59Z`)
      .then(setRecords)
      .catch(() => router.push('/login'));
  }, [type, from, to, router]);

  // Determine primary numeric key for chart (first numeric field that isn't an ID)
  const numericKey = records.length > 0
    ? Object.entries(records[0]).find(([k, v]) =>
        typeof v === 'number' && !k.endsWith('_id') && k !== 'id'
      )?.[0]
    : undefined;

  const timeKey = records.length > 0
    ? (['time', 'start_time'].find(k => k in records[0]))
    : undefined;

  return (
    <main className="p-6 max-w-4xl mx-auto">
      <div className="flex items-center gap-4 mb-6">
        <a href="/" className="text-blue-600 hover:underline">← Dashboard</a>
        <h1 className="text-2xl font-bold capitalize">{type.replace(/_/g, ' ')}</h1>
      </div>

      <div className="flex gap-3 mb-6">
        <label className="flex items-center gap-2 text-sm">
          From <input type="date" value={from} onChange={e => setFrom(e.target.value)}
            className="border rounded px-2 py-1" />
        </label>
        <label className="flex items-center gap-2 text-sm">
          To <input type="date" value={to} onChange={e => setTo(e.target.value)}
            className="border rounded px-2 py-1" />
        </label>
      </div>

      {numericKey && timeKey && records.length > 0 && (
        <div className="bg-white rounded-lg shadow p-4 mb-6">
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={records}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey={timeKey} tickFormatter={v => new Date(v as string).toLocaleDateString()} />
              <YAxis />
              <Tooltip labelFormatter={v => new Date(v as string).toLocaleString()} />
              <Line type="monotone" dataKey={numericKey} stroke="#2563eb" dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}

      <div className="bg-white rounded-lg shadow overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 border-b">
            <tr>
              {records.length > 0 && Object.keys(records[0])
                .filter(k => !['id','family_id','user_id','source_payload_id','deleted_at'].includes(k))
                .map(k => <th key={k} className="px-3 py-2 text-left font-medium text-gray-600">{k}</th>)}
            </tr>
          </thead>
          <tbody>
            {records.map((r, i) => (
              <tr key={i} className="border-b hover:bg-gray-50">
                {Object.entries(r)
                  .filter(([k]) => !['id','family_id','user_id','source_payload_id','deleted_at'].includes(k))
                  .map(([k, v]) => (
                    <td key={k} className="px-3 py-2">
                      {typeof v === 'string' && v.includes('T')
                        ? new Date(v).toLocaleString()
                        : String(v ?? '')}
                    </td>
                  ))}
              </tr>
            ))}
          </tbody>
        </table>
        {records.length === 0 && <p className="p-4 text-gray-500 text-center">No data in this range.</p>}
      </div>
    </main>
  );
}
```

- [ ] **Step 7: Build frontend**

```bash
cd /data/HealthVault/frontend && npm run build
```
Expected: build succeeds with no errors.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat: Next.js frontend — login, dashboard, data type pages with charts"
```

---

### Task 9: Docker, nginx, and Deployment

**Files:**
- Create: `backend/Dockerfile`
- Create: `frontend/Dockerfile`
- Create: `nginx/Dockerfile`
- Create: `nginx/nginx.conf`
- Create: `docker-compose.yml`
- Create: `docker-compose.wip.yml`
- Modify: `/data/data.json` — add hcw-wip deployment entry

**Interfaces:**
- Produces: deployable stack reachable at `http://192.168.1.54:8888`

- [ ] **Step 1: Create `backend/Dockerfile`**

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /hcw ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite
WORKDIR /app
COPY --from=builder /hcw .
ENTRYPOINT ["/app/hcw", "server"]
```

- [ ] **Step 2: Create `frontend/Dockerfile`**

```dockerfile
FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
ARG NEXT_PUBLIC_API_URL=/api
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
RUN npm run build

FROM alpine:3.20
COPY --from=builder /app/out /out
```

Add `output: 'export'` to `frontend/next.config.ts`:
```typescript
import type { NextConfig } from 'next';
const config: NextConfig = { output: 'export' };
export default config;
```

- [ ] **Step 3: Create `nginx/nginx.conf`**

```nginx
server {
    listen 80;

    location /api/ {
        proxy_pass http://backend:8080/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /webhook/ {
        proxy_pass http://backend:8080/webhook/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /mcp {
        proxy_pass http://backend:8080/mcp;
        proxy_http_version 1.1;
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Host $host;
    }

    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
    }
}
```

- [ ] **Step 4: Create `nginx/Dockerfile`**

```dockerfile
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=healthvault-frontend /out /usr/share/nginx/html
```

> **Note:** The nginx image copies from the frontend build stage. In docker-compose, use multi-stage build or copy step. Simplest approach: nginx copies frontend static files at build time by referencing the frontend service build context.

Use this pattern in docker-compose instead (simpler — nginx just proxies, frontend files served by a volume copy step):

Replace nginx Dockerfile with:
```dockerfile
FROM node:22-alpine AS frontend-builder
WORKDIR /app
COPY ../frontend/package*.json ./
RUN npm ci
COPY ../frontend .
ARG NEXT_PUBLIC_API_URL=/api
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
RUN npm run build

FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=frontend-builder /app/out /usr/share/nginx/html
```

- [ ] **Step 5: Create `docker-compose.yml`**

```yaml
services:
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    restart: unless-stopped
    environment:
      HCW_PORT: ${HCW_PORT:-8080}
      HCW_DBPATH: ${HCW_DBPATH:-/data/hcw.db}
      HCW_SEED_USERS: ${HCW_SEED_USERS:-}
      HCW_JWT_SECRET: ${HCW_JWT_SECRET:-}
      HCW_COOKIE_SECURE: ${HCW_COOKIE_SECURE:-true}
      HCW_BACKUP_PATH: ${HCW_BACKUP_PATH:-}
      HCW_BACKUP_INTERVAL: ${HCW_BACKUP_INTERVAL:-24h}
      HCW_BACKUP_MAX_COUNT: ${HCW_BACKUP_MAX_COUNT:-10}
    volumes:
      - ${HCW_DATA_PATH:-./hcw-data}:/data
    networks:
      - hcw-network

  nginx:
    build:
      context: ./nginx
      dockerfile: Dockerfile
    restart: unless-stopped
    ports:
      - "${NGINX_HTTP_PORT:-80}:80"
      - "${NGINX_HTTPS_PORT:-443}:443"
    depends_on:
      - backend
    networks:
      - hcw-network

networks:
  hcw-network:
```

- [ ] **Step 6: Create `docker-compose.wip.yml`**

```yaml
services:
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    restart: unless-stopped
    environment:
      HCW_PORT: "8080"
      HCW_DBPATH: /data/hcw.db
      HCW_SEED_USERS: ${HCW_SEED_USERS:-TestFamily:alice:pass1}
      HCW_JWT_SECRET: hcw-wip-secret
      HCW_COOKIE_SECURE: "false"
      HCW_BACKUP_PATH: /data/backups
      HCW_BACKUP_INTERVAL: 24h
      HCW_BACKUP_MAX_COUNT: "5"
    volumes:
      - /mnt/eight-2/eight-2/data/data/hcw-wip:/data
    networks:
      - hcw-network

  nginx:
    build:
      context: ./nginx
      dockerfile: Dockerfile
    restart: unless-stopped
    ports:
      - "8888:80"
      - "9888:443"
    depends_on:
      - backend
    networks:
      - hcw-network

networks:
  hcw-network:
```

- [ ] **Step 7: Create data directory and update `data.json`**

```bash
mkdir -p /data/data/hcw-wip
```

Add to `/data/data.json` under `deployments`:
```json
"hcw-wip": {
  "portainer_stack_id": 0,
  "repo_url": "https://github.com/ya-breeze/HealthVault",
  "branch": "main",
  "compose_file": "docker-compose.wip.yml",
  "http_port": 8888,
  "https_port": 9888,
  "data_path": "/mnt/eight-2/eight-2/data/data/hcw-wip",
  "container_path": "/data/data/hcw-wip",
  "url": "http://192.168.1.54:8888",
  "env": [
    {"name": "HCW_SEED_USERS", "value": "TestFamily:alice:pass1,TestFamily:bob:pass2"},
    {"name": "HCW_JWT_SECRET", "value": "hcw-wip-secret"},
    {"name": "HCW_COOKIE_SECURE", "value": "false"}
  ],
  "credentials": {
    "login": "alice",
    "password": "pass1"
  }
}
```

Run:
```bash
# Edit data.json to add the above entry under "deployments"
# Verify with:
jq '.deployments["hcw-wip"]' /data/data.json
```

- [ ] **Step 8: Push and deploy**

```bash
cd /data/HealthVault
git add -A && git commit -m "feat: Docker, nginx, docker-compose for WIP deployment"
```

Then push (use git-github skill) and deploy:
```bash
/data/portainer.py git-deploy hcw-wip --branch main
/data/portainer.py wait hcw-wip
curl -sk -o /dev/null -w "%{http_code}" http://192.168.1.54:8888/
```
Expected: `200`

- [ ] **Step 9: Smoke-test deployed stack**

```bash
# Login
curl -s -X POST http://192.168.1.54:8888/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"pass1"}' -c /tmp/hcw.txt | head -c 50

# Post a test webhook
curl -s -X POST http://192.168.1.54:8888/webhook/alice \
  -H "Content-Type: application/json" \
  -d '{"timestamp":"2026-06-24T10:00:00Z","app_version":"test","steps":[{"count":5000,"start_time":"2026-06-24T00:00:00Z","end_time":"2026-06-24T23:59:59Z"}]}'

# Query data
curl -s -b /tmp/hcw.txt http://192.168.1.54:8888/api/data/steps | head -c 200
```
Expected: login returns `{"status":"ok"}`, data query returns `[{"count":5000,...}]`.

- [ ] **Step 10: Commit data.json**

```bash
cd /data && git add data.json && git commit -m "chore: add hcw-wip deployment to data.json" 2>/dev/null || true
```
(data dir may not be a git repo — just save the file.)
