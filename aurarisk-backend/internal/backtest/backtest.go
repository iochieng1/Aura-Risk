// Package backtest replays historical weather and river discharge through the
// live scoring code and measures how the score would have done on known
// flood and non-flood days (#22, #58). It runs offline, never in the API.
package backtest

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"aurarisk-backend/internal/services"
)

const dateLayout = "2006-01-02"

// Event is one row of the events file.
type Event struct {
	Region  string
	Place   string
	Lat     float64
	Lon     float64
	Date    time.Time // the local day it flooded, or a control day
	Flooded bool
	Source  string
}

var header = []string{"region", "place", "lat", "lon", "date", "flooded", "source"}

// ParseEvents reads the events CSV: region,place,lat,lon,date,flooded,source.
// Flood rows must cite a source.
func ParseEvents(r io.Reader) ([]Event, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || !slices.Equal(rows[0], header) {
		return nil, fmt.Errorf("first line must be the header %s", strings.Join(header, ","))
	}
	var events []Event
	for i, row := range rows[1:] {
		line := i + 2
		e, err := parseEvent(row)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		events = append(events, e)
	}
	return events, nil
}

func parseEvent(row []string) (Event, error) {
	e := Event{Region: strings.TrimSpace(row[0]), Place: strings.TrimSpace(row[1]), Source: strings.TrimSpace(row[6])}
	var err error
	if e.Lat, err = strconv.ParseFloat(strings.TrimSpace(row[2]), 64); err != nil || math.Abs(e.Lat) > 90 {
		return e, errors.New("invalid lat")
	}
	if e.Lon, err = strconv.ParseFloat(strings.TrimSpace(row[3]), 64); err != nil || math.Abs(e.Lon) > 180 {
		return e, errors.New("invalid lon")
	}
	if e.Date, err = time.Parse(dateLayout, strings.TrimSpace(row[4])); err != nil {
		return e, errors.New("date must be YYYY-MM-DD")
	}
	switch strings.TrimSpace(row[5]) {
	case "yes":
		e.Flooded = true
	case "no":
	default:
		return e, errors.New(`flooded must be "yes" or "no"`)
	}
	if e.Region == "" || e.Place == "" {
		return e, errors.New("region and place are required")
	}
	if e.Flooded && !strings.HasPrefix(e.Source, "http") {
		return e, errors.New("flood rows must cite a source URL")
	}
	return e, nil
}

// Source supplies historical inputs. *services.OpenMeteoClient satisfies it.
type Source interface {
	FetchArchive(ctx context.Context, lat, lon float64, start, end string) (*services.OpenMeteoResponse, error)
	FetchRiverHistory(ctx context.Context, lat, lon float64, start, end string) ([]services.RiverSeries, error)
	FetchTerrain(ctx context.Context, lat, lon float64) (*services.Terrain, error)
}

// Options control a run. Zero values use defaults.
type Options struct {
	// How many days before the event day are scored, besides the day itself.
	// An alert on any scored hour counts. Default 3.
	LeadDays int
	// The level that counts as a warning. Default "Alert".
	Threshold string
}

func (o Options) withDefaults() Options {
	if o.LeadDays <= 0 {
		o.LeadDays = 3
	}
	if o.Threshold == "" {
		o.Threshold = "Alert"
	}
	return o
}

// Outcome is the result for one event.
type Outcome struct {
	Event
	// The highest score and level over the scored hours.
	MaxScore int
	MaxLevel string
	// Warned is true when any scored hour reached the threshold.
	Warned bool
	// For a warned flood, hours from the first warning to the start of the
	// event day; negative when the first warning came during the day.
	LeadHours int
	// Whether the ground and river inputs were available.
	HadTerrain bool
	HadRiver   bool
	// Set when inputs couldn't be fetched; the event is left out of metrics.
	Err error
}

// levelRank orders levels for the threshold comparison.
var levelRank = map[string]int{"Normal": 0, "Advisory": 1, "Alert": 2, "Emergency": 3}

// Run scores every event. progress, if set, is called after each one.
func Run(ctx context.Context, src Source, events []Event, opts Options, progress func(Outcome)) []Outcome {
	opts = opts.withDefaults()
	var out []Outcome
	for _, e := range events {
		o := evaluate(ctx, src, e, opts)
		out = append(out, o)
		if progress != nil {
			progress(o)
		}
	}
	return out
}

// The scorer needs 7 days of rain history before the first scored hour, and
// 6 hours of "forecast" after the last; river baselines need 92 days.
const (
	weatherHistoryDays = 7
	riverHistoryDays   = 92
	riverPeakDays      = 3
)

