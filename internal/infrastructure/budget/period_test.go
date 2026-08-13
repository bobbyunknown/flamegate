package budget

import (
	"testing"
	"time"
)

func TestPeriodStart(t *testing.T) {
	// A Wednesday: 2026-07-01 15:04:05 UTC.
	now := time.Date(2026, 7, 1, 15, 4, 5, 0, time.UTC)

	tests := []struct {
		period string
		want   time.Time
	}{
		{"daily", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		// Week starts Monday; 2026-07-01 is a Wednesday, so Monday is 2026-06-29.
		{"weekly", time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)},
		{"monthly", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		{"total", time.Time{}},
		{"unknown", time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got := PeriodStart(tt.period, now)
			if !got.Equal(tt.want) {
				t.Fatalf("PeriodStart(%q) = %v, want %v", tt.period, got, tt.want)
			}
		})
	}
}

func TestPeriodStartWeeklyOnMonday(t *testing.T) {
	// 2026-06-29 is a Monday; the weekly window should start on the same day.
	monday := time.Date(2026, 6, 29, 9, 30, 0, 0, time.UTC)
	got := PeriodStart("weekly", monday)
	want := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("PeriodStart weekly on Monday = %v, want %v", got, want)
	}
}

func TestPeriodStartWeeklyOnSunday(t *testing.T) {
	// 2026-07-05 is a Sunday; Monday of that week is 2026-06-29.
	sunday := time.Date(2026, 7, 5, 23, 59, 59, 0, time.UTC)
	got := PeriodStart("weekly", sunday)
	want := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("PeriodStart weekly on Sunday = %v, want %v", got, want)
	}
}

func TestPeriodStartNormalizesToUTC(t *testing.T) {
	// A local time that is a different calendar day in UTC must be normalized.
	loc := time.FixedZone("UTC+9", 9*3600)
	// 2026-07-02 02:00 +09:00 == 2026-07-01 17:00 UTC.
	local := time.Date(2026, 7, 2, 2, 0, 0, 0, loc)
	got := PeriodStart("daily", local)
	want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("PeriodStart daily (tz) = %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("PeriodStart returned non-UTC location: %v", got.Location())
	}
}
