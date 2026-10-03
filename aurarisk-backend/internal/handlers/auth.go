package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour

	ctxAccountID = "account_id"
	ctxDeviceID  = "device_id"
)

type TokenPair struct {
	AccountID        string    `json:"account_id"`
	DeviceID         string    `json:"device_id"`
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type registerDeviceRequest struct {
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

var validPlatforms = map[string]bool{"ios": true, "android": true, "web": true}

func issueTokens(tx *sql.Tx, accountID, deviceID string) (*TokenPair, error) {
	access, err := services.NewToken()
	if err != nil {
		return nil, err
	}
	refresh, err := services.NewToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	pair := &TokenPair{
		AccountID:        accountID,
		DeviceID:         deviceID,
		AccessToken:      access,
		AccessExpiresAt:  now.Add(AccessTokenTTL),
		RefreshToken:     refresh,
		RefreshExpiresAt: now.Add(RefreshTokenTTL),
	}

	_, err = tx.Exec(`
		INSERT INTO auth_tokens (token_hash, device_id, kind, expires_at)
		VALUES ($1, $2, 'access', $3), ($4, $2, 'refresh', $5)
	`, services.HashToken(access), deviceID, pair.AccessExpiresAt, services.HashToken(refresh), pair.RefreshExpiresAt)
	if err != nil {
		return nil, err
	}
	return pair, nil
}

func RegisterDevice(c *gin.Context) {
	var req registerDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if !validPlatforms[req.Platform] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "platform must be ios, android, or web"})
		return
	}
	if len(req.AppVersion) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "app_version is too long"})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var accountID, deviceID string
	if err := tx.QueryRow(`INSERT INTO accounts DEFAULT VALUES RETURNING id`).Scan(&accountID); err != nil {
		internalError(c, err)
		return
	}
	if err := tx.QueryRow(`
		INSERT INTO devices (account_id, platform, app_version)
		VALUES ($1, $2, NULLIF($3, ''))
		RETURNING id
	`, accountID, req.Platform, req.AppVersion).Scan(&deviceID); err != nil {
		internalError(c, err)
		return
	}

	pair, err := issueTokens(tx, accountID, deviceID)
	if err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, pair)
}

// RefreshTokens rotates a refresh token. A refresh token that was already
// rotated indicates theft or a replay, so every token for that device is
// revoked and the client must register again.
func RefreshTokens(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var (
		deviceID, accountID, kind string
		expiresAt                 time.Time
		revokedAt                 sql.NullTime
	)
	err = tx.QueryRow(`
		SELECT t.device_id, d.account_id, t.kind, t.expires_at, t.revoked_at
		FROM auth_tokens t
		JOIN devices d ON d.id = t.device_id
		WHERE t.token_hash = $1
		FOR UPDATE OF t
	`, services.HashToken(req.RefreshToken)).Scan(&deviceID, &accountID, &kind, &expiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && kind != "refresh") {
		unauthorized(c, "invalid refresh token")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	if revokedAt.Valid {
		if _, err := tx.Exec(`UPDATE auth_tokens SET revoked_at = NOW() WHERE device_id = $1 AND revoked_at IS NULL`, deviceID); err != nil {
			internalError(c, err)
			return
		}
		if err := tx.Commit(); err != nil {
			internalError(c, err)
			return
		}
		log.Printf("⚠️ Refresh token reuse detected for device %s; revoked all tokens", deviceID)
		unauthorized(c, "refresh token reuse detected")
		return
	}
	if time.Now().After(expiresAt) {
		unauthorized(c, "refresh token expired")
		return
	}

	if _, err := tx.Exec(`UPDATE auth_tokens SET revoked_at = NOW() WHERE token_hash = $1`, services.HashToken(req.RefreshToken)); err != nil {
		internalError(c, err)
		return
	}
	// Expired rows are no longer needed for reuse detection.
	if _, err := tx.Exec(`DELETE FROM auth_tokens WHERE device_id = $1 AND expires_at < NOW() - INTERVAL '1 day'`, deviceID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := tx.Exec(`UPDATE devices SET last_seen_at = NOW() WHERE id = $1`, deviceID); err != nil {
		internalError(c, err)
		return
	}

	pair, err := issueTokens(tx, accountID, deviceID)
	if err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, pair)
}

func Logout(c *gin.Context) {
	deviceID := c.GetString(ctxDeviceID)

	tx, err := db.Begin()
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE auth_tokens SET revoked_at = NOW() WHERE device_id = $1 AND revoked_at IS NULL`, deviceID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := tx.Exec(`UPDATE devices SET push_token = NULL, push_consented_at = NULL WHERE id = $1`, deviceID); err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteAccount removes the account, its devices, tokens, and subscriptions.
// Community reports and photos stay public but are no longer linked to it.
func DeleteAccount(c *gin.Context) {
	if _, err := db.Exec(`DELETE FROM accounts WHERE id = $1`, c.GetString(ctxAccountID)); err != nil {
		internalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", true
	}
	return token, true
}

func authenticate(c *gin.Context, token string) bool {
	var accountID, deviceID string
	err := db.QueryRow(`
		SELECT d.account_id, t.device_id
		FROM auth_tokens t
		JOIN devices d ON d.id = t.device_id
		WHERE t.token_hash = $1
		AND t.kind = 'access'
		AND t.revoked_at IS NULL
		AND t.expires_at > NOW()
	`, services.HashToken(token)).Scan(&accountID, &deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		unauthorized(c, "invalid or expired access token")
		return false
	}
	if err != nil {
		internalError(c, err)
		return false
	}

	c.Set(ctxAccountID, accountID)
	c.Set(ctxDeviceID, deviceID)
	return true
}

// RequireAuth rejects requests without a valid access token.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, present := bearerToken(c)
		if !present || token == "" {
			unauthorized(c, "missing bearer token")
			return
		}
		if !authenticate(c, token) {
			return
		}
		c.Next()
	}
}

// OptionalAuth attaches the caller's identity when a token is sent. An
// invalid token is still rejected so clients know to refresh it.
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, present := bearerToken(c)
		if !present {
			c.Next()
			return
		}
		if token == "" {
			unauthorized(c, "malformed authorization header")
			return
		}
		if !authenticate(c, token) {
			return
		}
		c.Next()
	}
}

func unauthorized(c *gin.Context, message string) {
	c.Header("WWW-Authenticate", `Bearer realm="aurarisk"`)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}

func internalError(c *gin.Context, err error) {
	log.Printf("❌ %s %s: %v", c.Request.Method, c.FullPath(), err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}
