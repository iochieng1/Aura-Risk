package services

import (
	"fmt"
	"time"

	"aurarisk-backend/internal/models"
)

const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// modelCalibrated stays false until the score has been validated against
// observed floods (issue #22). Until then confidence never exceeds medium.
const modelCalibrated = false

// maxObservationAge matches the AuraRiskWeatherObservationsOld alert.
const maxObservationAge = 2 * time.Hour

// AssessConfidence rates how far an assessment's inputs can be trusted. It
// describes data quality and model maturity, not a probability that the
// score is right. Every reason lowers the level; Reasons lists them all.
func AssessConfidence(weather *OpenMeteoResponse, stale bool, now time.Time) *models.Confidence {
	var low []string

	if stale {
		low = append(low, "Live weather was unavailable; this uses cached data.")
	}
	if observed, ok := weather.ObservedAt(); !ok {
		low = append(low, "The weather provider did not say when its current conditions were observed.")
	} else if age := now.Sub(observed); age > maxObservationAge {
		low = append(low, fmt.Sprintf("Current weather conditions are %s old.", age.Truncate(time.Minute)))
	}

	index, found := currentHourIndex(weather)
	precip := weather.Hourly.Precipitation
	switch {
	case !found:
		low = append(low, "The current hour could not be matched in the hourly forecast, so rainfall timing is approximate.")
	default:
		if have := min(index+1, len(precip)); have < historicalDays*24 {
			low = append(low, fmt.Sprintf("Only %d of %d hours of rainfall history are available.", have, historicalDays*24))
		}
		if have := len(hoursAfter(precip, index, forecastHours)); have < forecastHours {
			low = append(low, fmt.Sprintf("Only %d of %d forecast hours are available.", have, forecastHours))
		}
	}
	if latestPositive(weather.Hourly.SoilMoisture, index+1) < 0 {
		low = append(low, "No soil moisture reading is available.")
	}

	reasons := low
	level := ConfidenceHigh
	if !modelCalibrated {
		level = ConfidenceMedium
		reasons = append(reasons, "The score is a heuristic that has not yet been validated against observed floods.")
	}
	if len(low) > 0 {
		level = ConfidenceLow
	}
	return &models.Confidence{Level: level, Reasons: reasons}
}
