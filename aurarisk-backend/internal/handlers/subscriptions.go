package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

const maxSubscriptionsPerAccount = 10

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var subscribableLevels = map[string]bool{"Advisory": true, "Alert": true, "Emergency": true}

type subscriptionInput struct {
	Name                           string             `json:"name"`
	Lat                            *float64           `json:"lat"`
	Lon                            *float64           `json:"lon"`
	MinLevel                       string             `json:"min_level"`
	QuietHours                     *models.QuietHours `json:"quiet_hours"`
	AllowEmergencyDuringQuietHours bool               `json:"allow_emergency_during_quiet_hours"`
}

func (in *subscriptionInput) validate() string {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "" || len(in.Name) > 100:
		return "name must be 1-100 characters"
	case in.Lat == nil || *in.Lat < -90 || *in.Lat > 90:
		return "lat must be between -90 and 90"
	case in.Lon == nil || *in.Lon < -180 || *in.Lon > 180:
		return "lon must be between -180 and 180"
	case !subscribableLevels[in.MinLevel]:
		return "min_level must be Advisory, Alert, or Emergency"
	}
	if in.QuietHours != nil {
		q := services.QuietHours(*in.QuietHours)
		if err := q.Validate(); err != nil {
			return err.Error()
		}
	}
	return ""
}

func (in *subscriptionInput) quietColumns() (start, end, tz sql.NullString) {
	if in.QuietHours == nil {
		return
	}
	return sql.NullString{String: in.QuietHours.Start, Valid: true},
		sql.NullString{String: in.QuietHours.End, Valid: true},
		sql.NullString{String: in.QuietHours.Timezone, Valid: true}
}

const subscriptionColumns = `id, name, lat, lon, min_level, quiet_start, quiet_end, quiet_timezone,
	allow_emergency_during_quiet, last_notified_level, last_notified_at, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row rowScanner) (*models.Subscription, error) {
	var (
		s                     models.Subscription
		start, end, tz, level sql.NullString
		notifiedAt            sql.NullTime
	)
	err := row.Scan(&s.ID, &s.Name, &s.Lat, &s.Lon, &s.MinLevel, &start, &end, &tz,
		&s.AllowEmergencyDuringQuietHours, &level, &notifiedAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	if start.Valid && end.Valid && tz.Valid {
		s.QuietHours = &models.QuietHours{Start: start.String, End: end.String, Timezone: tz.String}
	}
	if level.Valid {
		s.LastNotifiedLevel = &level.String
	}
	if notifiedAt.Valid {
		s.LastNotifiedAt = &notifiedAt.Time
	}
	return &s, nil
}

func ListSubscriptions(c *gin.Context) {
	rows, err := db.Query(`SELECT `+subscriptionColumns+` FROM location_subscriptions WHERE account_id = $1 ORDER BY created_at`, c.GetString(ctxAccountID))
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	subs := []models.Subscription{}
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			internalError(c, err)
			return
		}
		subs = append(subs, *s)
	}
	if err := rows.Err(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, subs)
}

func CreateSubscription(c *gin.Context) {
	var in subscriptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if msg := in.validate(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	accountID := c.GetString(ctxAccountID)
	start, end, tz := in.quietColumns()

	// The count check and insert share one statement so concurrent requests
	// can't exceed the limit by much; the limit is a guard, not a quota.
	row := db.QueryRow(`
		INSERT INTO location_subscriptions
			(account_id, name, lat, lon, min_level, quiet_start, quiet_end, quiet_timezone, allow_emergency_during_quiet)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
		WHERE (SELECT COUNT(*) FROM location_subscriptions WHERE account_id = $1) < $10
		RETURNING `+subscriptionColumns,
		accountID, in.Name, *in.Lat, *in.Lon, in.MinLevel, start, end, tz, in.AllowEmergencyDuringQuietHours, maxSubscriptionsPerAccount)

	s, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{"error": "subscription limit reached"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, s)
}

func UpdateSubscription(c *gin.Context) {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}

	var in subscriptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if msg := in.validate(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	start, end, tz := in.quietColumns()
	// Moving the location or changing the threshold starts alerting fresh.
	row := db.QueryRow(`
		UPDATE location_subscriptions
		SET name = $3, lat = $4, lon = $5, min_level = $6,
			quiet_start = $7, quiet_end = $8, quiet_timezone = $9,
			allow_emergency_during_quiet = $10,
			last_notified_level = CASE WHEN lat = $4 AND lon = $5 AND min_level = $6 THEN last_notified_level END,
			last_notified_at = CASE WHEN lat = $4 AND lon = $5 AND min_level = $6 THEN last_notified_at END,
			updated_at = NOW()
		WHERE id = $1 AND account_id = $2
		RETURNING `+subscriptionColumns,
		id, c.GetString(ctxAccountID), in.Name, *in.Lat, *in.Lon, in.MinLevel, start, end, tz, in.AllowEmergencyDuringQuietHours)

	s, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, s)
}

func DeleteSubscription(c *gin.Context) {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}

	result, err := db.Exec(`DELETE FROM location_subscriptions WHERE id = $1 AND account_id = $2`, id, c.GetString(ctxAccountID))
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}
	c.Status(http.StatusNoContent)
}
