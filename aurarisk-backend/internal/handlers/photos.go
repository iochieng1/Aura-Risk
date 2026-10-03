package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
	"aurarisk-backend/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

const (
	MaxPhotoBytes      = 10 << 20
	MaxPhotosPerReport = 4
	photoUploadTTL     = 15 * time.Minute
	photoViewTTL       = time.Hour
)

var photoStore storage.ObjectStore

func SetPhotoStore(store storage.ObjectStore) {
	photoStore = store
}

type photoUploadRequest struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

func QuarantineKey(reportID, photoID string) string {
	return fmt.Sprintf("quarantine/%s/%s", reportID, photoID)
}

func requirePhotoStore(c *gin.Context) bool {
	if photoStore == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "photo uploads are not configured"})
		return false
	}
	return true
}

// CreatePhotoUpload reserves a photo slot on a report and returns a presigned
// URL for uploading the original into the quarantine prefix.
func CreatePhotoUpload(c *gin.Context) {
	if !requirePhotoStore(c) {
		return
	}
	reportID := c.Param("id")
	if !uuidPattern.MatchString(reportID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}

	var req photoUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if !services.AllowedPhotoTypes[req.ContentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content_type must be image/jpeg, image/png, or image/webp"})
		return
	}
	if req.SizeBytes <= 0 || req.SizeBytes > MaxPhotoBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("size_bytes must be between 1 and %d", MaxPhotoBytes)})
		return
	}

	accountID := c.GetString(ctxAccountID)

	tx, err := db.Begin()
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var owner sql.NullString
	err = tx.QueryRow(`SELECT account_id FROM community_reports WHERE id = $1 FOR UPDATE`, reportID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if !owner.Valid || owner.String != accountID {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the report author can attach photos"})
		return
	}

	var count int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM report_photos
		WHERE report_id = $1
		AND (status IN ('processing', 'ready')
			OR (status = 'pending_upload' AND created_at > NOW() - make_interval(secs => $2)))
	`, reportID, photoUploadTTL.Seconds()).Scan(&count); err != nil {
		internalError(c, err)
		return
	}
	if count >= MaxPhotosPerReport {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("a report can have at most %d photos", MaxPhotosPerReport)})
		return
	}

	var photoID string
	if err := tx.QueryRow(`SELECT gen_random_uuid()`).Scan(&photoID); err != nil {
		internalError(c, err)
		return
	}
	key := QuarantineKey(reportID, photoID)

	if _, err := tx.Exec(`
		INSERT INTO report_photos (id, report_id, account_id, status, content_type, size_bytes, quarantine_key)
		VALUES ($1, $2, $3, 'pending_upload', $4, $5, $6)
	`, photoID, reportID, accountID, req.ContentType, req.SizeBytes, key); err != nil {
		internalError(c, err)
		return
	}

	presigned, err := photoStore.PresignPut(c.Request.Context(), key, req.ContentType, req.SizeBytes, photoUploadTTL)
	if err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"photo_id":       photoID,
		"upload_url":     presigned.URL,
		"upload_method":  presigned.Method,
		"upload_headers": presigned.Headers,
		"expires_at":     presigned.ExpiresAt,
	})
}

// CompletePhotoUpload hands an uploaded photo to the processing worker.
func CompletePhotoUpload(c *gin.Context) {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "photo not found"})
		return
	}

	// Abandoned slots stop counting toward the per-report limit once their
	// upload URL expires, so they can't be completed afterwards either.
	var status string
	var expired bool
	err := db.QueryRow(`
		UPDATE report_photos
		SET status = CASE WHEN status = 'pending_upload' AND NOT expired THEN 'processing' ELSE status END,
			updated_at = CASE WHEN status = 'pending_upload' AND NOT expired THEN NOW() ELSE updated_at END
		FROM (SELECT created_at <= NOW() - make_interval(secs => $3) AS expired FROM report_photos WHERE id = $1) AS slot
		WHERE id = $1 AND account_id = $2
		RETURNING status, slot.expired
	`, id, c.GetString(ctxAccountID), photoUploadTTL.Seconds()).Scan(&status, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "photo not found"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if status == "pending_upload" && expired {
		c.JSON(http.StatusGone, gin.H{"error": "upload slot expired; request a new one"})
		return
	}

	c.JSON(http.StatusAccepted, models.Photo{ID: id, Status: status})
}

func GetPhoto(c *gin.Context) {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "photo not found"})
		return
	}

	var (
		photo              models.Photo
		largeKey, thumbKey sql.NullString
	)
	err := db.QueryRow(`
		SELECT id, status, large_key, thumb_key FROM report_photos
		WHERE id = $1 AND account_id = $2
	`, id, c.GetString(ctxAccountID)).Scan(&photo.ID, &photo.Status, &largeKey, &thumbKey)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "photo not found"})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	if photo.Status == "ready" && photoStore != nil {
		if photo.LargeURL, err = photoStore.PresignGet(c.Request.Context(), largeKey.String, photoViewTTL); err != nil {
			internalError(c, err)
			return
		}
		if photo.ThumbURL, err = photoStore.PresignGet(c.Request.Context(), thumbKey.String, photoViewTTL); err != nil {
			internalError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, photo)
}

// readyPhotosByReport returns presigned URLs for ready photos, keyed by report.
func readyPhotosByReport(c *gin.Context, reportIDs []string) (map[string][]models.ReportPhoto, error) {
	result := map[string][]models.ReportPhoto{}
	if photoStore == nil || len(reportIDs) == 0 {
		return result, nil
	}

	rows, err := db.Query(`
		SELECT id, report_id, large_key, thumb_key FROM report_photos
		WHERE report_id = ANY($1) AND status = 'ready'
		ORDER BY created_at
	`, pq.Array(reportIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var photo models.ReportPhoto
		var reportID, largeKey, thumbKey string
		if err := rows.Scan(&photo.ID, &reportID, &largeKey, &thumbKey); err != nil {
			return nil, err
		}
		if photo.LargeURL, err = photoStore.PresignGet(c.Request.Context(), largeKey, photoViewTTL); err != nil {
			return nil, err
		}
		if photo.ThumbURL, err = photoStore.PresignGet(c.Request.Context(), thumbKey, photoViewTTL); err != nil {
			return nil, err
		}
		result[reportID] = append(result[reportID], photo)
	}
	return result, rows.Err()
}
