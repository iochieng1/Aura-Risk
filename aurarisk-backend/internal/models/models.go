package models

import "time"

type Location struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type RiskAssessment struct {
	Location Location `json:"location"`
	Score    int      `json:"score"`
	Level    string   `json:"level"`
	Summary  string   `json:"summary"`
	Tips     []string `json:"tips"`
}

type CommunityReport struct {
	ID        string    `json:"id"`
	Location  Location  `json:"location"`
	Category  string    `json:"category"`
	Note      string    `json:"note"`
	Timestamp time.Time `json:"timestamp"`
}
