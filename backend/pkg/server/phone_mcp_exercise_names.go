package server

import (
	"strconv"
	"strings"
)

// Android Health Connect ExerciseSessionRecord constants, verified 2026-10-10.
// https://developer.android.com/reference/androidx/health/connect/client/records/ExerciseSessionRecord
// Numeric interpretation does not establish an imported record's device or origin.
var phoneHealthConnectExerciseNames = map[int]string{
	0:  "Other workout",
	2:  "Badminton",
	4:  "Baseball",
	5:  "Basketball",
	8:  "Biking",
	9:  "Biking stationary",
	10: "Boot camp",
	11: "Boxing",
	13: "Calisthenics",
	14: "Cricket",
	16: "Dancing",
	25: "Elliptical",
	26: "Exercise class",
	27: "Fencing",
	28: "Football american",
	29: "Football australian",
	31: "Frisbee disc",
	32: "Golf",
	33: "Guided breathing",
	34: "Gymnastics",
	35: "Handball",
	36: "High intensity interval training",
	37: "Hiking",
	38: "Ice hockey",
	39: "Ice skating",
	44: "Martial arts",
	46: "Paddling",
	47: "Paragliding",
	48: "Pilates",
	50: "Racquetball",
	51: "Rock climbing",
	52: "Roller hockey",
	53: "Rowing",
	54: "Rowing machine",
	55: "Rugby",
	56: "Running",
	57: "Running treadmill",
	58: "Sailing",
	59: "Scuba diving",
	60: "Skating",
	61: "Skiing",
	62: "Snowboarding",
	63: "Snowshoeing",
	64: "Soccer",
	65: "Softball",
	66: "Squash",
	68: "Stair climbing",
	69: "Stair climbing machine",
	70: "Strength training",
	71: "Stretching",
	72: "Surfing",
	73: "Swimming open water",
	74: "Swimming pool",
	75: "Table tennis",
	76: "Tennis",
	78: "Volleyball",
	79: "Walking",
	80: "Water polo",
	81: "Weightlifting",
	82: "Wheelchair",
	83: "Yoga",
}

// phoneExerciseType preserves the bounded stored value and names only known codes.
// Text remains stored evidence, not a claim that a code was resolved.
func phoneExerciseType(value string) (raw string, name *string, mapping string, truncated bool) {
	raw, truncated = phoneShortText(value)
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return raw, nil, "unknown", truncated
	}
	digits := strings.TrimPrefix(strings.TrimPrefix(trimmed, "+"), "-")
	numeric := digits != ""
	for _, r := range digits {
		if r < '0' || r > '9' {
			numeric = false
			break
		}
	}
	if numeric {
		if truncated || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") {
			return raw, nil, "unknown", truncated
		}
		code, err := strconv.Atoi(trimmed)
		if err == nil {
			if label, ok := phoneHealthConnectExerciseNames[code]; ok {
				return raw, &label, "health_connect", truncated
			}
		}
		return raw, nil, "unknown", truncated
	}
	return raw, &raw, "stored_text", truncated
}
