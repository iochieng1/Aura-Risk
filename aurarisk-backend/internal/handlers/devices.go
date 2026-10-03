package handlers

import (
	"net/http"
	"time"

	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

type pushRegistrationRequest struct {
	PushToken string `json:"push_token"`
	Consent   *bool  `json:"consent"`
}

// RegisterPushToken stores the device's Expo push token. The client must
// send consent: true explicitly; a token alone is never treated as consent.
func RegisterPushToken(c *gin.Context) {
	var req pushRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Consent == nil || !*req.Consent {
		c.JSON(http.StatusBadRequest, gin.H{"error": "explicit consent is required to enable push notifications"})
		return
	}
	if !services.IsExpoPushToken(req.PushToken) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "push_token must be an Expo push token"})
		return
	}

	deviceID := c.GetString(ctxDeviceID)

	tx, err := db.Begin()
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	// A reinstall can hand the same token to a new device record.
	if _, err := tx.Exec(`UPDATE devices SET push_token = NULL, push_consented_at = NULL WHERE push_token = $1 AND id <> $2`, req.PushToken, deviceID); err != nil {
		internalError(c, err)
		return
	}

	var consentedAt time.Time
	err = tx.QueryRow(`
		UPDATE devices
		SET push_token = $1,
			push_consented_at = COALESCE(push_consented_at, NOW()),
			last_seen_at = NOW()
		WHERE id = $2
		RETURNING push_consented_at
	`, req.PushToken, deviceID).Scan(&consentedAt)
	if err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"push_enabled": true, "consented_at": consentedAt})
}

// UnregisterPushToken withdraws consent and forgets the token.
func UnregisterPushToken(c *gin.Context) {
	_, err := db.Exec(`UPDATE devices SET push_token = NULL, push_consented_at = NULL WHERE id = $1`, c.GetString(ctxDeviceID))
	if err != nil {
		internalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
