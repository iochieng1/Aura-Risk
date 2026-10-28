package services

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestClientTerrain(t *testing.T) {
	got, err := ClientTerrain(&ClientElevation{Elevation: []float64{1140, 1150, 1150, 1150, 1150, 1150, 1150, 1150, 1150}})
	if err != nil || got.Elevation != 1140 || got.Depression != 10 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got, err := ClientTerrain(nil); got != nil || err != nil {
		t.Fatalf("nil: %v %v", got, err)
	}
	for name, e := range map[string][]float64{
		"too few":  {1, 2, 3},
		"too high": {1, 2, 3, 4, 5, 6, 7, 8, 9001},
		"too low":  {-501, 2, 3, 4, 5, 6, 7, 8, 9},
	} {
		if _, err := ClientTerrain(&ClientElevation{Elevation: e}); !errors.Is(err, ErrInvalidClientHydrology) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// floodCells returns n Flood API cells around (lat, lon) whose day
// riverPastDays is today, with steady flow then a peak.
func floodCells(n int, lat, lon float64, today time.Time, baseline, peak float64) []RiverSeries {
	days := riverPastDays + riverForecastDays
	cells := make([]RiverSeries, n)
	for i := range cells {
		cells[i].Latitude, cells[i].Longitude = lat+float64(i)*0.01, lon
		for d := range days {
			cells[i].Daily.Time = append(cells[i].Daily.Time, today.AddDate(0, 0, d-riverPastDays).Format(time.DateOnly))
			v := baseline / float64(i+1)
			if d == riverPastDays+1 {
				v = peak / float64(i+1)
			}
			cells[i].Daily.RiverDischarge = append(cells[i].Daily.RiverDischarge, &v)
		}
	}
	return cells
}

func TestClientRiver(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	today := now.Truncate(24 * time.Hour)
	got, err := ClientRiver(floodCells(9, -0.17, 34.92, today, 40, 200), -0.17, 34.92, now)
	if err != nil || got.Baseline != 40 || got.Peak != 200 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got, err := ClientRiver(nil, -0.17, 34.92, now); got != nil || err != nil {
		t.Fatalf("nil: %v %v", got, err)
	}

	bad := map[string][]RiverSeries{
		"elsewhere": floodCells(1, 1.5, 34.92, today, 40, 200),
		"old":       floodCells(1, -0.17, 34.92, today.AddDate(0, 0, -5), 40, 200),
		"future":    floodCells(1, -0.17, 34.92, today.AddDate(0, 0, 3), 40, 200),
		"too many":  floodCells(10, -0.17, 34.92, today, 40, 200),
		"none":      {},
		"short": func() []RiverSeries {
			c := floodCells(1, -0.17, 34.92, today, 40, 200)
			c[0].Daily.Time = c[0].Daily.Time[:50]
			return c
		}(),
		"negative": floodCells(1, -0.17, 34.92, today, -1, 200),
		"bad date": func() []RiverSeries {
			c := floodCells(1, -0.17, 34.92, today, 40, 200)
			c[0].Daily.Time[riverPastDays] = "x"
			return c
		}(),
		"implausible": floodCells(1, -0.17, 34.92, today, 40, 2e6),
	}
	for name, cells := range bad {
		if _, err := ClientRiver(cells, -0.17, 34.92, now); !errors.Is(err, ErrInvalidClientHydrology) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAssessClientWeatherUsesClientHydrologyOnlyForGaps(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 7, 0, 0, time.UTC)
	client := Hydrology{
		Terrain: &Terrain{Elevation: 1140, Depression: 10},
		River:   &RiverFlow{Baseline: 40, Peak: 200},
	}

	// No server data: the client's fills both gaps, and confidence says so.
	risk := AssessClientWeather(context.Background(), 1, 2, "Kisumu", clientWeather(now), client, now)
	if risk.Hydrology == nil || *risk.Hydrology.ElevationM != 1140 || *risk.Hydrology.RiverPeakM3s != 200 {
		t.Fatalf("hydrology = %+v", risk.Hydrology)
	}
	for _, want := range []string{"Ground elevation was fetched by your device", "River discharge was fetched by your device"} {
		if !slices.ContainsFunc(risk.Confidence.Reasons, func(r string) bool { return len(r) >= len(want) && r[:len(want)] == want }) {
			t.Errorf("reasons %v lack %q", risk.Confidence.Reasons, want)
		}
	}

	// Server data wins where it has some.
	SetHydrologyService(NewHydrologyService(fakeTerrainOnly{}, HydrologyConfig{}))
	t.Cleanup(func() { SetHydrologyService(nil) })
	risk = AssessClientWeather(context.Background(), 1, 2, "Kisumu", clientWeather(now), client, now)
	if *risk.Hydrology.ElevationM != 5 || *risk.Hydrology.RiverPeakM3s != 200 {
		t.Fatalf("server terrain should win, client river fill in: %+v", risk.Hydrology)
	}
}

// fakeTerrainOnly has terrain but no river data.
type fakeTerrainOnly struct{}

func (fakeTerrainOnly) FetchTerrain(context.Context, float64, float64) (*Terrain, error) {
	return &Terrain{Elevation: 5}, nil
}

func (fakeTerrainOnly) FetchRiver(context.Context, float64, float64) (*RiverFlow, error) {
	return nil, errNoRiverData
}
