package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aurarisk-backend/internal/metrics"
)

const historicalDays = 7

type OpenMeteoResponse struct {
	UTCOffsetSeconds int `json:"utc_offset_seconds"`
	Current          struct {
		Time             string  `json:"time"`
		Temperature2m    float64 `json:"temperature_2m"`
		RelativeHumidity float64 `json:"relative_humidity_2m"`
		Precipitation    float64 `json:"precipitation"`
		Rain             float64 `json:"rain"`
		WeatherCode      int     `json:"weather_code"`
	} `json:"current"`
	Hourly struct {
		Time          []string  `json:"time"`
		Precipitation []float64 `json:"precipitation"`
		Rain          []float64 `json:"rain"`
		SoilMoisture  []float64 `json:"soil_moisture_0_to_7cm"`
	} `json:"hourly"`
}

func FetchWeatherData(lat, lon float64) (_ *OpenMeteoResponse, err error) {
	defer metrics.ObserveProvider(metrics.ProviderOpenMeteo, "forecast", time.Now(), &err)

	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.6f&longitude=%.6f&past_days=%d&forecast_days=1&current=temperature_2m,relative_humidity_2m,precipitation,rain,weather_code&hourly=precipitation,rain,soil_moisture_0_to_7cm&timezone=auto",
		lat,
		lon,
		historicalDays,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	var data OpenMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode weather data: %w", err)
	}

	if observedAt, ok := data.ObservedAt(); ok {
		metrics.ObserveWeatherAge(time.Since(observedAt))
	}

	return &data, nil
}

// ObservedAt returns when the current conditions were valid. Open-Meteo
// reports local time without an offset when timezone=auto is requested.
func (r *OpenMeteoResponse) ObservedAt() (time.Time, bool) {
	local, err := time.Parse("2006-01-02T15:04", r.Current.Time)
	if err != nil {
		return time.Time{}, false
	}
	return local.Add(-time.Duration(r.UTCOffsetSeconds) * time.Second), true
}
