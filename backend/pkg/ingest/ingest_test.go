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

func TestProcess_UpsertUpdatesValue(t *testing.T) {
	db, _ := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	userID, familyID := uuid.New(), uuid.New()

	p1 := &ingest.PayloadJSON{
		Weight: []ingest.WeightJSON{{Kilograms: 75.5, Time: "2026-06-24T07:30:00Z"}},
	}
	if err := ingest.Process(db, userID, familyID, uuid.New(), p1); err != nil {
		t.Fatalf("first Process: %v", err)
	}

	p2 := &ingest.PayloadJSON{
		Weight: []ingest.WeightJSON{{Kilograms: 75.3, Time: "2026-06-24T07:30:00Z"}},
	}
	if err := ingest.Process(db, userID, familyID, uuid.New(), p2); err != nil {
		t.Fatalf("second Process: %v", err)
	}

	var weights []database.Weight
	db.Find(&weights)
	if len(weights) != 1 {
		t.Fatalf("want 1 record, got %d", len(weights))
	}
	if weights[0].Kilograms != 75.3 {
		t.Errorf("want upserted value 75.3, got %v", weights[0].Kilograms)
	}
}

func TestProcess_SleepDeduplication(t *testing.T) {
	db, _ := database.Open(slog.New(slog.NewTextHandler(os.Stderr, nil)), ":memory:")
	userID, familyID := uuid.New(), uuid.New()

	p := &ingest.PayloadJSON{
		Sleep: []ingest.SleepJSON{
			{
				SessionEndTime:  "2026-06-24T07:00:00Z",
				DurationSeconds: 27000,
				Stages: []ingest.SleepStageJSON{
					{Stage: "deep", StartTime: "2026-06-24T00:00:00Z", EndTime: "2026-06-24T02:00:00Z", DurationSeconds: 7200},
				},
			},
		},
	}
	ingest.Process(db, userID, familyID, uuid.New(), p) //nolint:errcheck
	ingest.Process(db, userID, familyID, uuid.New(), p) // resend same payload

	var sleeps []database.Sleep
	db.Find(&sleeps)
	if len(sleeps) != 1 {
		t.Fatalf("sleep deduplication failed: want 1 record, got %d", len(sleeps))
	}

	var stages []database.SleepStage
	db.Find(&stages)
	if len(stages) != 1 {
		t.Fatalf("sleep stage deduplication failed: want 1 stage, got %d", len(stages))
	}
}
