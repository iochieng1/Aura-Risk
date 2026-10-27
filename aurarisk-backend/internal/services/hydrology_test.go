package services

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTerrainDepressionIsSurroundingsMinusCentre(t *testing.T) {
	got := terrainFrom([]float64{1150, 1160, 1160, 1160, 1160, 1170, 1170, 1170, 1170})
	if got.Elevation != 1150 || got.Depression != 15 {
		t.Fatalf("terrain = %+v, want elevation 1150, depression 15", got)
	}
	if ridge := terrainFrom([]float64{1200, 1190, 1190, 1190, 1190, 1190, 1190, 1190, 1190}); ridge.Depression != -10 {
		t.Fatalf("ridge depression = %v, want -10", ridge.Depression)
	}
}

func TestNeighbourhoodListsPointThenEightAround(t *testing.T) {
	lats, lons := neighbourhood(-0.1, 34.8, 0.05)
	if len(lats) != 9 || len(lons) != 9 || lats[0] != "-0.1000" || lons[0] != "34.8000" {
		t.Fatalf("lats %v lons %v", lats, lons)
	}
	seen := map[string]bool{}
	for i := range lats {
		seen[lats[i]+","+lons[i]] = true
	}
	if len(seen) != 9 || !seen["-0.1500,34.7500"] || !seen["-0.0500,34.8500"] {
		t.Fatalf("points %v", seen)
	}
}

// floodSeries is one cell's past then forecast values, all equal to past
// except the forecast, which is ahead.
func floodSeries(lat, lon, past, ahead float64) RiverSeries {
	c := RiverSeries{Latitude: lat, Longitude: lon}
	for i := 0; i < riverPastDays+riverForecastDays; i++ {
		v := past
		if i >= riverPastDays {
			v = ahead
		}
		c.Daily.RiverDischarge = append(c.Daily.RiverDischarge, &v)
	}
	return c
}

func TestMainRiverPicksTheCellCarryingMostWater(t *testing.T) {
	// As near the Nyando: the nearest cell is a dry side stream.
	got := RiverFlowOn([]RiverSeries{
		floodSeries(-0.175, 34.925, 0.05, 0.2),
		floodSeries(-0.125, 34.925, 46, 120),
		floodSeries(-0.225, 34.975, 0, 0),
	}, riverPastDays)
	if got == nil || got.Lat != -0.125 || got.Baseline != 46 || got.Peak != 120 {
		t.Fatalf("river = %+v", got)
	}
}

func TestMainRiverPeakOnlyCountsTheNextDays(t *testing.T) {
	c := floodSeries(0, 0, 50, 60)
	late := 500.0
	c.Daily.RiverDischarge[riverPastDays+riverPeakDays] = &late // beyond the peak window
	if got := RiverFlowOn([]RiverSeries{c}, riverPastDays); got == nil || got.Peak != 60 {
		t.Fatalf("river = %+v, want peak 60", got)
	}
}

func TestMainRiverSkipsNullsAndShortSeries(t *testing.T) {
	short := RiverSeries{}
	v := 5.0
	short.Daily.RiverDischarge = []*float64{&v}
	allNull := RiverSeries{}
	allNull.Daily.RiverDischarge = make([]*float64, riverPastDays+riverForecastDays)
	if got := RiverFlowOn([]RiverSeries{short, allNull}, riverPastDays); got != nil {
		t.Fatalf("river = %+v, want nil", got)
	}

	gappy := floodSeries(0, 0, 30, 40)
	gappy.Daily.RiverDischarge[0] = nil
	if got := RiverFlowOn([]RiverSeries{gappy}, riverPastDays); got == nil || got.Baseline != 30 {
		t.Fatalf("river = %+v", got)
	}
}

func TestRiverFlowOnAnEarlierDay(t *testing.T) {
	// Historical series run past the evaluated day; the peak is the next 3 days.
	c := floodSeries(0, 0, 40, 40)
	for i := 0; i < 10; i++ {
		c.Daily.RiverDischarge = append(c.Daily.RiverDischarge, nil)
	}
	peak := 200.0
	c.Daily.RiverDischarge[riverPastDays+2] = &peak
	if got := RiverFlowOn([]RiverSeries{c}, riverPastDays); got == nil || got.Peak != 200 {
		t.Fatalf("river = %+v, want the peak within 3 days", got)
	}
	if got := RiverFlowOn([]RiverSeries{c}, riverPastDays-1); got != nil {
		t.Fatalf("river = %+v, want nil without %d days of history", got, riverPastDays)
	}
}

