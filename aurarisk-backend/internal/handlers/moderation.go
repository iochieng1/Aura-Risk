package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

const ctxModerator = "moderator"

// moderatorTokenHashes maps sha256(token) to the moderator's name.
var moderatorTokenHashes map[[sha256.Size]byte]string

// SetModerators configures who may use the moderation API, as name -> token.
// With none configured the API answers 503.
func SetModerators(tokens map[string]string) {
	moderatorTokenHashes = make(map[[sha256.Size]byte]string, len(tokens))
	for name, token := range tokens {
		moderatorTokenHashes[sha256.Sum256([]byte(token))] = name
	}
}

// RequireModerator authenticates a moderator by bearer token. Every
// configured hash is compared in constant time so timing reveals nothing.
func RequireModerator() gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(moderatorTokenHashes) == 0 {
			respondError(c, http.StatusServiceUnavailable, "moderation is not configured")
			return
		}
		token, _ := bearerToken(c)
		if token == "" {
			unauthorized(c, "missing moderator token")
			return
		}
		presented := sha256.Sum256([]byte(token))
		var name string
		for hash, candidate := range moderatorTokenHashes {
			if subtle.ConstantTimeCompare(presented[:], hash[:]) == 1 {
				name = candidate
			}
		}
		if name == "" {
			unauthorized(c, "invalid moderator token")
			return
		}
		c.Set(ctxModerator, name)
		c.Next()
	}
}

const moderationReportColumns = `
	r.id, r.location_name, r.lat, r.lon, r.category, r.note, r.created_at,
	r.moderation_status, r.corroboration_count, r.duplicate_of, r.verification_score,
	r.account_id, r.moderated_by, r.moderation_reason, r.status_updated_at,
	(SELECT COUNT(*) FROM community_reports h
		WHERE r.account_id IS NOT NULL AND h.account_id = r.account_id AND h.id <> r.id
		AND h.moderation_status = 'verified' AND h.moderated_by <> 'auto'),
	(SELECT COUNT(*) FROM community_reports h
		WHERE r.account_id IS NOT NULL AND h.account_id = r.account_id AND h.id <> r.id
		AND h.moderation_status = 'rejected' AND h.moderated_by <> 'auto')`

func scanModerationReport(row interface{ Scan(...any) error }) (models.ModerationReport, error) {
	var r models.ModerationReport
	err := row.Scan(
		&r.ID, &r.Location.Name, &r.Location.Lat, &r.Location.Lon, &r.Category, &r.Note, &r.Timestamp,
		&r.Status, &r.Corroborations, &r.DuplicateOf, &r.VerificationScore,
		&r.AccountID, &r.ModeratedBy, &r.ModerationReason, &r.StatusUpdatedAt,
		&r.ReporterVerified, &r.ReporterRejected,
	)
	r.Photos = []models.ReportPhoto{}
	return r, err
}

