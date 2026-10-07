package services

import (
	"math"
	"strings"
	"time"
	"unicode"
)

// Moderation statuses for community reports.
const (
	ReportPending  = "pending"
	ReportVerified = "verified"
	ReportRejected = "rejected"
)

// ModeratedByAuto marks status changes made by the verification algorithm
// rather than a human moderator.
const ModeratedByAuto = "auto"

func IsModerationStatus(s string) bool {
	return s == ReportPending || s == ReportVerified || s == ReportRejected
}

// Duplicate detection windows. A report is a duplicate when the same account
// files a similar report close by, or when anyone posts near-identical text.
const (
	SameReporterRadiusMeters = 500
	SameReporterWindow       = 3 * time.Hour
	SimilarTextRadiusMeters  = 250
	SimilarTextWindow        = 24 * time.Hour
	SimilarTextThreshold     = 0.8
)

// Corroboration: independent reports of the same kind of problem nearby.
const (
	CorroborationRadiusMeters = 1000
	CorroborationWindow       = 6 * time.Hour
)

// AutoVerifyScore is the verification score at which a pending report is
// marked verified without a moderator.
const AutoVerifyScore = 50

// categoryGroups treats categories that describe the same event as
// corroborating each other.
var categoryGroups = map[string]string{
	"flooding":       "water",
	"water_rising":   "water",
	"road_blocked":   "road_blocked",
	"drainage_issue": "drainage_issue",
}

// CategoryGroup returns the corroboration group for a report category.
func CategoryGroup(category string) string {
	if g, ok := categoryGroups[category]; ok {
		return g
	}
	return category
}

// CategoriesForGroup lists the categories in the same group as category.
func CategoriesForGroup(category string) []string {
	group := CategoryGroup(category)
	var out []string
	for c, g := range categoryGroups {
		if g == group {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		out = []string{category}
	}
	return out
}

// DistanceMeters is the great-circle distance between two points.
func DistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6_371_000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}

// BoundingBox returns lat/lon bounds that contain every point within
// radiusMeters of the center, for an index-friendly SQL prefilter.
func BoundingBox(lat, lon, radiusMeters float64) (minLat, maxLat, minLon, maxLon float64) {
	// Slightly under the true ~111.2 km per degree, so the box errs larger.
	const metersPerDegree = 111_000.0
	latDelta := radiusMeters / metersPerDegree
	cos := math.Cos(lat * math.Pi / 180)
	lonDelta := 180.0
	if cos > 0.01 {
		lonDelta = math.Min(180, radiusMeters/(metersPerDegree*cos))
	}
	return lat - latDelta, lat + latDelta, lon - lonDelta, lon + lonDelta
}

// noteTokens lowercases the note and splits it into words, ignoring
// punctuation so "Water rising!!" and "water rising" compare equal.
func noteTokens(note string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(note), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make(map[string]bool, len(words))
	for _, w := range words {
		tokens[w] = true
	}
	return tokens
}

// NoteSimilarity is the Jaccard similarity of the notes' word sets, in [0, 1].
// Notes too short to tell apart ("flooding") score 0 unless identical, so
// independent short reports are not mistaken for copies.
func NoteSimilarity(a, b string) float64 {
	ta, tb := noteTokens(a), noteTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	shared := 0
	for w := range ta {
		if tb[w] {
			shared++
		}
	}
	union := len(ta) + len(tb) - shared
	if len(ta) < 3 || len(tb) < 3 {
		if shared == union {
			return 1
		}
		return 0
	}
	return float64(shared) / float64(union)
}

// DuplicateCandidate is an earlier, non-rejected report near a new one.
type DuplicateCandidate struct {
	ID          string
	DuplicateOf string // set when the candidate is itself a duplicate
	AccountID   string
	Category    string
	Note        string
	Lat, Lon    float64
	CreatedAt   time.Time
}

// NewReport is the report being checked for duplicates.
type NewReport struct {
	AccountID string
	Category  string
	Note      string
	Lat, Lon  float64
	CreatedAt time.Time
}

// FindDuplicate returns the original report that r duplicates, or "" if it
// is new. Candidates should be ordered oldest first; the earliest match wins
// and chains resolve to their original.
func FindDuplicate(r NewReport, candidates []DuplicateCandidate) string {
	for _, c := range candidates {
		dist := DistanceMeters(r.Lat, r.Lon, c.Lat, c.Lon)
		age := r.CreatedAt.Sub(c.CreatedAt)
		if age < 0 {
			age = -age
		}

		sameReporter := r.AccountID != "" && r.AccountID == c.AccountID &&
			CategoryGroup(r.Category) == CategoryGroup(c.Category) &&
			dist <= SameReporterRadiusMeters && age <= SameReporterWindow
		copiedText := dist <= SimilarTextRadiusMeters && age <= SimilarTextWindow &&
			NoteSimilarity(r.Note, c.Note) >= SimilarTextThreshold

		if sameReporter || copiedText {
			if c.DuplicateOf != "" {
				return c.DuplicateOf
			}
			return c.ID
		}
	}
	return ""
}

// Evidence is what the verification score is computed from.
type Evidence struct {
	// Distinct other signed-in accounts reporting the same kind of problem nearby.
	IndependentReporters int
	// Anonymous corroborating reports. They may all come from one person, so
	// they count once however many there are.
	AnonymousCorroborations int
	HasReadyPhoto           bool
	SignedIn                bool
	// The reporter's other reports, as decided by human moderators only, so
	// automatic verification can never inflate a reporter's reputation.
	ReporterVerified int
	ReporterRejected int
}

// VerificationScore rates how likely a report is genuine, from 0 to 100.
//
//	+25 per independent signed-in reporter nearby  ┐ capped at 50
//	+15 if any anonymous report corroborates it    ┘
//	+20 a photo passed malware scanning and processing
//	+10 filed by a signed-in device
//	+10 per moderator-verified past report (max +20)
//	-25 per moderator-rejected past report
func VerificationScore(e Evidence) int {
	corroboration := 25 * e.IndependentReporters
	if e.AnonymousCorroborations > 0 {
		corroboration += 15
	}
	score := min(corroboration, 50)
	if e.HasReadyPhoto {
		score += 20
	}
	if e.SignedIn {
		score += 10
	}
	score += min(10*e.ReporterVerified, 20)
	score -= 25 * e.ReporterRejected
	return max(0, min(score, 100))
}
