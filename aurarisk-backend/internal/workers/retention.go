package workers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/storage"
	"github.com/lib/pq"
)

const (
	// Arbitrary constant identifying the retention worker's advisory lock.
	retentionLockID = 7_311_004
	retentionBatch  = 200
	// Upper bound on batches per run so one run cannot hold the lock for hours.
	retentionMaxBatches = 25
)

// RetentionPolicy says how long data is kept. A zero duration keeps that
// kind of data forever.
type RetentionPolicy struct {
	Rejected  time.Duration // rejected reports, from the time they were rejected
	Duplicate time.Duration // duplicate reports, from creation
	Pending   time.Duration // reports never verified or rejected, from creation
	Verified  time.Duration // verified reports, from creation
	// Photo slots whose upload never completed.
	AbandonedUpload time.Duration
	// Photos that failed processing or were rejected (e.g. malware).
	FailedPhoto time.Duration
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		Rejected:        30 * 24 * time.Hour,
		Duplicate:       30 * 24 * time.Hour,
		Pending:         180 * 24 * time.Hour,
		Verified:        730 * 24 * time.Hour,
		AbandonedUpload: 24 * time.Hour,
		FailedPhoto:     30 * 24 * time.Hour,
	}
}

// RetentionCleaner deletes expired reports and photos, removing stored photo
// objects before the rows that reference them.
type RetentionCleaner struct {
	DB       *sql.DB
	Store    storage.ObjectStore // nil when photo storage is not configured
	Policy   RetentionPolicy
	Interval time.Duration
}

