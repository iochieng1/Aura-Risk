package services

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Ground and river data a client fetched from Open-Meteo itself (#69), sent
// with device-fetched weather when the server's own calls are refused. Like
// client weather it is checked for plausibility, never cached, and only
// affects the caller's own result.

// ClientElevation is the Elevation API response for the point and its eight
// neighbours terrainNeighbourDegrees away, in neighbourhood order.
type ClientElevation struct {
	Elevation []float64 `json:"elevation"`
}

var ErrInvalidClientHydrology = errors.New("invalid ground or river data")

const (
	// Lowest and highest ground on Earth, with margin.
	minElevationM = -500
	maxElevationM = 9000
	// The world's largest river discharge is ~200,000 m³/s.
	maxDischargeM3s = 1_000_000
)

// ClientTerrain checks client elevation data and turns it into a Terrain.
func ClientTerrain(e *ClientElevation) (*Terrain, error) {
	if e == nil {
		return nil, nil
	}
	if len(e.Elevation) != 9 {
		return nil, fmt.Errorf("%w: elevation must have 9 values (the point, then its 8 neighbours)", ErrInvalidClientHydrology)
	}
	for i, v := range e.Elevation {
		if math.IsNaN(v) || v < minElevationM || v > maxElevationM {
			return nil, fmt.Errorf("%w: elevation[%d] is out of range", ErrInvalidClientHydrology, i)
		}
	}
	return terrainFrom(e.Elevation), nil
}

// ClientRiver checks client Flood API data for (lat, lon) at now and
// summarises it as the server would. Data whose day riverPastDays isn't
// today (UTC, give or take a day) is refused, so old floods can't be replayed.
func ClientRiver(cells []RiverSeries, lat, lon float64, now time.Time) (*RiverFlow, error) {
	if cells == nil {
		return nil, nil
	}
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidClientHydrology, fmt.Sprintf(format, args...))
	}
	if len(cells) == 0 || len(cells) > 9 {
		return nil, invalid("flood must have 1 to 9 cells")
	}
	days := riverPastDays + riverForecastDays
	for i, cell := range cells {
		if math.Abs(cell.Latitude-lat) > 2*riverNeighbourDegrees || math.Abs(cell.Longitude-lon) > 2*riverNeighbourDegrees {
			return nil, invalid("flood[%d] is for a different location", i)
		}
		t, q := cell.Daily.Time, cell.Daily.RiverDischarge
		if len(t) != days || len(q) != days {
			return nil, invalid("flood[%d] must have %d days (past_days=%d, forecast_days=%d)", i, days, riverPastDays, riverForecastDays)
		}
		today, err := time.Parse(time.DateOnly, t[riverPastDays])
		if err != nil {
			return nil, invalid("flood[%d].daily.time is malformed", i)
		}
		if d := now.UTC().Sub(today); d < -24*time.Hour || d > 48*time.Hour {
			return nil, invalid("flood[%d] is not current", i)
		}
		for j, v := range q {
			if v != nil && (math.IsNaN(*v) || *v < 0 || *v > maxDischargeM3s) {
				return nil, invalid("flood[%d].daily.river_discharge[%d] is out of range", i, j)
			}
		}
	}
	if flow := RiverFlowOn(cells, riverPastDays); flow != nil {
		return flow, nil
	}
	return nil, nil
}
