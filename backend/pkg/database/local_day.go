package database

import (
	"fmt"
	"time"
)

// LocalDayStart returns the first instant of a calendar date. Some zones skip
// midnight during DST; ParseInLocation can normalize that midnight into the
// previous date. Preserve the requested date and find the transition instead.
func LocalDayStart(date string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, err
	}
	if LocalDate(t, loc) == date {
		return t, nil
	}
	if LocalDate(t, loc) > date {
		return time.Time{}, fmt.Errorf("local calendar date %s does not exist in %s", date, loc)
	}
	lo, hi := t.UnixNano(), t.Add(48*time.Hour).UnixNano()
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if LocalDate(time.Unix(0, mid), loc) < date {
			lo = mid
		} else {
			hi = mid
		}
	}
	start := time.Unix(0, hi).In(loc)
	if LocalDate(start, loc) != date {
		return time.Time{}, fmt.Errorf("local calendar date %s does not exist in %s", date, loc)
	}
	return start, nil
}

// LocalDayWindow uses UTC date values only for calendar arithmetic, then maps
// both date keys into actual local boundaries. It does not add 24-hour offsets.
func LocalDayWindow(from, to string, loc *time.Location) (time.Time, time.Time, error) {
	start, err := LocalDayStart(from, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	calendarEnd, err := time.Parse("2006-01-02", to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := LocalDayStart(calendarEnd.AddDate(0, 0, 1).Format("2006-01-02"), loc)
	return start.UTC(), end.UTC(), err
}
