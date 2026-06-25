package libraImport_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/libraImport"
)

const validCSV = `#Version: 6
#Units: kg

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T08:00:00.000Z;85.0;85.0;;;;;
2026-06-02T08:00:00.000Z;84.8;84.9;;;;;
2026-06-03T08:00:00.000Z;84.6;84.8;22.5;22.5;;;
`

func TestRead_HappyPath(t *testing.T) {
	payload, counts, err := libraImport.Read(strings.NewReader(validCSV))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if counts.Weight != 3 {
		t.Errorf("weight count: want 3, got %d", counts.Weight)
	}
	if counts.BodyFat != 1 {
		t.Errorf("body_fat count: want 1, got %d", counts.BodyFat)
	}
	if payload.Weight[0].Kilograms != 85.0 {
		t.Errorf("first weight: want 85.0, got %v", payload.Weight[0].Kilograms)
	}
	if payload.BodyFat[0].Percentage != 22.5 {
		t.Errorf("body fat pct: want 22.5, got %v", payload.BodyFat[0].Percentage)
	}
}

func TestRead_Deduplication(t *testing.T) {
	csv := `#Version: 6
#Units: kg

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T08:00:00.000Z;85.0;85.0;;;;;
2026-06-01T10:00:00.000Z;85.0;85.0;;;;;
2026-06-02T08:00:00.000Z;84.8;84.9;;;;;
`
	_, counts, err := libraImport.Read(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if counts.Weight != 2 {
		t.Errorf("dedup: want 2 records (not 3), got %d", counts.Weight)
	}
}

func TestRead_BodyFatImportedWhenPresent(t *testing.T) {
	csv := `#Version: 6
#Units: kg

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T08:00:00.000Z;85.0;85.0;27.5;27.5;;;
2026-06-02T08:00:00.000Z;84.8;84.9;;;;;
`
	payload, counts, err := libraImport.Read(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if counts.BodyFat != 1 {
		t.Errorf("body_fat count: want 1, got %d", counts.BodyFat)
	}
	if len(payload.BodyFat) != 1 || payload.BodyFat[0].Percentage != 27.5 {
		t.Errorf("body fat value: want 27.5, got %+v", payload.BodyFat)
	}
}

func TestRead_BodyFatOnLaterRowSameDate(t *testing.T) {
	// First occurrence for a date has no body fat; second does.
	// Body fat should still be captured from the later row.
	csv := `#Version: 6
#Units: kg

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T06:00:00.000Z;85.0;85.0;;;;;
2026-06-01T08:00:00.000Z;85.0;85.0;22.5;22.5;;;
`
	payload, counts, err := libraImport.Read(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if counts.Weight != 1 {
		t.Errorf("weight count: want 1 (deduped), got %d", counts.Weight)
	}
	if counts.BodyFat != 1 {
		t.Errorf("body_fat count: want 1 (from later row), got %d", counts.BodyFat)
	}
	if len(payload.BodyFat) > 0 && payload.BodyFat[0].Percentage != 22.5 {
		t.Errorf("body fat pct: want 22.5, got %v", payload.BodyFat[0].Percentage)
	}
}

func TestRead_VersionMismatch(t *testing.T) {
	csv := `#Version: 5
#Units: kg

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T08:00:00.000Z;85.0;85.0;;;;;
`
	_, _, err := libraImport.Read(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for wrong version, got nil")
	}
	var ve *libraImport.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestRead_UnitsMismatch(t *testing.T) {
	csv := `#Version: 6
#Units: lbs

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T08:00:00.000Z;187.4;187.4;;;;;
`
	_, _, err := libraImport.Read(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for wrong units, got nil")
	}
	var ve *libraImport.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestRead_MissingVersionHeader(t *testing.T) {
	csv := `#Units: kg

#date;weight;weight trend;body fat;body fat trend;muscle mass;muscle mass trend;log
2026-06-01T08:00:00.000Z;85.0;85.0;;;;;
`
	_, _, err := libraImport.Read(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for missing version header, got nil")
	}
}
