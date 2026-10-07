package services

import (
	"math"
	"testing"
	"time"
)

func TestDistanceMeters(t *testing.T) {
	// 0.001 degrees of latitude is about 111 m everywhere.
	if d := DistanceMeters(-1.29, 36.82, -1.289, 36.82); math.Abs(d-111.2) > 1 {
		t.Fatalf("distance = %.1f m, want ~111 m", d)
	}
	if d := DistanceMeters(10, 20, 10, 20); d != 0 {
		t.Fatalf("distance to self = %v", d)
	}
}

func TestBoundingBoxContainsRadius(t *testing.T) {
	for _, lat := range []float64{0, 45, 80} {
		minLat, maxLat, minLon, maxLon := BoundingBox(lat, 10, 1000)
		if DistanceMeters(lat, 10, maxLat, 10) < 999 || DistanceMeters(lat, 10, minLat, 10) < 999 {
			t.Fatalf("lat %v: latitude bounds narrower than radius", lat)
		}
		if DistanceMeters(lat, 10, lat, maxLon) < 999 || DistanceMeters(lat, 10, lat, minLon) < 999 {
			t.Fatalf("lat %v: longitude bounds narrower than radius", lat)
		}
	}
}

func TestNoteSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		want func(float64) bool
	}{
		{"Water rising fast near the market bridge", "water rising fast near the market bridge!!", func(s float64) bool { return s == 1 }},
		{"Water rising fast near the market bridge", "Road blocked by a fallen tree on Ngong road", func(s float64) bool { return s < 0.2 }},
		{"Flooding", "flooding!", func(s float64) bool { return s == 1 }},
		// Short notes that merely share a word are not copies.
		{"Flooding here", "Flooding there", func(s float64) bool { return s == 0 }},
		{"", "anything", func(s float64) bool { return s == 0 }},
	}
	for _, c := range cases {
		if got := NoteSimilarity(c.a, c.b); !c.want(got) {
			t.Errorf("NoteSimilarity(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestFindDuplicate(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	base := NewReport{AccountID: "acct-1", Category: "flooding", Note: "Street is under water by the school", Lat: -1.29, Lon: 36.82, CreatedAt: now}

	tests := []struct {
		name       string
		report     NewReport
		candidates []DuplicateCandidate
		want       string
	}{
		{
			name:   "same reporter, related category, nearby, recent",
			report: base,
			candidates: []DuplicateCandidate{
				{ID: "a", AccountID: "acct-1", Category: "water_rising", Note: "different words entirely here", Lat: -1.2912, Lon: 36.82, CreatedAt: now.Add(-time.Hour)},
			},
			want: "a",
		},
		{
			name:   "same reporter outside the time window is a new report",
			report: base,
			candidates: []DuplicateCandidate{
				{ID: "a", AccountID: "acct-1", Category: "flooding", Note: "other text about it", Lat: -1.29, Lon: 36.82, CreatedAt: now.Add(-4 * time.Hour)},
			},
			want: "",
		},
		{
			name:   "same reporter, unrelated category",
			report: base,
			candidates: []DuplicateCandidate{
				{ID: "a", AccountID: "acct-1", Category: "road_blocked", Note: "tree across the road", Lat: -1.29, Lon: 36.82, CreatedAt: now.Add(-time.Hour)},
			},
			want: "",
		},
		{
			name:   "different reporter with different text corroborates, not duplicates",
			report: base,
			candidates: []DuplicateCandidate{
				{ID: "a", AccountID: "acct-2", Category: "flooding", Note: "Water entering houses on the main road", Lat: -1.29, Lon: 36.82, CreatedAt: now.Add(-time.Hour)},
			},
			want: "",
		},
		{
			name:   "anonymous copy-paste of nearby text",
			report: NewReport{Category: "flooding", Note: "street is under water by the school!", Lat: -1.29, Lon: 36.8205, CreatedAt: now},
			candidates: []DuplicateCandidate{
				{ID: "a", Category: "flooding", Note: base.Note, Lat: -1.29, Lon: 36.82, CreatedAt: now.Add(-10 * time.Hour)},
			},
			want: "a",
		},
		{
			name:   "copied text far away is not a duplicate",
			report: NewReport{Category: "flooding", Note: base.Note, Lat: -1.30, Lon: 36.82, CreatedAt: now},
			candidates: []DuplicateCandidate{
				{ID: "a", Category: "flooding", Note: base.Note, Lat: -1.29, Lon: 36.82, CreatedAt: now.Add(-time.Hour)},
			},
			want: "",
		},
		{
			name:   "a duplicate of a duplicate points at the original",
			report: base,
			candidates: []DuplicateCandidate{
				{ID: "b", DuplicateOf: "a", AccountID: "acct-1", Category: "flooding", Note: "x y z", Lat: -1.29, Lon: 36.82, CreatedAt: now.Add(-time.Hour)},
			},
			want: "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FindDuplicate(tt.report, tt.candidates); got != tt.want {
				t.Fatalf("FindDuplicate = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerificationScore(t *testing.T) {
	tests := []struct {
		name     string
		e        Evidence
		want     int
		verified bool
	}{
		{"bare anonymous report", Evidence{}, 0, false},
		{"two independent reporters", Evidence{IndependentReporters: 2}, 50, true},
		{"corroboration is capped", Evidence{IndependentReporters: 5, AnonymousCorroborations: 9}, 50, true},
		{"many anonymous reports count once", Evidence{AnonymousCorroborations: 10}, 15, false},
		{"signed in with photo and one corroboration", Evidence{IndependentReporters: 1, HasReadyPhoto: true, SignedIn: true}, 55, true},
		{"trusted reporter with photo", Evidence{SignedIn: true, HasReadyPhoto: true, ReporterVerified: 5}, 50, true},
		{"rejected history outweighs evidence", Evidence{IndependentReporters: 2, SignedIn: true, ReporterRejected: 1}, 35, false},
		{"never negative", Evidence{ReporterRejected: 10}, 0, false},
	}
	for _, tt := range tests {
		got := VerificationScore(tt.e)
		if got != tt.want || (got >= AutoVerifyScore) != tt.verified {
			t.Errorf("%s: score %d (verified %v), want %d (verified %v)", tt.name, got, got >= AutoVerifyScore, tt.want, tt.verified)
		}
	}
}

func TestCategoriesForGroup(t *testing.T) {
	got := map[string]bool{}
	for _, c := range CategoriesForGroup("flooding") {
		got[c] = true
	}
	if len(got) != 2 || !got["flooding"] || !got["water_rising"] {
		t.Fatalf("CategoriesForGroup(flooding) = %v", got)
	}
}
