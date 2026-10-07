package workers

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/services"
	"aurarisk-backend/internal/storage"
)

const (
	maxPhotoAttempts = 5
	photoLease       = 5 * time.Minute
	maxPhotoBytes    = 10 << 20
)

type Scanner interface {
	Scan(ctx context.Context, r io.Reader) (services.ScanResult, error)
}

// PhotoProcessor scans, resizes, and publishes uploaded photos. Rows are
// claimed with SKIP LOCKED and a lease, so several API instances can run it.
type PhotoProcessor struct {
	DB       *sql.DB
	Store    storage.ObjectStore
	Scanner  Scanner
	Interval time.Duration
}

type claimedPhoto struct {
	ID            string
	ReportID      string
	QuarantineKey string
	SizeBytes     int64
	Attempts      int
}

func (p *PhotoProcessor) Run(ctx context.Context) {
	if p.Scanner == nil {
		log.Println("⚠️ CLAMD_ADDR is not set: uploaded photos will stay unpublished until a malware scanner is configured")
		return
	}
	metrics.SetWorkerInterval("photo_processor", p.Interval)
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		metrics.ObserveWorkerRun("photo_processor", p.drain(ctx))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// drain processes photos until none are claimable. It returns an error only
// when the queue itself cannot be read; per-photo failures are retried.
func (p *PhotoProcessor) drain(ctx context.Context) error {
	for ctx.Err() == nil {
		photo, err := p.claim(ctx)
		if err != nil {
			log.Printf("❌ Photo claim failed: %v", err)
			return err
		}
		if photo == nil {
			return nil
		}
		p.ProcessOne(ctx, photo)
	}
	return ctx.Err()
}

func (p *PhotoProcessor) claim(ctx context.Context) (*claimedPhoto, error) {
	var photo claimedPhoto
	err := p.DB.QueryRowContext(ctx, `
		UPDATE report_photos
		SET locked_until = NOW() + make_interval(secs => $1), attempts = attempts + 1
		WHERE id = (
			SELECT id FROM report_photos
			WHERE status = 'processing' AND (locked_until IS NULL OR locked_until < NOW())
			ORDER BY updated_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, report_id, quarantine_key, size_bytes, attempts
	`, photoLease.Seconds()).Scan(&photo.ID, &photo.ReportID, &photo.QuarantineKey, &photo.SizeBytes, &photo.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &photo, nil
}

func (p *PhotoProcessor) ProcessOne(ctx context.Context, photo *claimedPhoto) {
	if photo.Attempts > maxPhotoAttempts {
		p.finish(ctx, photo, "failed", "too many processing attempts")
		return
	}

	data, err := p.Store.Get(ctx, photo.QuarantineKey, maxPhotoBytes)
	if errors.Is(err, storage.ErrNotFound) {
		p.finish(ctx, photo, "rejected", "upload not found")
		return
	}
	if err != nil {
		p.retry(ctx, photo, fmt.Errorf("download: %w", err))
		return
	}
	if int64(len(data)) != photo.SizeBytes {
		p.finish(ctx, photo, "rejected", "uploaded size does not match declared size")
		return
	}

	result, err := p.Scanner.Scan(ctx, bytes.NewReader(data))
	if err != nil {
		p.retry(ctx, photo, fmt.Errorf("scan: %w", err))
		return
	}
	if !result.Clean {
		log.Printf("🛑 Photo %s rejected: malware signature %s", photo.ID, result.Signature)
		p.finish(ctx, photo, "rejected", "malware detected: "+result.Signature)
		return
	}

	processed, err := services.ProcessImage(data)
	if errors.Is(err, services.ErrUnsupportedImage) {
		p.finish(ctx, photo, "rejected", err.Error())
		return
	}
	if err != nil {
		p.retry(ctx, photo, fmt.Errorf("process: %w", err))
		return
	}

	largeKey := fmt.Sprintf("photos/%s/%s/large.jpg", photo.ReportID, photo.ID)
	thumbKey := fmt.Sprintf("photos/%s/%s/thumb.jpg", photo.ReportID, photo.ID)
	if err := p.Store.Put(ctx, largeKey, "image/jpeg", processed.Large); err != nil {
		p.retry(ctx, photo, fmt.Errorf("store large: %w", err))
		return
	}
	if err := p.Store.Put(ctx, thumbKey, "image/jpeg", processed.Thumb); err != nil {
		p.retry(ctx, photo, fmt.Errorf("store thumb: %w", err))
		return
	}

	_, err = p.DB.ExecContext(ctx, `
		UPDATE report_photos
		SET status = 'ready', large_key = $2, thumb_key = $3, locked_until = NULL, updated_at = NOW()
		WHERE id = $1
	`, photo.ID, largeKey, thumbKey)
	if err != nil {
		p.retry(ctx, photo, fmt.Errorf("mark ready: %w", err))
		return
	}
	p.deleteQuarantine(ctx, photo)
}

func (p *PhotoProcessor) finish(ctx context.Context, photo *claimedPhoto, status, reason string) {
	_, err := p.DB.ExecContext(ctx, `
		UPDATE report_photos
		SET status = $2, rejection_reason = $3, locked_until = NULL, updated_at = NOW()
		WHERE id = $1
	`, photo.ID, status, reason)
	if err != nil {
		log.Printf("❌ Failed to mark photo %s %s: %v", photo.ID, status, err)
		return
	}
	p.deleteQuarantine(ctx, photo)
}

// retry releases the lease after an exponential backoff so a transient
// scanner or storage outage doesn't burn through every attempt at once.
func (p *PhotoProcessor) retry(ctx context.Context, photo *claimedPhoto, cause error) {
	log.Printf("⚠️ Photo %s attempt %d failed: %v", photo.ID, photo.Attempts, cause)
	backoff := time.Duration(1<<photo.Attempts) * 30 * time.Second
	_, err := p.DB.ExecContext(ctx, `
		UPDATE report_photos SET locked_until = NOW() + make_interval(secs => $2) WHERE id = $1
	`, photo.ID, backoff.Seconds())
	if err != nil {
		log.Printf("❌ Failed to schedule retry for photo %s: %v", photo.ID, err)
	}
}

func (p *PhotoProcessor) deleteQuarantine(ctx context.Context, photo *claimedPhoto) {
	if err := p.Store.Delete(ctx, photo.QuarantineKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
		log.Printf("⚠️ Failed to delete quarantine object %s: %v", photo.QuarantineKey, err)
	}
}
