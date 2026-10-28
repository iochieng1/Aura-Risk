package handlers

import (
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

// Public county statistics and the open data export (#63). Both only use
// verified, non-duplicate reports. See docs/OPEN_DATA.md.

const (
	defaultStatsPeriod = 90 * 24 * time.Hour
	maxStatsPeriod     = 366 * 24 * time.Hour
	// Open data coordinates are rounded to 0.01° (about 1.1 km).
	openDataCoordDecimals = 2
	maxOpenDataRows       = 50_000
	OpenDataLicense       = "CC BY 4.0"
	OpenDataLicenseURL    = "https://creativecommons.org/licenses/by/4.0/"
	OpenDataAttribution   = "AuraRisk community flood reports"
)

// period reads from and to (YYYY-MM-DD, to inclusive). The default is the 90
// days up to today (UTC); at most 366 days.
func period(c *gin.Context) (from, to time.Time, ok bool) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	to = today
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "to must be a date (YYYY-MM-DD)")
			return from, to, false
		}
		to = t
	}
	from = to.Add(-defaultStatsPeriod + 24*time.Hour)
	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "from must be a date (YYYY-MM-DD)")
			return from, to, false
		}
		from = t
	}
	if from.After(to) {
		respondError(c, http.StatusBadRequest, "from must not be after to")
		return from, to, false
	}
	if to.Sub(from) >= maxStatsPeriod {
		respondError(c, http.StatusBadRequest, "the period can be at most 366 days")
		return from, to, false
	}
	return from, to, true
}

// CountyStats is one county's row in GET /api/stats/counties.
type CountyStats struct {
	County       string `json:"county"`
	Verified     int    `json:"verified"`
	Open         int    `json:"open"`
	Acknowledged int    `json:"acknowledged"`
	Resolved     int    `json:"resolved"`
	// Medians from the report's creation, in hours; null when no report in
	// the period reached that step.
	MedianHoursToAcknowledge *float64 `json:"median_hours_to_acknowledge"`
	MedianHoursToResolve     *float64 `json:"median_hours_to_resolve"`
}

// GetCountyStats reports, per county, verified reports created in the
// period, their response status, and median response times.
func GetCountyStats(c *gin.Context) {
	from, to, ok := period(c)
	if !ok {
		return
	}
	// created_at has no time zone; it holds the database session's local
	// time, so it is converted the same way before subtracting.
	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT county, COUNT(*),
			COUNT(*) FILTER (WHERE response_status = 'open'),
			COUNT(*) FILTER (WHERE response_status = 'acknowledged'),
			COUNT(*) FILTER (WHERE response_status = 'resolved'),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM acknowledged_at - created_at::timestamptz))
				FILTER (WHERE acknowledged_at IS NOT NULL),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM resolved_at - created_at::timestamptz))
				FILTER (WHERE resolved_at IS NOT NULL)
		FROM community_reports
		WHERE moderation_status = 'verified' AND duplicate_of IS NULL AND county IS NOT NULL
		AND created_at >= $1::timestamptz::timestamp AND created_at < $2::timestamptz::timestamp
		GROUP BY county
		ORDER BY COUNT(*) DESC, county
	`, from, to.Add(24*time.Hour))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	counties := []CountyStats{}
	for rows.Next() {
		var s CountyStats
		var ack, resolve *float64
		if err := rows.Scan(&s.County, &s.Verified, &s.Open, &s.Acknowledged, &s.Resolved, &ack, &resolve); err != nil {
			internalError(c, err)
			return
		}
		s.MedianHoursToAcknowledge, s.MedianHoursToResolve = hours(ack), hours(resolve)
		counties = append(counties, s)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{
		"from":     from.Format(time.DateOnly),
		"to":       to.Format(time.DateOnly),
		"counties": counties,
	})
}

// hours converts seconds to hours, to one decimal place.
func hours(seconds *float64) *float64 {
	if seconds == nil {
		return nil
	}
	h := math.Round(*seconds/360) / 10
	return &h
}

// OpenDataReport is one row of the open data export. It has no note, photos,
// place name, report ID or reporter data, and coordinates are rounded.
type OpenDataReport struct {
	CreatedAt      time.Time  `json:"created_at"`
	Category       string     `json:"category"`
	County         *string    `json:"county"`
	Ward           *string    `json:"ward"`
	Lat            float64    `json:"lat"`
	Lon            float64    `json:"lon"`
	Corroborations int        `json:"corroborations"`
	ResponseStatus string     `json:"response_status"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
}

