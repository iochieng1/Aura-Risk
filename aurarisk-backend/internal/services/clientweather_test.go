package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

// clientWeather returns a plausible Open-Meteo response for (1, 2) at now.
func clientWeather(now time.Time) *OpenMeteoResponse {
	w := &OpenMeteoResponse{Latitude: 1.05, Longitude: 1.98, UTCOffsetSeconds: 3 * 3600}
	local := now.Add(3 * time.Hour)
	w.Current.Time = local.Truncate(15 * time.Minute).Format("2006-01-02T15:04")
	w.Current.WeatherCode = 61
	start := local.Truncate(time.Hour).Add(-historicalDays * 24 * time.Hour)
	for i := range (historicalDays + forecastDays) * 24 {
		w.Hourly.Time = append(w.Hourly.Time, start.Add(time.Duration(i)*time.Hour).Format("2006-01-02T15:04"))
		w.Hourly.Precipitation = append(w.Hourly.Precipitation, 0.4)
		w.Hourly.Rain = append(w.Hourly.Rain, 0.4)
		w.Hourly.SoilMoisture = append(w.Hourly.SoilMoisture, 0.3)
	}
	return w
}

func TestValidateClientWeatherAcceptsPlausibleData(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 7, 0, 0, time.UTC)
	if err := ValidateClientWeather(clientWeather(now), 1, 2, now); err != nil {
		t.Fatal(err)
	}
}

func TestValidateClientWeatherRejects(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 7, 0, 0, time.UTC)
	cases := map[string]func(w *OpenMeteoResponse){
		"other location":     func(w *OpenMeteoResponse) { w.Latitude = 3 },
		"old observation":    func(w *OpenMeteoResponse) { w.Current.Time = "2026-10-11T00:00" },
		"future observation": func(w *OpenMeteoResponse) { w.Current.Time = "2026-10-13T00:00" },
		"bad current time":   func(w *OpenMeteoResponse) { w.Current.Time = "yesterday" },
		"weather code":       func(w *OpenMeteoResponse) { w.Current.WeatherCode = 500 },
		"no hours":           func(w *OpenMeteoResponse) { w.Hourly.Time = nil },
		"too many hours": func(w *OpenMeteoResponse) {
			for range maxClientWeatherHours {
				w.Hourly.Time = append(w.Hourly.Time, w.Hourly.Time[0])
			}
		},
		"length mismatch":   func(w *OpenMeteoResponse) { w.Hourly.Rain = w.Hourly.Rain[1:] },
		"bad hour":          func(w *OpenMeteoResponse) { w.Hourly.Time[3] = "nope" },
		"negative rain":     func(w *OpenMeteoResponse) { w.Hourly.Precipitation[5] = -1 },
		"absurd rain":       func(w *OpenMeteoResponse) { w.Hourly.Rain[5] = 9000 },
		"soil out of range": func(w *OpenMeteoResponse) { w.Hourly.SoilMoisture[5] = 4 },
	}
	for name, mutate := range cases {
		w := clientWeather(now)
		mutate(w)
		if err := ValidateClientWeather(w, 1, 2, now); !errors.Is(err, ErrInvalidClientWeather) {
			t.Errorf("%s: err = %v, want ErrInvalidClientWeather", name, err)
		}
	}
	if err := ValidateClientWeather(nil, 1, 2, now); !errors.Is(err, ErrInvalidClientWeather) {
		t.Errorf("nil: err = %v", err)
	}
}

func TestAssessClientWeatherNotesTheSource(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 7, 0, 0, time.UTC)
	risk := AssessClientWeather(context.Background(), 1, 2, "Kisumu", clientWeather(now), now)
	if risk.Stale || !risk.SourceTimestamps.WeatherFetchedAt.Equal(now) {
		t.Errorf("stale=%v fetched=%v", risk.Stale, risk.SourceTimestamps.WeatherFetchedAt)
	}
	reasons := risk.Confidence.Reasons
	if len(reasons) == 0 || reasons[len(reasons)-1] != "Weather data was fetched by your device rather than the AuraRisk server." {
		t.Errorf("reasons = %v", reasons)
	}
}
