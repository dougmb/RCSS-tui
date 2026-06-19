package scheduler

import (
	"testing"
	"time"
)

// TestJobCadenceAndTime checks the human-readable recurrence and time labels,
// including the weekday for weekly jobs and the unknown-time fallback.
func TestJobCadenceAndTime(t *testing.T) {
	daily := Job{Kind: Upload, Hour: 3, Min: 5}
	if got := daily.Cadence(); got != "daily" {
		t.Errorf("daily cadence = %q", got)
	}
	if got := daily.Time(); got != "03:05" {
		t.Errorf("time = %q", got)
	}

	weekly := Job{Kind: Clean, Hour: 7, Min: 30, Weekly: true, Weekday: time.Wednesday}
	if got := weekly.Cadence(); got != "weekly (Wed)" {
		t.Errorf("weekly cadence = %q", got)
	}

	if got := (Job{Hour: -1, Min: -1}).Time(); got != "??:??" {
		t.Errorf("unknown time = %q", got)
	}
}

// TestWeekdayShort checks the three-letter label for every weekday, including
// out-of-range normalization (cron's 7 → Sunday).
func TestWeekdayShort(t *testing.T) {
	want := map[time.Weekday]string{
		time.Sunday: "Sun", time.Monday: "Mon", time.Tuesday: "Tue",
		time.Wednesday: "Wed", time.Thursday: "Thu", time.Friday: "Fri",
		time.Saturday: "Sat",
	}
	for d, w := range want {
		if got := weekdayShort(d); got != w {
			t.Errorf("weekdayShort(%v) = %q, want %q", d, got, w)
		}
	}
	if got := weekdayShort(time.Weekday(7)); got != "Sun" {
		t.Errorf("weekdayShort(7) = %q, want Sun", got)
	}
}
