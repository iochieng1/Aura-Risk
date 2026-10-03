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
	ID        string        `json:"id"`
	Location  Location      `json:"location"`
	Category  string        `json:"category"`
	Note      string        `json:"note"`
	Timestamp time.Time     `json:"timestamp"`
	Photos    []ReportPhoto `json:"photos"`
}

type ReportPhoto struct {
	ID       string `json:"id"`
	ThumbURL string `json:"thumb_url"`
	LargeURL string `json:"large_url"`
}

type Photo struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	ThumbURL string `json:"thumb_url,omitempty"`
	LargeURL string `json:"large_url,omitempty"`
}

type QuietHours struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

type Subscription struct {
	ID                             string      `json:"id"`
	Name                           string      `json:"name"`
	Lat                            float64     `json:"lat"`
	Lon                            float64     `json:"lon"`
	MinLevel                       string      `json:"min_level"`
	QuietHours                     *QuietHours `json:"quiet_hours"`
	AllowEmergencyDuringQuietHours bool        `json:"allow_emergency_during_quiet_hours"`
	LastNotifiedLevel              *string     `json:"last_notified_level"`
	LastNotifiedAt                 *time.Time  `json:"last_notified_at"`
	CreatedAt                      time.Time   `json:"created_at"`
}
