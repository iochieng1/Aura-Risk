package handlers

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"aurarisk-backend/config"
	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

const (
	ctxResponder       = "responder"
	ctxResponderCounty = "responder_county"
)

var (
	// responderTokenHashes maps sha256(token) to the responder's name.
	responderTokenHashes map[[sha256.Size]byte]string
	// responderCounties maps a responder's name to their county.
	responderCounties map[string]string
)

// SetResponders configures who may use the responder API, as name ->
// county and token. Each responder only sees and updates reports in their
// county. With none configured the API answers 503.
func SetResponders(responders map[string]config.Responder) error {
	hashes := make(map[[sha256.Size]byte]string, len(responders))
	counties := make(map[string]string, len(responders))
	for name, r := range responders {
		county, ok := services.NormalizeCounty(r.County)
		if !ok {
			return fmt.Errorf("RESPONDER_TOKENS: %q is not a Kenyan county", r.County)
		}
		hashes[sha256.Sum256([]byte(r.Token))] = name
		counties[name] = county
	}
	responderTokenHashes, responderCounties = hashes, counties
	return nil
}

// RequireResponder authenticates a county responder by bearer token.
func RequireResponder() gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(responderTokenHashes) == 0 {
			respondError(c, http.StatusServiceUnavailable, "county responders are not configured")
			return
		}
		token, _ := bearerToken(c)
		if token == "" {
			unauthorized(c, "missing responder token")
			return
		}
		name := matchToken(responderTokenHashes, token)
		if name == "" {
			unauthorized(c, "invalid responder token")
			return
		}
		c.Set(ctxResponder, name)
		c.Set(ctxResponderCounty, responderCounties[name])
		c.Next()
	}
}

// Reports a responder can act on: verified, not duplicates, in their county.
const responderScope = `moderation_status = 'verified' AND duplicate_of IS NULL AND county = $1`

// ListResponderReports returns the responder's county's verified reports,
// newest first.
//
//	status=open|acknowledged|resolved (default open)
//	limit=1..100                      (default 50)
func ListResponderReports(c *gin.Context) {
	status := c.DefaultQuery("status", services.ResponseOpen)
	if !services.IsResponseStatus(status) {
		respondError(c, http.StatusBadRequest, "status must be open, acknowledged, or resolved")
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		respondError(c, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}

	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT id, location_name, lat, lon, category, note, created_at, moderation_status, corroboration_count, county, ward,
			response_status, response_updated_at
		FROM community_reports
		WHERE `+responderScope+` AND response_status = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, c.GetString(ctxResponderCounty), status, limit)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	reports := []models.CommunityReport{}
	for rows.Next() {
		var r models.CommunityReport
		if err := rows.Scan(&r.ID, &r.Location.Name, &r.Location.Lat, &r.Location.Lon, &r.Category, &r.Note,
			&r.Timestamp, &r.Status, &r.Corroborations, &r.County, &r.Ward, &r.ResponseStatus, &r.ResponseUpdatedAt); err != nil {
			internalError(c, err)
			return
		}
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}

	ids := make([]string, len(reports))
	for i, r := range reports {
		ids[i] = r.ID
	}
	photos, err := readyPhotosByReport(c, ids)
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

type responseUpdate struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// UpdateReportResponse moves a report in the responder's county forward
// (open -> acknowledged -> resolved) and records who did it. Reports outside
// the county answer 404, so responders can't probe other counties.
func UpdateReportResponse(c *gin.Context) {
	var req responseUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestBody(c, err)
		return
	}
	if req.Status != services.ResponseAcknowledged && req.Status != services.ResponseResolved {
		respondError(c, http.StatusBadRequest, "status must be acknowledged or resolved")
		return
	}
	if len(req.Note) > 500 {
		respondError(c, http.StatusBadRequest, "note is too long")
		return
	}
	reportID := c.Param("id")
	if !uuidPattern.MatchString(reportID) {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}
	ctx := c.Request.Context()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var current string
	err = tx.QueryRowContext(ctx, `
		SELECT response_status FROM community_reports WHERE id = $2 AND `+responderScope+` FOR UPDATE
	`, c.GetString(ctxResponderCounty), reportID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if !services.CanChangeResponse(current, req.Status) {
		respondError(c, http.StatusConflict, fmt.Sprintf("report is %s and cannot become %s", current, req.Status))
		return
	}

	var r models.CommunityReport
	err = tx.QueryRowContext(ctx, `
		UPDATE community_reports
		SET response_status = $2, response_updated_at = NOW(),
			acknowledged_at = COALESCE(acknowledged_at, NOW()),
			resolved_at = CASE WHEN $2 = 'resolved' THEN NOW() ELSE resolved_at END
		WHERE id = $1
		RETURNING id, location_name, lat, lon, category, note, created_at, moderation_status, corroboration_count,
			county, ward, response_status, response_updated_at
	`, reportID, req.Status).Scan(&r.ID, &r.Location.Name, &r.Location.Lat, &r.Location.Lon, &r.Category, &r.Note,
		&r.Timestamp, &r.Status, &r.Corroborations, &r.County, &r.Ward, &r.ResponseStatus, &r.ResponseUpdatedAt)
	if err != nil {
		internalError(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO report_response_events (report_id, from_status, to_status, actor, note)
		VALUES ($1, $2, $3, $4, $5)
	`, reportID, current, req.Status, c.GetString(ctxResponder), sql.NullString{String: req.Note, Valid: req.Note != ""}); err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	r.Photos = []models.ReportPhoto{}
	c.JSON(http.StatusOK, r)
}

// ListResponseEvents returns a report's response history, oldest first.
func ListResponseEvents(c *gin.Context) {
	reportID := c.Param("id")
	if !uuidPattern.MatchString(reportID) {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}
	var inScope bool
	if err := db.QueryRowContext(c.Request.Context(), `
		SELECT EXISTS (SELECT 1 FROM community_reports WHERE id = $2 AND `+responderScope+`)
	`, c.GetString(ctxResponderCounty), reportID).Scan(&inScope); err != nil {
		internalError(c, err)
		return
	}
	if !inScope {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}

	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT from_status, to_status, actor, note, created_at
		FROM report_response_events
		WHERE report_id = $1
		ORDER BY created_at, id
	`, reportID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	events := []models.ResponseEvent{}
	for rows.Next() {
		var e models.ResponseEvent
		if err := rows.Scan(&e.FromStatus, &e.ToStatus, &e.Actor, &e.Note, &e.CreatedAt); err != nil {
			internalError(c, err)
			return
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, events)
}
