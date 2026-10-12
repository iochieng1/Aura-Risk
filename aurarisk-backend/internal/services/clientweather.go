package services

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Limits for weather data sent by a client. They reject data that is not a
// plausible Open-Meteo response for the requested place and time; they can't
// prove it is genuine, which is why it only affects the caller's own result.
const (
	maxClientWeatherHours    = (historicalDays + forecastDays + 1) * 24
	maxClientGridOffsetDeg   = 0.5
	maxClientObservationAge  = 6 * time.Hour
	maxHourlyPrecipitationMM = 500
)

var ErrInvalidClientWeather = errors.New("invalid weather data")

// ValidateClientWeather checks client-supplied weather for (lat, lon) at now.
func ValidateClientWeather(w *OpenMeteoResponse, lat, lon float64, now time.Time) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidClientWeather, fmt.Sprintf(format, args...))
	}
	if w == nil {
		return invalid("weather is required")
	}
	if math.Abs(w.Latitude-lat) > maxClientGridOffsetDeg || math.Abs(w.Longitude-lon) > maxClientGridOffsetDeg {
		return invalid("weather is for a different location")
	}
	if math.Abs(float64(w.UTCOffsetSeconds)) > 14*3600 {
		return invalid("utc_offset_seconds is out of range")
	}

	observed, ok := w.ObservedAt()
	if !ok {
		return invalid("current.time is missing or malformed")
	}
	if age := now.Sub(observed); age > maxClientObservationAge || age < -time.Hour {
		return invalid("current conditions are not recent")
	}
	if w.Current.WeatherCode < 0 || w.Current.WeatherCode > 99 {
		return invalid("current.weather_code is out of range")
	}

	h := w.Hourly
	n := len(h.Time)
	if n == 0 || n > maxClientWeatherHours {
		return invalid("hourly.time must have 1 to %d entries", maxClientWeatherHours)
	}
	if len(h.Precipitation) != n || len(h.Rain) != n || len(h.SoilMoisture) != n {
		return invalid("hourly series must all have the same length")
	}
	for i := range n {
		if _, err := time.Parse("2006-01-02T15:04", h.Time[i]); err != nil {
			return invalid("hourly.time[%d] is malformed", i)
		}
		if p := h.Precipitation[i]; p < 0 || p > maxHourlyPrecipitationMM {
			return invalid("hourly.precipitation[%d] is out of range", i)
		}
		if r := h.Rain[i]; r < 0 || r > maxHourlyPrecipitationMM {
			return invalid("hourly.rain[%d] is out of range", i)
		}
		if m := h.SoilMoisture[i]; m < 0 || m > 1 {
			return invalid("hourly.soil_moisture_0_to_7cm[%d] is out of range", i)
		}
	}
	return nil
}