func TestMedian(t *testing.T) {
	if median([]float64{3, 1, 2}) != 2 || median([]float64{4, 1, 3, 2}) != 2.5 {
		t.Fatal("median is wrong")
	}
}

func TestLowLyingScore(t *testing.T) {
	cases := []struct {
		terrain *Terrain
		want    float64
	}{
		{nil, 0},
		{&Terrain{Depression: -20}, 0},
		{&Terrain{Depression: 7.5}, maxLowLyingScore / 2},
		{&Terrain{Depression: 100}, maxLowLyingScore},
	}
	for _, tc := range cases {
		if got := lowLyingScore(tc.terrain); got != tc.want {
			t.Errorf("lowLyingScore(%+v) = %v, want %v", tc.terrain, got, tc.want)
		}
	}
}

func TestRiverScore(t *testing.T) {
	cases := []struct {
		name  string
		river *RiverFlow
		want  float64
	}{
		{"unavailable", nil, 0},
		{"small stream", &RiverFlow{Baseline: 1, Peak: 9}, 0},
		{"normal flow", &RiverFlow{Baseline: 40, Peak: 50}, 0},
		// Seen in Nairobi at the start of the short rains: 0.95 → 16.9 m³/s.
		{"first rains on a dry river", &RiverFlow{Baseline: 0.95, Peak: 16.9}, 1.52},
		{"halfway", &RiverFlow{Baseline: 40, Peak: 110}, maxRiverScore / 2},
		{"flood peak", &RiverFlow{Baseline: 40, Peak: 400}, maxRiverScore},
	}
	for _, tc := range cases {
		if got := riverScore(tc.river); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: riverScore = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGroundAndRiverTermsAddToTheScore(t *testing.T) {
	// They replace heuristic-1's placeholder, which added up to 20 points from
	// sin/cos of the coordinates.
	dry := weatherAt(15)
	low := Hydrology{Terrain: &Terrain{Depression: 15}, River: &RiverFlow{Baseline: 40, Peak: 400}}
	if got, want := CalculateRiskScore(dry, low)-CalculateRiskScore(dry, Hydrology{}), maxLowLyingScore+maxRiverScore; got != want {
		t.Fatalf("ground and river terms added %d, want %d", got, want)
	}
}

type fakeHydrology struct {
	terrainCalls, riverCalls atomic.Int32
	terrainErr, riverErr     error
	peak                     float64
}

func (f *fakeHydrology) FetchTerrain(ctx context.Context, lat, lon float64) (*Terrain, error) {
	f.terrainCalls.Add(1)
	if f.terrainErr != nil {
		return nil, f.terrainErr
	}
	return &Terrain{Elevation: 1150, Depression: 3}, nil
}

func (f *fakeHydrology) FetchRiver(ctx context.Context, lat, lon float64) (*RiverFlow, error) {
	f.riverCalls.Add(1)
	if f.riverErr != nil {
		return nil, f.riverErr
	}
	return &RiverFlow{Baseline: 40, Peak: f.peak}, nil
}

func newTestHydrology(f *fakeHydrology, now *time.Time) *HydrologyService {
	s := NewHydrologyService(f, HydrologyConfig{})
	s.now = func() time.Time { return *now }
	return s
}

func TestHydrologyCachesTerrainForeverAndRiverForAWhile(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	f := &fakeHydrology{peak: 50}
	s := newTestHydrology(f, &now)

	s.Get(context.Background(), -0.091, 34.771)
	s.Get(context.Background(), -0.0912, 34.7712) // same cells
	if f.terrainCalls.Load() != 1 || f.riverCalls.Load() != 1 {
		t.Fatalf("calls: terrain %d river %d, want 1 each", f.terrainCalls.Load(), f.riverCalls.Load())
	}

	now = now.Add(7 * time.Hour) // past RiverFreshFor
	f.peak = 90
	h := s.Get(context.Background(), -0.091, 34.771)
	if f.terrainCalls.Load() != 1 || f.riverCalls.Load() != 2 || h.River.Peak != 90 {
		t.Fatalf("calls: terrain %d river %d, peak %v", f.terrainCalls.Load(), f.riverCalls.Load(), h.River.Peak)
	}
}

func TestHydrologyFailureServesStaleRiverThenNothing(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	f := &fakeHydrology{peak: 50}
	s := newTestHydrology(f, &now)
	s.Get(context.Background(), 0, 0)

	f.riverErr = errors.New("flood API down")
	now = now.Add(7 * time.Hour)
	if h := s.Get(context.Background(), 0, 0); h.River == nil || h.River.Peak != 50 {
		t.Fatalf("within the stale window: river = %+v, want the cached one", h.River)
	}

	now = now.Add(48 * time.Hour)
	if h := s.Get(context.Background(), 0, 0); h.River != nil || h.Terrain == nil {
		t.Fatalf("past the stale window: %+v, want no river but terrain still cached", h)
	}
}

func TestHydrologyWaitsBeforeRetryingAFailedCell(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	f := &fakeHydrology{terrainErr: errors.New("down"), riverErr: errors.New("down")}
	s := newTestHydrology(f, &now)

	for range 3 {
		if h := s.Get(context.Background(), 0, 0); h.Terrain != nil || h.River != nil {
			t.Fatalf("got %+v from a failing provider", h)
		}
	}
	if f.terrainCalls.Load() != 1 || f.riverCalls.Load() != 1 {
		t.Fatalf("calls: terrain %d river %d, want 1 each until RetryAfter", f.terrainCalls.Load(), f.riverCalls.Load())
	}
	now = now.Add(11 * time.Minute)
	s.Get(context.Background(), 0, 0)
	if f.terrainCalls.Load() != 2 {
		t.Fatalf("terrain calls = %d, want a retry after RetryAfter", f.terrainCalls.Load())
	}
}

func TestMissingHydrologyLowersConfidence(t *testing.T) {
	w := weatherAt(15)
	gaps := hydrologyGaps(Hydrology{}, true)
	c := AssessConfidence(w, false, observedNow(15), gaps...)
	if c.Level != ConfidenceLow || len(c.Reasons) != 3 || !strings.Contains(c.Reasons[0], "elevation") {
		t.Fatalf("confidence = %+v", c)
	}
	if hydrologyGaps(Hydrology{}, false) != nil {
		t.Fatal("an unconfigured source should not be reported as missing")
	}
}

func TestFetchTerrainRequest(t *testing.T) {
	var got *http.Request
	srv, _ := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		got = r
		w.Write([]byte(`{"elevation":[1150,1160,1160,1160,1160,1160,1160,1160,1160]}`))
	})
	terrain, err := (&OpenMeteoClient{ElevationURL: srv.URL, APIKey: "k"}).FetchTerrain(context.Background(), -0.09, 34.77)
	if err != nil || terrain.Depression != 10 {
		t.Fatalf("terrain %+v err %v", terrain, err)
	}
	q := got.URL.Query()
	if n := len(strings.Split(q.Get("latitude"), ",")); n != 9 || q.Get("apikey") != "k" {
		t.Fatalf("query = %v", q)
	}

	short, _ := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Write([]byte(`{"elevation":[1150]}`))
	})
	if _, err := (&OpenMeteoClient{ElevationURL: short.URL}).FetchTerrain(context.Background(), 0, 0); err == nil {
		t.Fatal("expected an error for a short answer")
	}
}

