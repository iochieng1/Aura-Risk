package workers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/models"
	"github.com/lib/pq"
)

const (
	// Arbitrary constant identifying the webhook worker's advisory lock.
	webhookLockID = 7_311_004
	// Reports verified longer ago than this are not announced, so adding a
	// county's webhook doesn't replay its whole history.
	webhookLookback = 24 * time.Hour
	webhookBatch    = 20
	// Retries back off from 1 minute, doubling, up to 6 hours apart.
	webhookMaxAttempts = 10
	webhookMaxBackoff  = 6 * time.Hour
)

// CountyWebhooks posts each newly verified report to its county's webhook
// (#62). Deliveries go through an outbox table, so a report is announced
// once even with several API instances, and failures are retried.
type CountyWebhooks struct {
	DB *sql.DB
	// URLs maps a county's official name to its webhook.
	URLs map[string]string
	// Secret signs each body: X-AuraRisk-Signature is
	// "sha256=" + hex(HMAC-SHA256(secret, timestamp + "." + body)).
	Secret   string
	HTTP     *http.Client
	Interval time.Duration
}

// WebhookPayload is the JSON body sent to a county webhook.
type WebhookPayload struct {
	Event  string                 `json:"event"`
	Report models.CommunityReport `json:"report"`
	SentAt time.Time              `json:"sent_at"`
}

func (w *CountyWebhooks) Run(ctx context.Context) {
	metrics.SetWorkerInterval("county_webhooks", w.Interval)
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		err := w.RunOnce(ctx)
		metrics.ObserveWorkerRun("county_webhooks", err)
		if err != nil {
			log.Printf("❌ County webhooks failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce queues newly verified reports and attempts due deliveries.
func (w *CountyWebhooks) RunOnce(ctx context.Context) error {
	if len(w.URLs) == 0 {
		return nil
	}
	conn, err := w.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, webhookLockID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, webhookLockID)

	counties := make([]string, 0, len(w.URLs))
	for county := range w.URLs {
		counties = append(counties, county)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO report_webhook_deliveries (report_id, county)
		SELECT id, county FROM community_reports
		WHERE moderation_status = 'verified' AND duplicate_of IS NULL AND county = ANY($1)
		AND status_updated_at > NOW() - make_interval(secs => $2)
		ON CONFLICT (report_id) DO NOTHING
	`, pq.Array(counties), webhookLookback.Seconds()); err != nil {
		return err
	}

	rows, err := conn.QueryContext(ctx, `
		SELECT d.attempts, r.id, r.location_name, r.lat, r.lon, r.category, r.note, r.created_at,
			r.moderation_status, r.corroboration_count, r.county, r.ward, r.response_status, r.response_updated_at
		FROM report_webhook_deliveries d
		JOIN community_reports r ON r.id = d.report_id
		WHERE d.delivered_at IS NULL AND d.attempts < $1 AND d.next_attempt_at <= NOW() AND d.county = ANY($2)
		ORDER BY d.next_attempt_at
		LIMIT $3
	`, webhookMaxAttempts, pq.Array(counties), webhookBatch)
	if err != nil {
		return err
	}
	type due struct {
		attempts int
		report   models.CommunityReport
	}
	var deliveries []due
	for rows.Next() {
		var d due
		r := &d.report
		if err := rows.Scan(&d.attempts, &r.ID, &r.Location.Name, &r.Location.Lat, &r.Location.Lon, &r.Category,
			&r.Note, &r.Timestamp, &r.Status, &r.Corroborations, &r.County, &r.Ward, &r.ResponseStatus,
			&r.ResponseUpdatedAt); err != nil {
			rows.Close()
			return err
		}
		r.Photos = []models.ReportPhoto{}
		deliveries = append(deliveries, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, d := range deliveries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sendErr := w.send(ctx, w.URLs[*d.report.County], d.report)
		if sendErr == nil {
			_, err = conn.ExecContext(ctx, `
				UPDATE report_webhook_deliveries SET delivered_at = NOW(), attempts = attempts + 1, last_error = NULL
				WHERE report_id = $1
			`, d.report.ID)
		} else {
			log.Printf("⚠️ Webhook for report %s (%s) failed: %v", d.report.ID, *d.report.County, sendErr)
			backoff := min(time.Minute<<d.attempts, webhookMaxBackoff)
			_, err = conn.ExecContext(ctx, `
				UPDATE report_webhook_deliveries
				SET attempts = attempts + 1, last_error = $2, next_attempt_at = NOW() + make_interval(secs => $3)
				WHERE report_id = $1
			`, d.report.ID, truncate(sendErr.Error(), 500), backoff.Seconds())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *CountyWebhooks) send(ctx context.Context, url string, report models.CommunityReport) error {
	body, err := json.Marshal(WebhookPayload{Event: "report.verified", Report: report, SentAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AuraRisk-Webhooks/1")
	req.Header.Set("X-AuraRisk-Event", "report.verified")
	req.Header.Set("X-AuraRisk-Delivery", report.ID)
	req.Header.Set("X-AuraRisk-Timestamp", timestamp)
	req.Header.Set("X-AuraRisk-Signature", "sha256="+SignWebhook(w.Secret, timestamp, body))

	client := w.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// SignWebhook returns the hex HMAC-SHA256 of timestamp + "." + body.
// Receivers recompute it to check a delivery came from AuraRisk, and reject
// old timestamps to stop replays.
func SignWebhook(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