func (r *RetentionCleaner) Run(ctx context.Context) {
	metrics.SetWorkerInterval("retention", r.Interval)
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	for {
		err := r.RunOnce(ctx)
		metrics.ObserveWorkerRun("retention", err)
		if err != nil {
			log.Printf("❌ Retention cleanup failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *RetentionCleaner) RunOnce(ctx context.Context) error {
	conn, err := r.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, retentionLockID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, retentionLockID)

	if err := r.cleanReports(ctx); err != nil {
		return fmt.Errorf("reports: %w", err)
	}
	if err := r.cleanPhotos(ctx); err != nil {
		return fmt.Errorf("photos: %w", err)
	}
	return nil
}

type expiredReport struct {
	ID     string
	Reason string
}

func (r *RetentionCleaner) cleanReports(ctx context.Context) error {
	for batch := 0; batch < retentionMaxBatches; batch++ {
		expired, err := r.expiredReports(ctx)
		if err != nil {
			return err
		}
		if len(expired) == 0 {
			return nil
		}

		deletable := map[string]bool{}
		for _, e := range expired {
			deletable[e.ID] = true
		}
		ids := make([]string, 0, len(expired))
		for _, e := range expired {
			ids = append(ids, e.ID)
		}

		// Duplicates of an expired report go with it (ON DELETE CASCADE), so
		// their photos must be removed too.
		keysByReport, err := r.photoKeys(ctx, `
			SELECT CASE WHEN r.id = ANY($1) THEN r.id ELSE r.duplicate_of END, p.quarantine_key, p.large_key, p.thumb_key
			FROM report_photos p JOIN community_reports r ON r.id = p.report_id
			WHERE r.id = ANY($1) OR r.duplicate_of = ANY($1)
		`, pq.Array(ids))
		if err != nil {
			return err
		}
		for reportID, keys := range keysByReport {
			if !r.deleteObjects(ctx, keys) {
				delete(deletable, reportID)
			}
		}

		var remove []string
		counts := map[string]int{}
		for _, e := range expired {
			if deletable[e.ID] {
				remove = append(remove, e.ID)
				counts[e.Reason]++
			}
		}
		if len(remove) == 0 {
			// Everything in this batch is blocked on object storage; retry next run.
			return nil
		}
		if _, err := r.DB.ExecContext(ctx, `DELETE FROM community_reports WHERE id = ANY($1)`, pq.Array(remove)); err != nil {
			return err
		}
		for reason, n := range counts {
			metrics.AddRetentionDeleted("report", reason, n)
		}
		log.Printf("🧹 Retention removed %d reports", len(remove))

		if len(remove) < len(expired) || len(expired) < retentionBatch {
			return nil
		}
	}
	return nil
}

func (r *RetentionCleaner) expiredReports(ctx context.Context) ([]expiredReport, error) {
	// A zero duration disables its rule ($n > 0).
	p := r.Policy
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, reason FROM (
			SELECT id, created_at,
				CASE
					WHEN duplicate_of IS NOT NULL THEN 'duplicate'
					ELSE moderation_status
				END AS reason,
				CASE
					WHEN duplicate_of IS NOT NULL THEN $2 > 0 AND created_at < NOW() - make_interval(secs => $2)
					WHEN moderation_status = 'rejected' THEN $1 > 0 AND status_updated_at < NOW() - make_interval(secs => $1)
					WHEN moderation_status = 'pending' THEN $3 > 0 AND created_at < NOW() - make_interval(secs => $3)
					WHEN moderation_status = 'verified' THEN $4 > 0 AND created_at < NOW() - make_interval(secs => $4)
				END AS expired
			FROM community_reports
		) r
		WHERE expired
		ORDER BY created_at
		LIMIT $5
	`, p.Rejected.Seconds(), p.Duplicate.Seconds(), p.Pending.Seconds(), p.Verified.Seconds(), retentionBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []expiredReport
	for rows.Next() {
		var e expiredReport
		if err := rows.Scan(&e.ID, &e.Reason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *RetentionCleaner) cleanPhotos(ctx context.Context) error {
	p := r.Policy
	keysByPhoto, err := r.photoKeys(ctx, `
		SELECT id, quarantine_key, large_key, thumb_key FROM report_photos
		WHERE ($1 > 0 AND status = 'pending_upload' AND created_at < NOW() - make_interval(secs => $1))
		OR ($2 > 0 AND status IN ('failed', 'rejected') AND updated_at < NOW() - make_interval(secs => $2))
		ORDER BY created_at
		LIMIT $3
	`, p.AbandonedUpload.Seconds(), p.FailedPhoto.Seconds(), retentionBatch)
	if err != nil {
		return err
	}

	var remove []string
	for id, keys := range keysByPhoto {
		if r.deleteObjects(ctx, keys) {
			remove = append(remove, id)
		}
	}
	if len(remove) == 0 {
		return nil
	}
	if _, err := r.DB.ExecContext(ctx, `DELETE FROM report_photos WHERE id = ANY($1)`, pq.Array(remove)); err != nil {
		return err
	}
	metrics.AddRetentionDeleted("photo", "expired", len(remove))
	log.Printf("🧹 Retention removed %d photos", len(remove))
	return nil
}

// photoKeys runs a query returning (group id, quarantine, large, thumb) rows
// and groups the non-empty object keys by the first column.
func (r *RetentionCleaner) photoKeys(ctx context.Context, query string, args ...any) (map[string][]string, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var group string
		var quarantine, large, thumb sql.NullString
		if err := rows.Scan(&group, &quarantine, &large, &thumb); err != nil {
			return nil, err
		}
		for _, k := range []sql.NullString{quarantine, large, thumb} {
			if k.Valid && k.String != "" {
				out[group] = append(out[group], k.String)
			}
		}
		if _, ok := out[group]; !ok {
			out[group] = nil
		}
	}
	return out, rows.Err()
}

// deleteObjects removes stored objects and reports whether the rows that
// reference them can now be deleted. Missing objects count as deleted.
func (r *RetentionCleaner) deleteObjects(ctx context.Context, keys []string) bool {
	if len(keys) == 0 {
		return true
	}
	if r.Store == nil {
		log.Printf("⚠️ Retention skipped data with %d stored photo objects: object storage is not configured", len(keys))
		return false
	}
	for _, key := range keys {
		if err := r.Store.Delete(ctx, key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Printf("⚠️ Retention could not delete object %s: %v", key, err)
			return false
		}
	}
	return true
}