var openDataColumns = []string{
	"created_at", "category", "county", "ward", "lat", "lon", "corroborations",
	"response_status", "acknowledged_at", "resolved_at",
}

// GetOpenDataReports exports verified reports created in the period
// (from/to, as for the stats) and optionally one county, oldest first, as
// CSV or JSON depending on the route. Licensed CC BY 4.0.
func GetOpenDataReports(format string) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, to, ok := period(c)
		if !ok {
			return
		}
		var county *string
		if raw := c.Query("county"); raw != "" {
			name, ok := services.NormalizeCounty(raw)
			if !ok {
				respondError(c, http.StatusBadRequest, "unknown county")
				return
			}
			county = &name
		}

		rows, err := db.QueryContext(c.Request.Context(), `
			SELECT created_at::timestamptz, category, county, ward, ROUND(lat, $4), ROUND(lon, $4), corroboration_count,
				response_status, acknowledged_at, resolved_at
			FROM community_reports
			WHERE moderation_status = 'verified' AND duplicate_of IS NULL
			AND created_at >= $1::timestamptz::timestamp AND created_at < $2::timestamptz::timestamp
			AND ($3::text IS NULL OR county = $3)
			ORDER BY created_at
			LIMIT $5
		`, from, to.Add(24*time.Hour), county, openDataCoordDecimals, maxOpenDataRows)
		if err != nil {
			internalError(c, err)
			return
		}
		defer rows.Close()

		reports := []OpenDataReport{}
		for rows.Next() {
			var r OpenDataReport
			if err := rows.Scan(&r.CreatedAt, &r.Category, &r.County, &r.Ward, &r.Lat, &r.Lon, &r.Corroborations,
				&r.ResponseStatus, &r.AcknowledgedAt, &r.ResolvedAt); err != nil {
				internalError(c, err)
				return
			}
			r.CreatedAt = r.CreatedAt.UTC()
			reports = append(reports, r)
		}
		if err := rows.Err(); err != nil {
			internalError(c, err)
			return
		}

		name := fmt.Sprintf("aurarisk-reports-%s-to-%s", from.Format(time.DateOnly), to.Format(time.DateOnly))
		c.Header("Cache-Control", "public, max-age=300")
		c.Header("Link", fmt.Sprintf(`<%s>; rel="license"`, OpenDataLicenseURL))
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, name, format))
		if format == "json" {
			c.JSON(http.StatusOK, gin.H{
				"license":      OpenDataLicense,
				"license_url":  OpenDataLicenseURL,
				"attribution":  OpenDataAttribution,
				"from":         from.Format(time.DateOnly),
				"to":           to.Format(time.DateOnly),
				"generated_at": time.Now().UTC(),
				"reports":      reports,
			})
			return
		}

		c.Status(http.StatusOK)
		c.Header("Content-Type", "text/csv; charset=utf-8")
		w := csv.NewWriter(c.Writer)
		w.Write(openDataColumns)
		for _, r := range reports {
			w.Write([]string{
				r.CreatedAt.Format(time.RFC3339), r.Category, deref(r.County), csvSafe(deref(r.Ward)),
				strconv.FormatFloat(r.Lat, 'f', openDataCoordDecimals, 64),
				strconv.FormatFloat(r.Lon, 'f', openDataCoordDecimals, 64),
				strconv.Itoa(r.Corroborations), r.ResponseStatus, formatTime(r.AcknowledgedAt), formatTime(r.ResolvedAt),
			})
		}
		w.Flush()
	}
}

// csvSafe stops spreadsheets running free text as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
