package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

var db *sql.DB

func SetDatabase(database *sql.DB) {
	db = database
}

type CreateReportRequest struct {
	Location struct {
		Name string  `json:"name"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	} `json:"location"`
	Category string `json:"category"`
	Note     string `json:"note"`
	// Optional Kenyan county (any of the 47, matched loosely) and ward.
	County string `json:"county"`
	Ward   string `json:"ward"`
}

// GetReports lists up to 50 public reports, newest first, near lat/lon
// (within radius km) and/or in a Kenyan county. At least one of the two is
// required.
func GetReports(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	radiusStr := c.DefaultQuery("radius", "10")

	county := ""
	if raw := c.Query("county"); raw != "" {
		var ok bool
		if county, ok = services.NormalizeCounty(raw); !ok {
			respondError(c, http.StatusBadRequest, "unknown county")
			return
		}
	}
	if county != "" && latStr == "" && lonStr == "" {
		getReports(c, county, nil)
		return
	}

	if latStr == "" || lonStr == "" {
		respondError(c, http.StatusBadRequest, "lat and lon are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid lat")
		return
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid lon")
		return
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		respondError(c, http.StatusBadRequest, "invalid coordinates")
		return
	}

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil || !(radius > 0 && radius <= maxReportRadiusKm) {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("radius must be between 0 and %d km", maxReportRadiusKm))
		return
	}

	getReports(c, county, &circle{lat: lat, lon: lon, meters: radius * 1000})
}

// maxReportRadiusKm caps GET /api/reports searches.
const maxReportRadiusKm = 200

// circle is a search area: a center and a radius in meters.
type circle struct {
	lat, lon, meters float64
}

// getReports runs the public listing, filtered by county when set and by
// distance from the center (a true radius, via PostGIS) when area is non-nil.
func getReports(c *gin.Context, county string, area *circle) {
	// Rejected reports and duplicates are never public. Pending reports are
	// shown with their status so clients can mark them unverified.
	statuses := []string{services.ReportPending, services.ReportVerified}
	if c.Query("verified_only") == "true" {
		statuses = []string{services.ReportVerified}
	}

	// Filters are added only when used, so each query shape can use its index.
	args := []any{pq.Array(statuses)}
	filters := ""
	if area != nil {
		args = append(args, area.lon, area.lat, area.meters)
		filters += " AND ST_DWithin(geog, ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography, $4)"
	}
	if county != "" {
		args = append(args, county)
		filters += fmt.Sprintf(" AND county = $%d", len(args))
	}
	query := `
		SELECT id, location_name, lat, lon, category, note, created_at, moderation_status, corroboration_count, county, ward,
			response_status, response_updated_at, location_approximate
		FROM community_reports
		WHERE moderation_status = ANY($1)
		AND duplicate_of IS NULL` + filters + `
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	var reports []models.CommunityReport
	for rows.Next() {
		var r models.CommunityReport
		if err := rows.Scan(
			&r.ID,
			&r.Location.Name,
			&r.Location.Lat,
			&r.Location.Lon,
			&r.Category,
			&r.Note,
			&r.Timestamp,
			&r.Status,
			&r.Corroborations,
			&r.County,
			&r.Ward,
			&r.ResponseStatus,
			&r.ResponseUpdatedAt,
			&r.LocationApproximate,
		); err != nil {
			internalError(c, err)
			return
		}
		reports = append(reports, r)
	}

	if reports == nil {
		reports = []models.CommunityReport{}
	}

	reportIDs := make([]string, len(reports))
	for i, r := range reports {
		reportIDs[i] = r.ID
	}
	photos, err := readyPhotosByReport(c, reportIDs)
	if err != nil {
		internalError(c, err)
		return
	}
	for i := range reports {
		reports[i].Photos = photos[reports[i].ID]
		if reports[i].Photos == nil {
			reports[i].Photos = []models.ReportPhoto{}
		}
	}

	c.JSON(http.StatusOK, reports)
}

func CreateReport(c *gin.Context) {
	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestBody(c, err)
		return
	}

	if req.Location.Name == "" || req.Category == "" || req.Note == "" {
		respondError(c, http.StatusBadRequest, "location, category, and note are required")
		return
	}

	validCategories := map[string]bool{
		"flooding":       true,
		"road_blocked":   true,
		"water_rising":   true,
		"drainage_issue": true,
	}

	if !validCategories[req.Category] {
		respondError(c, http.StatusBadRequest, "invalid category")
		return
	}

	if req.Location.Lat < -90 || req.Location.Lat > 90 || req.Location.Lon < -180 || req.Location.Lon > 180 {
		respondError(c, http.StatusBadRequest, "invalid coordinates")
		return
	}

	if len(req.Location.Name) > 200 || len(req.Note) > 2000 {
		respondError(c, http.StatusBadRequest, "location name or note is too long")
		return
	}

	county, ward, msg := validateCountyWard(req.County, req.Ward)
	if msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	// Reports from signed-in devices are owned so photos can be attached.
	report, err := storeReport(c, newReport{
		Location:  models.Location{Name: req.Location.Name, Lat: req.Location.Lat, Lon: req.Location.Lon},
		Category:  req.Category,
		Note:      req.Note,
		AccountID: c.GetString(ctxAccountID),
		County:    county,
		Ward:      ward,
		Source:    services.SourceApp,
	})
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, report)
}