// ListModerationQueue returns reports for review, newest first.
//
//	status=pending|verified|rejected (default pending)
//	duplicates=include|only|exclude  (default exclude)
//	before=<RFC 3339 timestamp>      (cursor: the last item's timestamp)
//	limit=1..100                     (default 50)
func ListModerationQueue(c *gin.Context) {
	status := c.DefaultQuery("status", services.ReportPending)
	if !services.IsModerationStatus(status) {
		respondError(c, http.StatusBadRequest, "status must be pending, verified, or rejected")
		return
	}

	duplicateFilter := ""
	switch c.DefaultQuery("duplicates", "exclude") {
	case "exclude":
		duplicateFilter = "AND r.duplicate_of IS NULL"
	case "only":
		duplicateFilter = "AND r.duplicate_of IS NOT NULL"
	case "include":
	default:
		respondError(c, http.StatusBadRequest, "duplicates must be include, only, or exclude")
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		respondError(c, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}

	// created_at has no time zone and is returned as-is, so the cursor is
	// compared back in the same form.
	var before sql.NullString
	if raw := c.Query("before"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "before must be an RFC 3339 timestamp")
			return
		}
		before = sql.NullString{String: t.UTC().Format("2006-01-02 15:04:05.999999"), Valid: true}
	}

	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT `+moderationReportColumns+`
		FROM community_reports r
		WHERE r.moderation_status = $1 AND ($2::timestamp IS NULL OR r.created_at < $2::timestamp) `+duplicateFilter+`
		ORDER BY r.created_at DESC
		LIMIT $3
	`, status, before, limit)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	reports := []models.ModerationReport{}
	for rows.Next() {
		r, err := scanModerationReport(rows)
		if err != nil {
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
		if p := photos[reports[i].ID]; p != nil {
			reports[i].Photos = p
		}
	}
	c.JSON(http.StatusOK, reports)
}

type moderationDecision struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
	// NotDuplicate clears a wrong duplicate match so the report is public again.
	NotDuplicate bool `json:"not_duplicate"`
}

// ModerateReport records a moderator's decision. Decisions are final: the
// verifier never changes a report a moderator has verified or rejected.
// Setting a report back to pending hands it back to automatic verification.
func ModerateReport(c *gin.Context) {
	var req moderationDecision
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequestBody(c, err)
		return
	}
	if !services.IsModerationStatus(req.Status) {
		respondError(c, http.StatusBadRequest, "status must be pending, verified, or rejected")
		return
	}
	if req.Status == services.ReportRejected && req.Reason == "" {
		respondError(c, http.StatusBadRequest, "a reason is required to reject a report")
		return
	}
	if len(req.Reason) > 500 {
		respondError(c, http.StatusBadRequest, "reason is too long")
		return
	}

	reportID := c.Param("id")
	if !uuidPattern.MatchString(reportID) {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}
	moderator := c.GetString(ctxModerator)

	tx, err := db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var current string
	err = tx.QueryRowContext(c.Request.Context(), `SELECT moderation_status FROM community_reports WHERE id = $1 FOR UPDATE`, reportID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	moderatedBy := sql.NullString{String: moderator, Valid: req.Status != services.ReportPending}
	reason := sql.NullString{String: req.Reason, Valid: req.Reason != ""}
	if _, err := tx.ExecContext(c.Request.Context(), `
		UPDATE community_reports
		SET moderation_status = $2, moderated_by = $3, moderation_reason = $4, status_updated_at = NOW(),
			duplicate_of = CASE WHEN $5 THEN NULL ELSE duplicate_of END
		WHERE id = $1
	`, reportID, req.Status, moderatedBy, reason, req.NotDuplicate); err != nil {
		internalError(c, err)
		return
	}

	eventReason := req.Reason
	if req.NotDuplicate {
		eventReason = "marked not a duplicate. " + eventReason
	}
	if _, err := tx.ExecContext(c.Request.Context(), `
		INSERT INTO report_moderation_events (report_id, from_status, to_status, actor, reason)
		VALUES ($1, $2, $3, $4, $5)
	`, reportID, current, req.Status, moderator, sql.NullString{String: eventReason, Valid: eventReason != ""}); err != nil {
		internalError(c, err)
		return
	}

	report, err := scanModerationReport(tx.QueryRowContext(c.Request.Context(), `
		SELECT `+moderationReportColumns+` FROM community_reports r WHERE r.id = $1
	`, reportID))
	if err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// ListModerationEvents returns a report's moderation history, oldest first.
func ListModerationEvents(c *gin.Context) {
	if !uuidPattern.MatchString(c.Param("id")) {
		respondError(c, http.StatusNotFound, "report not found")
		return
	}
	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT from_status, to_status, actor, reason, created_at
		FROM report_moderation_events
		WHERE report_id = $1
		ORDER BY created_at, id
	`, c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	events := []models.ModerationEvent{}
	for rows.Next() {
		var e models.ModerationEvent
		if err := rows.Scan(&e.FromStatus, &e.ToStatus, &e.Actor, &e.Reason, &e.CreatedAt); err != nil {
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