func TestFetchRiverRequestAndDecode(t *testing.T) {
	var got *http.Request
	srv, _ := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		got = r
		cells := []RiverSeries{floodSeries(-0.175, 34.925, 0.05, 0.1), floodSeries(-0.125, 34.925, 46, 71)}
		// A null, as GloFAS sends for missing days.
		body, _ := json.Marshal(cells)
		w.Write([]byte(strings.Replace(string(body), "0.05", "null", 1)))
	})
	river, err := (&OpenMeteoClient{FloodURL: srv.URL}).FetchRiver(context.Background(), -0.17, 34.92)
	if err != nil || river.Baseline != 46 || river.Peak != 71 {
		t.Fatalf("river %+v err %v", river, err)
	}
	q := got.URL.Query()
	if q.Get("daily") != "river_discharge" || q.Get("past_days") != "92" || q.Get("forecast_days") != "7" {
		t.Fatalf("query = %v", q)
	}
}

func TestHydrologyEndpointSelection(t *testing.T) {
	free, paid := &OpenMeteoClient{}, &OpenMeteoClient{APIKey: "k"}
	if free.pick(free.FloodURL, OpenMeteoFloodFreeURL, OpenMeteoFloodCustomerURL) != OpenMeteoFloodFreeURL ||
		paid.pick(paid.FloodURL, OpenMeteoFloodFreeURL, OpenMeteoFloodCustomerURL) != OpenMeteoFloodCustomerURL {
		t.Fatal("flood endpoint selection by API key is wrong")
	}
}
