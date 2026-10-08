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
	// Stale is true when live weather was unavailable and cached data (see
	// SourceTimestamps) was used instead.
	Stale            bool              `json:"stale"`
	SourceTimestamps *SourceTimestamps `json:"source_timestamps,omitempty"`
	// How far the inputs can be trusted; see services.AssessConfidence.
	Confidence *Confidence `json:"confidence,omitempty"`
	// Scoring logic that produced Score; see services.ModelVersion.
	ModelVersion string `json:"model_version,omitempty"`
}

type Confidence struct {
	// low, medium, or high.
	Level string `json:"level"`
	// Why the level is not higher, in plain language.
	Reasons []string `json:"reasons"`
}

type SourceTimestamps struct {
	// When the weather data was retrieved from the provider.
	WeatherFetchedAt time.Time `json:"weather_fetched_at"`
	// When the provider's current conditions were valid.
	WeatherObservedAt *time.Time `json:"weather_observed_at,omitempty"`
}

type CommunityReport struct {
	ID        string        `json:"id"`
	Location  Location      `json:"location"`
	Category  string        `json:"category"`
	Note      string        `json:"note"`
	Timestamp time.Time     `json:"timestamp"`
	Photos    []ReportPhoto `json:"photos"`
	// Status is the moderation status: pending, verified, or rejected.
	Status string `json:"status"`
	// Corroborations counts independent nearby reports of the same problem.
	Corroborations int `json:"corroborations"`
	// DuplicateOf is set when this report repeats an earlier one. Duplicates
	// are hidden from public listings.
	DuplicateOf *string `json:"duplicate_of,omitempty"`
}

// ModerationReport is a report as moderators see it.
type ModerationReport struct {
	CommunityReport
	DuplicateOf       *string   `json:"duplicate_of"`
	VerificationScore int       `json:"verification_score"`
	AccountID         *string   `json:"account_id"`
	ModeratedBy       *string   `json:"moderated_by"`
	ModerationReason  *string   `json:"moderation_reason"`
	StatusUpdatedAt   time.Time `json:"status_updated_at"`
	// The reporter's other reports as decided by human moderators.
	ReporterVerified int `json:"reporter_verified"`
	ReporterRejected int `json:"reporter_rejected"`
}

type ModerationEvent struct {
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Actor      string    `json:"actor"`
	Reason     *string   `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
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
