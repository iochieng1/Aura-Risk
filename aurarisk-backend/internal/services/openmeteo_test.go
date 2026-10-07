package services

import (
	"testing"
	"time"
)

func TestObservedAtAppliesUTCOffset(t *testing.T) {
	var r OpenMeteoResponse
	r.Current.Time = "2026-10-07T14:15"
	r.UTCOffsetSeconds = 3 * 3600 // Nairobi

	got, ok := r.ObservedAt()
	if !ok {
		t.Fatal("expected a parsed time")
	}
	want := time.Date(2026, 10, 7, 11, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ObservedAt = %v, want %v", got, want)
	}
}

func TestObservedAtMissingTime(t *testing.T) {
	var r OpenMeteoResponse
	if _, ok := r.ObservedAt(); ok {
		t.Fatal("expected no time when the response has none")
	}
}
