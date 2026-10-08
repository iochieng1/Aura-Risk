package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"aurarisk-backend/internal/models"
)

// ModelVersion identifies the scoring logic in every assessment. Bump it
// whenever the score's inputs, weights, or thresholds change, so stored or
// compared assessments can be told apart.
const ModelVersion = "heuristic-1"

// forecastHours is how far ahead the score looks. openmeteo.go requests
// enough forecast days that these hours always exist.
const forecastHours = 6

func CalculateRiskScore(lat, lon float64, weather *OpenMeteoResponse) int {
	precip := weather.Hourly.Precipitation
	// Hourly precipitation is the total for the hour ending at that time, so
	// index now covers the last hour and now+1.. are the hours ahead.
	now, _ := currentHourIndex(weather)

	baseScore := weather.Current.Precipitation * 10

	forecastScore := 0.0
	for _, value := range hoursAfter(precip, now, forecastHours) {
		forecastScore += value * 5
	}

	antecedent48h := sumPreviousHours(precip, now+1, 48)
	antecedent7d := sumPreviousHours(precip, now+1, historicalDays*24)
	antecedentRainScore := math.Min(20, antecedent48h/50*20) + math.Min(15, antecedent7d/150*15)
	soilMoistureScore := soilMoistureRiskScore(weather.Hourly.SoilMoisture, now+1)

	terrainFactor := math.Abs(math.Sin(lat*100)*math.Cos(lon*100)) * 20

	weatherSeverity := 0.0
	switch {
	case weather.Current.WeatherCode >= 95:
		weatherSeverity = 30
	case weather.Current.WeatherCode >= 80:
		weatherSeverity = 15
	case weather.Current.WeatherCode >= 60:
		weatherSeverity = 10
	}

	rawScore := baseScore + forecastScore + antecedentRainScore + soilMoistureScore + terrainFactor + weatherSeverity
	score := int(math.Min(100, math.Max(0, rawScore)))

	return score
}

// currentHourIndex returns the index in the hourly series of the hour that
// contains current.time (both are local time). When it cannot be matched it
// assumes the series starts historicalDays before now and reports false.
func currentHourIndex(weather *OpenMeteoResponse) (int, bool) {
	if len(weather.Current.Time) >= 13 {
		hour := weather.Current.Time[:13] // "2006-01-02T15"
		for i, t := range weather.Hourly.Time {
			if strings.HasPrefix(t, hour) {
				return i, true
			}
		}
	}
	return min(historicalDays*24, len(weather.Hourly.Precipitation)), false
}

// hoursAfter returns up to n values following index.
func hoursAfter(values []float64, index, n int) []float64 {
	start := min(index+1, len(values))
	return values[start:min(start+n, len(values))]
}

// sumPreviousHours sums up to hours values ending just before end.
func sumPreviousHours(values []float64, end, hours int) float64 {
	start := max(end-hours, 0)
	end = min(end, len(values))

	total := 0.0
	for _, value := range values[start:end] {
		total += value
	}
	return total
}

// soilMoistureRiskScore scores the latest positive reading before end.
func soilMoistureRiskScore(values []float64, end int) float64 {
	if i := latestPositive(values, end); i >= 0 {
		return math.Min(20, values[i]/0.40*20)
	}
	return 0
}

func latestPositive(values []float64, end int) int {
	for i := min(end, len(values)) - 1; i >= 0; i-- {
		if values[i] > 0 {
			return i
		}
	}
	return -1
}

func ScoreToLevel(score int) string {
	switch {
	case score >= 75:
		return "Emergency"
	case score >= 50:
		return "Alert"
	case score >= 25:
		return "Advisory"
	default:
		return "Normal"
	}
}

func GetTips(level string) []string {
	tips := map[string][]string{
		"Normal": {
			"No immediate flood threat detected.",
			"Keep drainage areas clear of debris.",
		},
		"Advisory": {
			"Monitor local weather updates regularly.",
			"Clear gutters and storm drains near your property.",
			"Review your emergency contact list.",
		},
		"Alert": {
			"Prepare an emergency go-bag with essentials.",
			"Move vehicles to higher ground if possible.",
			"Avoid walking or driving through floodwater.",
			"Charge all devices and keep flashlights ready.",
		},
		"Emergency": {
			"Evacuate immediately if instructed by authorities.",
			"Move to the highest floor of your building.",
			"Do NOT attempt to cross flowing water.",
			"Call emergency services if trapped.",
		},
	}
	return tips[level]
}

// weatherService supplies weather to risk assessments. main replaces it with
// one built from configuration before serving traffic.
var weatherService = NewWeatherService(&OpenMeteoClient{}, WeatherConfig{})

// SetWeatherService replaces the weather source. Call it before serving.
func SetWeatherService(s *WeatherService) {
	weatherService = s
}

// GenerateRiskAssessment scores flood risk at a point. When the weather
// provider is failing it may use cached data, in which case the result is
// marked Stale. It returns ErrWeatherUnavailable when there is nothing to use.
func GenerateRiskAssessment(ctx context.Context, lat, lon float64, locationName string) (*models.RiskAssessment, error) {
	w, err := weatherService.Get(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	weather := w.Data

	score := CalculateRiskScore(lat, lon, weather)
	level := ScoreToLevel(score)
	tips := GetTips(level)

	summary := fmt.Sprintf(
		"Flood risk for %s is currently %s with a risk score of %d/100.",
		locationName, level, score,
	)

	return &models.RiskAssessment{
		Location: models.Location{
			Name: locationName,
			Lat:  lat,
			Lon:  lon,
		},
		Score:   score,
		Level:   level,
		Summary: summary,
		Tips:    tips,
		Stale:   w.Stale,
		SourceTimestamps: &models.SourceTimestamps{
			WeatherFetchedAt:  w.FetchedAt.UTC(),
			WeatherObservedAt: observedAt(weather),
		},
		Confidence:   AssessConfidence(weather, w.Stale, time.Now()),
		ModelVersion: ModelVersion,
	}, nil
}

func observedAt(weather *OpenMeteoResponse) *time.Time {
	t, ok := weather.ObservedAt()
	if !ok {
		return nil
	}
	t = t.UTC()
	return &t
}