// newReport is a validated report from any channel.
type newReport struct {
	Location  models.Location
	Category  string
	Note      string
	AccountID string
	County    *string
	Ward      *string
	// services.SourceApp, SourceSMS or SourceUSSD. SMS and USSD reports sit
	// at the county headquarters, so their location is approximate.
	Source string
}

// storeReport saves a report after duplicate detection and logs it for the
// audit trail.
func storeReport(c *gin.Context, r newReport) (models.CommunityReport, error) {
	accountID := sql.NullString{String: r.AccountID, Valid: r.AccountID != ""}
	approximate := r.Source != services.SourceApp

	ctx := c.Request.Context()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return models.CommunityReport{}, err
	}
	defer tx.Rollback()

	duplicateOf, err := findDuplicate(ctx, tx, services.NewReport{
		AccountID: r.AccountID,
		Category:  r.Category,
		Note:      r.Note,
		Lat:       r.Location.Lat,
		Lon:       r.Location.Lon,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return models.CommunityReport{}, err
	}

	var id string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO community_reports (location_name, lat, lon, category, note, account_id, duplicate_of, county, ward,
			source, location_approximate)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`,
		r.Location.Name,
		r.Location.Lat,
		r.Location.Lon,
		r.Category,
		r.Note,
		accountID,
		sql.NullString{String: duplicateOf, Valid: duplicateOf != ""},
		r.County,
		r.Ward,
		r.Source,
		approximate,
	).Scan(&id, &createdAt)
	if err != nil {
		return models.CommunityReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.CommunityReport{}, err
	}
	auditReportCreated(c, id, r.AccountID, r.Category, duplicateOf, r.Source)

	report := models.CommunityReport{
		ID:                  id,
		Location:            r.Location,
		Category:            r.Category,
		Note:                r.Note,
		Timestamp:           createdAt,
		Photos:              []models.ReportPhoto{},
		Status:              services.ReportPending,
		County:              r.County,
		Ward:                r.Ward,
		ResponseStatus:      services.ResponseOpen,
		LocationApproximate: approximate,
	}
	if duplicateOf != "" {
		report.DuplicateOf = &duplicateOf
	}
	return report, nil
}

// validateCountyWard returns the official county name and trimmed ward (nil
// when not given), or an error message. A ward needs a county.
func validateCountyWard(rawCounty, rawWard string) (county, ward *string, msg string) {
	if rawCounty = strings.TrimSpace(rawCounty); rawCounty != "" {
		name, ok := services.NormalizeCounty(rawCounty)
		if !ok {
			return nil, nil, "unknown county"
		}
		county = &name
	}
	if rawWard = strings.TrimSpace(rawWard); rawWard != "" {
		if county == nil {
			return nil, nil, "ward requires a county"
		}
		if len(rawWard) > services.MaxWardBytes {
			return nil, nil, "ward is too long"
		}
		ward = &rawWard
	}
	return county, ward, ""
}

// auditReportCreated logs who submitted a report so abuse can be traced.
// The note and location name are left out because they are free text.
func auditReportCreated(c *gin.Context, reportID, accountID, category, duplicateOf, source string) {
	if accountID == "" {
		accountID = "anonymous"
	}
	if duplicateOf == "" {
		duplicateOf = "-"
	}
	slog.Info("report created", "request_id", GetRequestID(c), "report_id", reportID,
		"account", accountID, "client_ip", c.ClientIP(), "category", category, "duplicate_of", duplicateOf, "source", source)
}

// findDuplicate looks for an earlier report that the new one repeats. Ages
// are computed in SQL because created_at has no time zone.
func findDuplicate(ctx context.Context, tx *sql.Tx, r services.NewReport) (string, error) {
	radius := float64(max(services.SameReporterRadiusMeters, services.SimilarTextRadiusMeters))
	window := max(services.SameReporterWindow, services.SimilarTextWindow)

	rows, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(duplicate_of::text, ''), COALESCE(account_id::text, ''), category, note, lat, lon,
			EXTRACT(EPOCH FROM NOW() - created_at)
		FROM community_reports
		WHERE ST_DWithin(geog, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		AND created_at > NOW() - make_interval(secs => $4)
		AND moderation_status <> 'rejected'
		ORDER BY created_at
		LIMIT 200
	`, r.Lon, r.Lat, radius, window.Seconds())
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var candidates []services.DuplicateCandidate
	for rows.Next() {
		var cand services.DuplicateCandidate
		var ageSeconds float64
		if err := rows.Scan(&cand.ID, &cand.DuplicateOf, &cand.AccountID, &cand.Category, &cand.Note,
			&cand.Lat, &cand.Lon, &ageSeconds); err != nil {
			return "", err
		}
		cand.CreatedAt = r.CreatedAt.Add(-time.Duration(ageSeconds * float64(time.Second)))
		candidates = append(candidates, cand)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return services.FindDuplicate(r, candidates), nil
}
