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

	currentIndex := findCurrentHourIndex(weather)
	antecedent48h := sumPreviousHours(weather.Hourly.Precipitation, currentIndex, 48)
	antecedent7d := sumPreviousHours(weather.Hourly.Precipitation, currentIndex, historicalDays*24)
	antecedentRainScore := math.Min(20, antecedent48h/50*20) + math.Min(15, antecedent7d/150*15)
	soilMoistureScore := soilMoistureRiskScore(weather.Hourly.SoilMoisture, currentIndex)

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

func findCurrentHourIndex(weather *OpenMeteoResponse) int {
	if len(weather.Hourly.Precipitation) > historicalDays*24 {
		return historicalDays * 24
	}

	return len(weather.Hourly.Precipitation)
}

func sumPreviousHours(values []float64, currentIndex, hours int) float64 {
	start := currentIndex - hours
	if start < 0 {
		start = 0
	}
	if currentIndex > len(values) {
		currentIndex = len(values)
	}

	total := 0.0
	for _, value := range values[start:currentIndex] {
		total += value
	}
	return total
}

func soilMoistureRiskScore(values []float64, currentIndex int) float64 {
	if len(values) == 0 {
		return 0
	}
	if currentIndex > len(values) {
		currentIndex = len(values)
	}

	for index := currentIndex - 1; index >= 0; index-- {
		if values[index] > 0 {
			return math.Min(20, values[index]/0.40*20)
		}
	}
	return 0
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
