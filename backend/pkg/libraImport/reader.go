package libraImport

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ya-breeze/healthvault/pkg/ingest"
)

// Counts holds the number of records parsed per type.
type Counts struct {
	Weight  int `json:"weight"`
	BodyFat int `json:"body_fat"`
}

// ValidationError is returned when the CSV header fails validation (version or units).
// The caller should respond with HTTP 422.
type ValidationError struct {
	msg string
}

func (e *ValidationError) Error() string { return e.msg }

// Read parses a Libra CSV export from r, deduplicates to the first record per
// UTC calendar date, and returns an ingest.PayloadJSON ready for ingest.Process.
func Read(r io.Reader) (*ingest.PayloadJSON, *Counts, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	versionOK := false
	unitsOK := false
	// Track weight and body fat deduplication independently so that a date
	// whose first row has no body fat can still pick up body fat from a later row.
	seenWeightDates := map[string]bool{}
	seenBodyFatDates := map[string]bool{}

	p := &ingest.PayloadJSON{}
	c := &Counts{}

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#Version:") {
				v := strings.TrimSpace(strings.TrimPrefix(line, "#Version:"))
				if v != "6" {
					return nil, nil, &ValidationError{msg: fmt.Sprintf("unsupported Libra version %q (want 6)", v)}
				}
				versionOK = true
			}
			if strings.HasPrefix(line, "#Units:") {
				u := strings.TrimSpace(strings.TrimPrefix(line, "#Units:"))
				if u != "kg" {
					return nil, nil, &ValidationError{msg: fmt.Sprintf("unsupported units %q (want kg)", u)}
				}
				unitsOK = true
			}
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip data rows until both required headers have been validated,
		// avoiding wasted allocations for files that are missing headers entirely.
		if !versionOK || !unitsOK {
			continue
		}

		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}

		dateStr := strings.TrimSpace(fields[0])
		weightStr := strings.TrimSpace(fields[1])

		t, err := time.Parse(time.RFC3339Nano, dateStr)
		if err != nil {
			continue
		}

		dateKey := t.UTC().Format("2006-01-02")
		ts := t.UTC().Format(time.RFC3339Nano)

		if weightStr != "" && !seenWeightDates[dateKey] {
			kg, err := strconv.ParseFloat(weightStr, 64)
			if err == nil {
				p.Weight = append(p.Weight, ingest.WeightJSON{Kilograms: kg, Time: ts})
				c.Weight++
				seenWeightDates[dateKey] = true
			}
		}

		if len(fields) > 3 && !seenBodyFatDates[dateKey] {
			bfStr := strings.TrimSpace(fields[3])
			if bfStr != "" {
				pct, err := strconv.ParseFloat(bfStr, 64)
				if err == nil {
					p.BodyFat = append(p.BodyFat, ingest.BodyFatJSON{Percentage: pct, Time: ts})
					c.BodyFat++
					seenBodyFatDates[dateKey] = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read csv: %w", err)
	}

	if !versionOK {
		return nil, nil, &ValidationError{msg: "missing #Version: header"}
	}
	if !unitsOK {
		return nil, nil, &ValidationError{msg: "missing #Units: header"}
	}

	return p, c, nil
}
