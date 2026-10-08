package services

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// weatherAt builds a response shaped like Open-Meteo's for past_days=7,
// forecast_days=2: hourly series from 7 days before the current local day to
// the end of tomorrow, with current conditions at hour:15 local time.
func weatherAt(hour int) *OpenMeteoResponse {
	var r OpenMeteoResponse
	r.UTCOffsetSeconds = 3 * 3600
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC) // naive local time
	n := (historicalDays + forecastDays) * 24
	for i := 0; i < n; i++ {
		r.Hourly.Time = append(r.Hourly.Time, start.Add(time.Duration(i)*time.Hour).Format("2006-01-02T15:04"))
	}
	r.Hourly.Precipitation = make([]float64, n)
	r.Hourly.SoilMoisture = make([]float64, n)
	for i := range r.Hourly.SoilMoisture {
		r.Hourly.SoilMoisture[i] = 0.2
	}
	r.Current.Time = fmt.Sprintf("2026-10-07T%02d:15", hour)
	return &r
}

// observedNow is the UTC instant of weatherAt(hour)'s current conditions.
func observedNow(hour int) time.Time {
	return time.Date(2026, 10, 7, hour, 15, 0, 0, time.UTC).Add(-3 * time.Hour)
}

func TestCurrentHourIndexMatchesCurrentTime(t *testing.T) {
	w := weatherAt(15)
	i, ok := currentHourIndex(w)
	if !ok || w.Hourly.Time[i] != "2026-10-07T15:00" {
		t.Fatalf("index %d (%v), want the 15:00 slot", i, ok)
	}
}

func TestScoreUsesHoursAheadNotStartOfHistory(t *testing.T) {
	// Rain at the very start of the history window must not count as forecast.
	old := weatherAt(15)
	for i := 0; i < 6; i++ {
		old.Hourly.Precipitation[i] = 10
	}
	// The same rain in the next six hours must.
	ahead := weatherAt(15)
	now, _ := currentHourIndex(ahead)
	for i := now + 1; i <= now+6; i++ {
		ahead.Hourly.Precipitation[i] = 10
	}
	if a, b := CalculateRiskScore(0, 0, old), CalculateRiskScore(0, 0, ahead); b-a < 50 {
		t.Fatalf("forecast rain barely moved the score: start-of-history %d, next-six-hours %d", a, b)
	}
}

func TestScoreCountsRainEarlierToday(t *testing.T) {
	dry, wet := weatherAt(15), weatherAt(15)
	now, _ := currentHourIndex(wet)
	wet.Hourly.Precipitation[now-2] = 50 // this afternoon, after local midnight
	if CalculateRiskScore(0, 0, wet) <= CalculateRiskScore(0, 0, dry) {
		t.Fatal("rain earlier today did not raise the score")
	}
}

func TestScoreIgnoresRainAfterForecastWindow(t *testing.T) {
	dry, later := weatherAt(15), weatherAt(15)
	now, _ := currentHourIndex(later)
	later.Hourly.Precipitation[now+7] = 50
	if CalculateRiskScore(0, 0, later) != CalculateRiskScore(0, 0, dry) {
		t.Fatal("rain 7 hours ahead changed the score")
	}
}

func TestConfidenceCappedAtMediumUntilCalibrated(t *testing.T) {
	c := AssessConfidence(weatherAt(15), false, observedNow(15))
	if c.Level != ConfidenceMedium || len(c.Reasons) != 1 || !strings.Contains(c.Reasons[0], "not yet been validated") {
		t.Fatalf("got %+v", c)
	}
}

func TestConfidenceLowWhenInputsAreWeak(t *testing.T) {
	cases := map[string]struct {
		weather func() *OpenMeteoResponse
		stale   bool
		now     time.Time
		reason  string
	}{
		"stale":           {func() *OpenMeteoResponse { return weatherAt(15) }, true, observedNow(15), "cached data"},
		"old observation": {func() *OpenMeteoResponse { return weatherAt(15) }, false, observedNow(15).Add(3 * time.Hour), "3h0m0s old"},
		"no observation time": {func() *OpenMeteoResponse {
			w := weatherAt(15)
			w.Current.Time = ""
			return w
		}, false, observedNow(15), "did not say when"},
		"short history": {func() *OpenMeteoResponse {
			w := weatherAt(15)
			w.Hourly.Time = w.Hourly.Time[48:]
			w.Hourly.Precipitation = w.Hourly.Precipitation[48:]
			w.Hourly.SoilMoisture = w.Hourly.SoilMoisture[48:]
			return w
		}, false, observedNow(15), "hours of rainfall history"},
		"short forecast": {func() *OpenMeteoResponse {
			w := weatherAt(23)
			w.Hourly.Precipitation = w.Hourly.Precipitation[:8*24]
			return w
		}, false, observedNow(23), "0 of 6 forecast hours"},
		"no soil moisture": {func() *OpenMeteoResponse {
			w := weatherAt(15)
			w.Hourly.SoilMoisture = nil
			return w
		}, false, observedNow(15), "soil moisture"},
		"hour not found": {func() *OpenMeteoResponse {
			w := weatherAt(15)
			w.Hourly.Time = nil
			return w
		}, false, observedNow(15), "could not be matched"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := AssessConfidence(tc.weather(), tc.stale, tc.now)
			if c.Level != ConfidenceLow || !strings.Contains(strings.Join(c.Reasons, " | "), tc.reason) {
				t.Fatalf("got %+v, want low with %q", c, tc.reason)
			}
		})
	}
}

func TestLateEveningStillHasSixForecastHours(t *testing.T) {
	c := AssessConfidence(weatherAt(23), false, observedNow(23))
	if c.Level != ConfidenceMedium {
		t.Fatalf("23:15 local with forecast_days=%d: %+v", forecastDays, c)
	}
}
