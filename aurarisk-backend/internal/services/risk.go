package services

import (
	"fmt"
	"math"

	"aurarisk-backend/internal/models"
)

func CalculateRiskScore(lat, lon float64, weather *OpenMeteoResponse) int {
	baseScore := weather.Current.Precipitation * 10

	forecastScore := 0.0
	hoursToCheck := 6
	if len(weather.Hourly.Precipitation) < hoursToCheck {
		hoursToCheck = len(weather.Hourly.Precipitation)
	}
	for i := 0; i < hoursToCheck; i++ {
		forecastScore += weather.Hourly.Precipitation[i] * 5
	}

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

	rawScore := baseScore + forecastScore + terrainFactor + weatherSeverity
	score := int(math.Min(100, math.Max(0, rawScore)))

	return score
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

func GenerateRiskAssessment(lat, lon float64, locationName string) (*models.RiskAssessment, error) {
	weather, err := FetchWeatherData(lat, lon)
	if err != nil {
		return nil, err
	}

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
	}, nil
}