func evaluate(ctx context.Context, src Source, e Event, opts Options) Outcome {
	o := Outcome{Event: e, MaxLevel: "Normal"}
	first := e.Date.AddDate(0, 0, -opts.LeadDays)

	weather, err := src.FetchArchive(ctx, e.Lat, e.Lon,
		first.AddDate(0, 0, -weatherHistoryDays).Format(dateLayout), e.Date.AddDate(0, 0, 1).Format(dateLayout))
	if err != nil {
		o.Err = fmt.Errorf("weather: %w", err)
		return o
	}
	if len(weather.Hourly.Time) == 0 {
		o.Err = errors.New("weather: no hourly data")
		return o
	}

	// Ground and river inputs are optional, as in the API.
	var h services.Hydrology
	if t, err := src.FetchTerrain(ctx, e.Lat, e.Lon); err == nil {
		h.Terrain = t
	}
	riverStart := first.AddDate(0, 0, -riverHistoryDays)
	river, err := src.FetchRiverHistory(ctx, e.Lat, e.Lon,
		riverStart.Format(dateLayout), e.Date.AddDate(0, 0, riverPeakDays).Format(dateLayout))
	if err != nil {
		river = nil
	}
	o.HadTerrain = h.Terrain != nil

	threshold := levelRank[opts.Threshold]
	firstWarning := -1
	for i, ts := range weather.Hourly.Time {
		at, err := time.Parse("2006-01-02T15:04", ts)
		if err != nil || at.Before(first) || !at.Before(e.Date.AddDate(0, 0, 1)) {
			continue
		}
		w := atHour(weather, i)
		day := int(at.Truncate(24*time.Hour).Sub(riverStart).Hours() / 24)
		h.River = services.RiverFlowOn(river, day)
		o.HadRiver = o.HadRiver || h.River != nil

		score := services.CalculateRiskScore(w, h)
		level := services.ScoreToLevel(score)
		if score > o.MaxScore {
			o.MaxScore, o.MaxLevel = score, level
		}
		if levelRank[level] >= threshold && firstWarning < 0 {
			firstWarning = i
			o.Warned = true
			o.LeadHours = int(e.Date.Sub(at).Hours())
		}
	}
	return o
}

// atHour returns the series as the live API would have seen it at hour i:
// current conditions are that hour's values. Hours after i stand in for the
// forecast, so this assumes a perfect forecast.
func atHour(series *services.OpenMeteoResponse, i int) *services.OpenMeteoResponse {
	w := *series
	w.Current.Time = series.Hourly.Time[i]
	w.Current.Precipitation = valueAt(series.Hourly.Precipitation, i)
	w.Current.Rain = valueAt(series.Hourly.Rain, i)
	if i < len(series.Hourly.WeatherCode) {
		w.Current.WeatherCode = series.Hourly.WeatherCode[i]
	}
	return &w
}

func valueAt(values []float64, i int) float64 {
	if i < len(values) {
		return values[i]
	}
	return 0
}

// Metrics summarise outcomes for one region (or "all").
type Metrics struct {
	Region      string `json:"region"`
	Floods      int    `json:"floods"`
	Detected    int    `json:"detected"`
	Controls    int    `json:"controls"`
	FalseAlarms int    `json:"false_alarms"`
	// Detected / Floods and FalseAlarms / Controls; nil without rows.
	DetectionRate  *float64 `json:"detection_rate"`
	FalseAlarmRate *float64 `json:"false_alarm_rate"`
	// Median hours from the first warning to the start of the flood day,
	// over detected floods; nil when none were detected.
	MedianLeadHours *float64 `json:"median_lead_hours"`
	// Events skipped because their inputs couldn't be fetched.
	Errors int `json:"errors"`
}

// Summarise computes metrics per region, sorted by name, then for all
// regions together.
func Summarise(outcomes []Outcome) []Metrics {
	byRegion := map[string][]Outcome{}
	for _, o := range outcomes {
		byRegion[o.Region] = append(byRegion[o.Region], o)
	}
	regions := make([]string, 0, len(byRegion))
	for r := range byRegion {
		regions = append(regions, r)
	}
	slices.Sort(regions)

	var out []Metrics
	for _, r := range regions {
		out = append(out, summarise(r, byRegion[r]))
	}
	return append(out, summarise("all", outcomes))
}

func summarise(region string, outcomes []Outcome) Metrics {
	m := Metrics{Region: region}
	var leads []float64
	for _, o := range outcomes {
		switch {
		case o.Err != nil:
			m.Errors++
		case o.Flooded:
			m.Floods++
			if o.Warned {
				m.Detected++
				leads = append(leads, float64(o.LeadHours))
			}
		default:
			m.Controls++
			if o.Warned {
				m.FalseAlarms++
			}
		}
	}
	m.DetectionRate = ratio(m.Detected, m.Floods)
	m.FalseAlarmRate = ratio(m.FalseAlarms, m.Controls)
	if len(leads) > 0 {
		slices.Sort(leads)
		n := len(leads)
		median := leads[n/2]
		if n%2 == 0 {
			median = (leads[n/2-1] + leads[n/2]) / 2
		}
		m.MedianLeadHours = &median
	}
	return m
}

func ratio(a, b int) *float64 {
	if b == 0 {
		return nil
	}
	r := float64(a) / float64(b)
	return &r
}
