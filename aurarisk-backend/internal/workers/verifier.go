package workers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/services"
	"github.com/lib/pq"
)

const (
	// Arbitrary constant identifying the verifier's Postgres advisory lock.
	verifierLockID = 7_311_003
	// Reports older than this are left to moderators; corroboration and
	// photos rarely arrive later.
	verifyLookback = 48 * time.Hour
	verifyBatch    = 500
)

// ReportVerifier scores pending reports and verifies those with enough
// independent evidence. It never rejects, and never touches a report a
// moderator has decided.
type ReportVerifier struct {
	DB       *sql.DB
	Interval time.Duration
}

type pendingReport struct {
	ID        string
	AccountID sql.NullString
	Category  string
	Lat, Lon  float64
}

func (v *ReportVerifier) Run(ctx context.Context) {
	metrics.SetWorkerInterval("report_verifier", v.Interval)
	ticker := time.NewTicker(v.Interval)
	defer ticker.Stop()

	for {
		err := v.RunOnce(ctx)
		metrics.ObserveWorkerRun("report_verifier", err)
		if err != nil {
			log.Printf("❌ Report verification failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce rescores recent pending reports. An advisory lock keeps multiple
// API instances from doing the same work.
func (v *ReportVerifier) RunOnce(ctx context.Context) error {
	conn, err := v.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, verifierLockID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, verifierLockID)

	reports, err := v.loadPending(ctx)
	if err != nil {
		return err
	}

	verified := 0
	for _, r := range reports {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		evidence, corroborations, err := v.gatherEvidence(ctx, r)
		if err != nil {
			return fmt.Errorf("report %s: %w", r.ID, err)
		}
		score := services.VerificationScore(evidence)
		ok, err := v.record(ctx, r.ID, score, corroborations)
		if err != nil {
			return fmt.Errorf("report %s: %w", r.ID, err)
		}
		if ok {
			verified++
		}
	}
	metrics.AddReportsAutoVerified(verified)

	var queue int
	if err := v.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM community_reports WHERE moderation_status = 'pending' AND duplicate_of IS NULL
	`).Scan(&queue); err != nil {
		return err
	}
	metrics.SetModerationQueue(queue)
	return nil
}

func (v *ReportVerifier) loadPending(ctx context.Context) ([]pendingReport, error) {
	rows, err := v.DB.QueryContext(ctx, `
		SELECT id, account_id, category, lat, lon
		FROM community_reports
		WHERE moderation_status = 'pending' AND moderated_by IS NULL AND duplicate_of IS NULL
		AND created_at > NOW() - make_interval(secs => $1)
		ORDER BY created_at DESC
		LIMIT $2
	`, verifyLookback.Seconds(), verifyBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []pendingReport
	for rows.Next() {
		var r pendingReport
		if err := rows.Scan(&r.ID, &r.AccountID, &r.Category, &r.Lat, &r.Lon); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// gatherEvidence collects corroborating reports, photos, and the reporter's
// moderation history. It returns the evidence and the number of
// corroborating reports within the radius.
func (v *ReportVerifier) gatherEvidence(ctx context.Context, r pendingReport) (services.Evidence, int, error) {
	e := services.Evidence{SignedIn: r.AccountID.Valid}

	minLat, maxLat, minLon, maxLon := services.BoundingBox(r.Lat, r.Lon, services.CorroborationRadiusMeters)
	rows, err := v.DB.QueryContext(ctx, `
		SELECT o.account_id, o.lat, o.lon
		FROM community_reports o, community_reports r
		WHERE r.id = $1 AND o.id <> r.id
		AND o.duplicate_of IS NULL AND o.moderation_status <> 'rejected'
		AND o.category = ANY($2)
		AND o.lat BETWEEN $3 AND $4 AND o.lon BETWEEN $5 AND $6
		AND o.created_at BETWEEN r.created_at - make_interval(secs => $7) AND r.created_at + make_interval(secs => $7)
		AND (r.account_id IS NULL OR o.account_id IS DISTINCT FROM r.account_id)
	`, r.ID, pq.Array(services.CategoriesForGroup(r.Category)), minLat, maxLat, minLon, maxLon,
		services.CorroborationWindow.Seconds())
	if err != nil {
		return e, 0, err
	}
	defer rows.Close()

	accounts := map[string]bool{}
	corroborations := 0
	for rows.Next() {
		var account sql.NullString
		var lat, lon float64
		if err := rows.Scan(&account, &lat, &lon); err != nil {
			return e, 0, err
		}
		if services.DistanceMeters(r.Lat, r.Lon, lat, lon) > services.CorroborationRadiusMeters {
			continue
		}
		corroborations++
		if account.Valid {
			accounts[account.String] = true
		} else {
			e.AnonymousCorroborations++
		}
	}
	if err := rows.Err(); err != nil {
		return e, 0, err
	}
	e.IndependentReporters = len(accounts)

	err = v.DB.QueryRowContext(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM report_photos WHERE report_id = $1 AND status = 'ready'),
			COUNT(*) FILTER (WHERE h.moderation_status = 'verified'),
			COUNT(*) FILTER (WHERE h.moderation_status = 'rejected')
		FROM community_reports h
		WHERE $2::uuid IS NOT NULL AND h.account_id = $2::uuid AND h.id <> $1
		AND h.moderated_by IS NOT NULL AND h.moderated_by <> $3
	`, r.ID, r.AccountID, services.ModeratedByAuto).Scan(&e.HasReadyPhoto, &e.ReporterVerified, &e.ReporterRejected)
	if err != nil {
		return e, 0, err
	}
	return e, corroborations, nil
}

// record stores the score and verifies the report when it qualifies. The
// WHERE clause re-checks that no moderator decided it in the meantime.
func (v *ReportVerifier) record(ctx context.Context, id string, score, corroborations int) (bool, error) {
	tx, err := v.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	verify := score >= services.AutoVerifyScore
	res, err := tx.ExecContext(ctx, `
		UPDATE community_reports
		SET verification_score = $2, corroboration_count = $3,
			moderation_status = CASE WHEN $4 THEN 'verified' ELSE moderation_status END,
			moderated_by = CASE WHEN $4 THEN $5 ELSE moderated_by END,
			status_updated_at = CASE WHEN $4 THEN NOW() ELSE status_updated_at END
		WHERE id = $1 AND moderation_status = 'pending' AND moderated_by IS NULL
	`, id, score, corroborations, verify, services.ModeratedByAuto)
	if err != nil {
		return false, err
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if verify && updated == 1 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO report_moderation_events (report_id, from_status, to_status, actor, reason)
			VALUES ($1, 'pending', 'verified', $2, $3)
		`, id, services.ModeratedByAuto, fmt.Sprintf("verification score %d with %d corroborating reports", score, corroborations)); err != nil {
			return false, err
		}
	}
	return verify && updated == 1, tx.Commit()
}
