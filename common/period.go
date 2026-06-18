package common

import (
	"fmt"
	"time"
)

// PeriodStartUnix returns the unix timestamp of the period start (local timezone).
// kind: day, week, month; other values return 0.
func PeriodStartUnix(t time.Time, kind string) int64 {
	switch kind {
	case "day":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
	case "week":
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := t.AddDate(0, 0, -(weekday - 1))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, t.Location()).Unix()
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Unix()
	default:
		return 0
	}
}

// PeriodKey returns a stable period identifier for counter storage (local timezone).
// kind: day -> 2006-01-02, week -> 2006-W02, month -> 2006-01; other values return "".
func PeriodKey(t time.Time, kind string) string {
	switch kind {
	case "day":
		return t.Format("2006-01-02")
	case "week":
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case "month":
		return t.Format("2006-01")
	default:
		return ""
	}
}
