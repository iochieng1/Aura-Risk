package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
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
}

func GetReports(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	radiusStr := c.DefaultQuery("radius", "10")

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

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid radius")
		return
	}

	latDelta := radius / 111.0
	lonDelta := radius / (111.0 * 0.9)

	// Rejected reports and duplicates are never public. Pending reports are
	// shown with their status so clients can mark them unverified.
	statuses := []string{services.ReportPending, services.ReportVerified}
	if c.Query("verified_only") == "true" {
		statuses = []string{services.ReportVerified}
	}

	query := `
		SELECT id, location_name, lat, lon, category, note, created_at, moderation_status, corroboration_count
		FROM community_reports
		WHERE lat BETWEEN $1 AND $2
		AND lon BETWEEN $3 AND $4
		AND moderation_status = ANY($5)
		AND duplicate_of IS NULL
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := db.Query(
		query,
		lat-latDelta, lat+latDelta,
		lon-lonDelta, lon+lonDelta,
		pq.Array(statuses),
	)
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

	// Reports from signed-in devices are owned so photos can be attached.
	var accountID sql.NullString
	if id := c.GetString(ctxAccountID); id != "" {
		accountID = sql.NullString{String: id, Valid: true}
	}

	tx, err := db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	duplicateOf, err := findDuplicate(tx, services.NewReport{
		AccountID: accountID.String,
		Category:  req.Category,
		Note:      req.Note,
		Lat:       req.Location.Lat,
		Lon:       req.Location.Lon,
		CreatedAt: time.Now(),
	})
	if err != nil {
		internalError(c, err)
		return
	}

	query := `
		INSERT INTO community_reports (location_name, lat, lon, category, note, account_id, duplicate_of)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	var id string
	var createdAt time.Time
	err = tx.QueryRow(
		query,
		req.Location.Name,
		req.Location.Lat,
		req.Location.Lon,
		req.Category,
		req.Note,
		accountID,
		sql.NullString{String: duplicateOf, Valid: duplicateOf != ""},
	).Scan(&id, &createdAt)
	if err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	report := models.CommunityReport{
		ID: id,
		Location: models.Location{
			Name: req.Location.Name,
			Lat:  req.Location.Lat,
			Lon:  req.Location.Lon,
		},
		Category:  req.Category,
		Note:      req.Note,
		Timestamp: createdAt,
		Photos:    []models.ReportPhoto{},
		Status:    services.ReportPending,
	}
	if duplicateOf != "" {
		report.DuplicateOf = &duplicateOf
	}

	c.JSON(http.StatusCreated, report)
}

// findDuplicate looks for an earlier report that the new one repeats. Ages
// are computed in SQL because created_at has no time zone.
func findDuplicate(tx *sql.Tx, r services.NewReport) (string, error) {
	radius := float64(max(services.SameReporterRadiusMeters, services.SimilarTextRadiusMeters))
	window := max(services.SameReporterWindow, services.SimilarTextWindow)
	minLat, maxLat, minLon, maxLon := services.BoundingBox(r.Lat, r.Lon, radius)

	rows, err := tx.Query(`
		SELECT id, COALESCE(duplicate_of::text, ''), COALESCE(account_id::text, ''), category, note, lat, lon,
			EXTRACT(EPOCH FROM NOW() - created_at)
		FROM community_reports
		WHERE lat BETWEEN $1 AND $2 AND lon BETWEEN $3 AND $4
		AND created_at > NOW() - make_interval(secs => $5)
		AND moderation_status <> 'rejected'
		ORDER BY created_at
		LIMIT 200
	`, minLat, maxLat, minLon, maxLon, window.Seconds())
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
