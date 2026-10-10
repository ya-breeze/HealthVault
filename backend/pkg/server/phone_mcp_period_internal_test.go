package server

import (
	"testing"
	"time"
)

func TestResolvePhonePeriodCalendarDST(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Prague")
	now := time.Date(2026, 3, 30, 0, 15, 0, 0, loc)
	p, zone, err := resolvePhonePeriod(phonePeriodInput{Period: "yesterday"}, `{"timezone":"Europe/Prague"}`, now)
	if err != nil || p.StartDate != "2026-03-29" || p.EndDate != "2026-03-29" || p.TimezoneFallback {
		t.Fatalf("period %+v: %v", p, err)
	}
	start, end := phoneWindow(p, zone)
	if end.Sub(start) != 23*time.Hour {
		t.Fatalf("DST duration %v", end.Sub(start))
	}
	p, _, err = resolvePhonePeriod(phonePeriodInput{Period: "last_7_days"}, `{"timezone":"Europe/Prague"}`, now)
	if err != nil || p.StartDate != "2026-03-23" || p.EndDate != "2026-03-29" {
		t.Fatalf("week %+v %v", p, err)
	}
	p, zone, err = resolvePhonePeriod(phonePeriodInput{Period: "range", StartDate: "2026-10-25", EndDate: "2026-10-25"}, `{"timezone":"Europe/Prague"}`, time.Date(2026, 10, 26, 12, 0, 0, 0, loc))
	start, end = phoneWindow(p, zone)
	if err != nil || end.Sub(start) != 25*time.Hour {
		t.Fatalf("fall DST %+v %v", p, err)
	}
	p, zone, err = resolvePhonePeriod(phonePeriodInput{Period: "range", StartDate: "2018-11-04", EndDate: "2018-11-04"}, `{"timezone":"America/Sao_Paulo"}`, now)
	if err != nil || p.StartDate != "2018-11-04" || p.EndDate != "2018-11-04" {
		t.Fatalf("midnight DST rewrote date: %+v %v", p, err)
	}
	start, end = phoneWindow(p, zone)
	if start.Format(time.RFC3339) != "2018-11-04T03:00:00Z" || end.Sub(start) != 23*time.Hour {
		t.Fatalf("midnight DST window %v %v", start, end)
	}
}

func TestPhonePeriodLocalSettingFallsBackBeforeDateResolution(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("server-local", 6*60*60)
	defer func() { time.Local = previous }()
	now := time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC)
	p, _, err := resolvePhonePeriod(phonePeriodInput{Period: "today"}, `{"timezone":"Local"}`, now)
	if err != nil || p.Today != "2026-03-29" || p.StartDate != "2026-03-29" || p.Timezone != "UTC" || !p.TimezoneFallback {
		t.Fatalf("mixed fallback/date zones %+v %v", p, err)
	}
}
