package services

import (
	"testing"
	"time"
)

func TestQuietHoursContains(t *testing.T) {
	overnight := QuietHours{Start: "22:00", End: "07:00", Timezone: "Africa/Nairobi"}
	daytime := QuietHours{Start: "13:00", End: "15:30", Timezone: "UTC"}

	// Africa/Nairobi is UTC+3.
	cases := []struct {
		name  string
		quiet QuietHours
		utc   string
		want  bool
	}{
		{"overnight before start", overnight, "2026-10-03T18:59:00Z", false},
		{"overnight at start", overnight, "2026-10-03T19:00:00Z", true},
		{"overnight after midnight", overnight, "2026-10-03T23:30:00Z", true},
		{"overnight just before end", overnight, "2026-10-04T03:59:00Z", true},
		{"overnight at end", overnight, "2026-10-04T04:00:00Z", false},
		{"daytime inside", daytime, "2026-10-03T14:00:00Z", true},
		{"daytime at end", daytime, "2026-10-03T15:30:00Z", false},
		{"daytime before", daytime, "2026-10-03T12:59:00Z", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at, _ := time.Parse(time.RFC3339, tc.utc)
			if got := tc.quiet.Contains(at); got != tc.want {
				t.Fatalf("Contains(%s) = %v, want %v", tc.utc, got, tc.want)
			}
		})
	}
}

func TestQuietHoursValidate(t *testing.T) {
	valid := QuietHours{Start: "22:00", End: "07:00", Timezone: "Europe/London"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	invalid := []QuietHours{
		{Start: "25:00", End: "07:00", Timezone: "UTC"},
		{Start: "22:00", End: "22:00", Timezone: "UTC"},
		{Start: "22:00", End: "07:00", Timezone: ""},
		{Start: "22:00", End: "07:00", Timezone: "Mars/Olympus"},
	}
	for _, q := range invalid {
		if err := q.Validate(); err == nil {
			t.Errorf("expected error for %+v", q)
		}
	}
}
