package workers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"aurarisk-backend/internal/metrics"
	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
)

const (
	// Repeat an unchanged elevated level at most this often.
	renotifyAfter = 12 * time.Hour
	// Arbitrary constant identifying the notifier's Postgres advisory lock.
	notifierLockID = 7_311_002
)

type PushSender interface {
	Send(ctx context.Context, messages []services.PushMessage) ([]services.PushTicket, error)
}

type RiskAssessor func(ctx context.Context, lat, lon float64, name string) (*models.RiskAssessment, error)

// Notifier evaluates risk for subscribed locations and pushes alerts to
// devices that consented, honoring each subscription's quiet hours.
type Notifier struct {
	DB       *sql.DB
	Push     PushSender
	Assess   RiskAssessor
	Interval time.Duration
	Now      func() time.Time
}

type notifierSubscription struct {
	ID                string
	AccountID         string
	Name              string
	Lat, Lon          float64
	MinLevel          string
	QuietHours        *services.QuietHours
	AllowEmergency    bool
	LastNotifiedLevel string
	LastNotifiedAt    time.Time
}

type decision int

const (
	decisionNone decision = iota
	decisionNotify
	// decisionLower records a lower (still elevated) level without notifying,
	// so a later escalation notifies again.
	decisionLower
	// decisionReset clears state once risk drops below the threshold.
	decisionReset
)

func decide(sub notifierSubscription, level string, now time.Time) decision {
	rank := services.LevelRank(level)
	if rank < services.LevelRank(sub.MinLevel) {
		if sub.LastNotifiedLevel != "" {
			return decisionReset
		}
		return decisionNone
	}

	lastRank := services.LevelRank(sub.LastNotifiedLevel)
	switch {
	case sub.LastNotifiedLevel == "", rank > lastRank, now.Sub(sub.LastNotifiedAt) >= renotifyAfter:
	case rank < lastRank:
		return decisionLower
	default:
		return decisionNone
	}

	if sub.QuietHours != nil && sub.QuietHours.Contains(now) {
		if !(level == "Emergency" && sub.AllowEmergency) {
			// Left pending; it is sent on the first run after quiet hours.
			return decisionNone
		}
	}
	return decisionNotify
}

func (n *Notifier) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

func (n *Notifier) Run(ctx context.Context) {
	metrics.SetWorkerInterval("notifier", n.Interval)
	ticker := time.NewTicker(n.Interval)
	defer ticker.Stop()

	for {
		err := n.RunOnce(ctx)
		metrics.ObserveWorkerRun("notifier", err)
		if err != nil {
			log.Printf("❌ Notifier run failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce evaluates every subscription with at least one consenting device.
// An advisory lock keeps multiple API instances from double-sending.
func (n *Notifier) RunOnce(ctx context.Context) error {
	conn, err := n.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, notifierLockID).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, notifierLockID)

	subs, err := n.loadSubscriptions(ctx)
	if err != nil {
		return err
	}

	// Nearby subscriptions share one weather lookup per run.
	assessments := map[string]*models.RiskAssessment{}
	now := n.now()

	for _, sub := range subs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := fmt.Sprintf("%.2f,%.2f", sub.Lat, sub.Lon)
		assessment, ok := assessments[key]
		if !ok {
			assessment, err = n.Assess(ctx, sub.Lat, sub.Lon, sub.Name)
			if err != nil {
				log.Printf("⚠️ Risk assessment failed for subscription %s: %v", sub.ID, err)
				continue
			}
			// Alerts describe current conditions, so never send one from
			// cached data. The next run retries with live data.
			if assessment.Stale {
				log.Printf("⚠️ Skipping subscription %s: weather data is stale", sub.ID)
				continue
			}
			assessments[key] = assessment
		}

		switch decide(sub, assessment.Level, now) {
		case decisionNotify:
			if err := n.notify(ctx, sub, assessment); err != nil {
				log.Printf("⚠️ Notify failed for subscription %s: %v", sub.ID, err)
			}
		case decisionLower:
			n.setLastNotified(ctx, sub.ID, assessment.Level, false)
		case decisionReset:
			if _, err := n.DB.ExecContext(ctx, `
				UPDATE location_subscriptions SET last_notified_level = NULL, last_notified_at = NULL WHERE id = $1
			`, sub.ID); err != nil {
				log.Printf("⚠️ Failed to reset subscription %s: %v", sub.ID, err)
			}
		}
	}
	return nil
}

func (n *Notifier) loadSubscriptions(ctx context.Context) ([]notifierSubscription, error) {
	rows, err := n.DB.QueryContext(ctx, `
		SELECT s.id, s.account_id, s.name, s.lat, s.lon, s.min_level,
			s.quiet_start, s.quiet_end, s.quiet_timezone, s.allow_emergency_during_quiet,
			s.last_notified_level, s.last_notified_at
		FROM location_subscriptions s
		WHERE EXISTS (
			SELECT 1 FROM devices d
			WHERE d.account_id = s.account_id
			AND d.push_token IS NOT NULL
			AND d.push_consented_at IS NOT NULL
		)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []notifierSubscription
	for rows.Next() {
		var (
			s                     notifierSubscription
			start, end, tz, level sql.NullString
			notifiedAt            sql.NullTime
		)
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Name, &s.Lat, &s.Lon, &s.MinLevel,
			&start, &end, &tz, &s.AllowEmergency, &level, &notifiedAt); err != nil {
			return nil, err
		}
		if start.Valid && end.Valid && tz.Valid {
			s.QuietHours = &services.QuietHours{Start: start.String, End: end.String, Timezone: tz.String}
		}
		s.LastNotifiedLevel = level.String
		s.LastNotifiedAt = notifiedAt.Time
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (n *Notifier) notify(ctx context.Context, sub notifierSubscription, assessment *models.RiskAssessment) error {
	rows, err := n.DB.QueryContext(ctx, `
		SELECT id, push_token FROM devices
		WHERE account_id = $1 AND push_token IS NOT NULL AND push_consented_at IS NOT NULL
	`, sub.AccountID)
	if err != nil {
		return err
	}
	var deviceIDs []string
	var messages []services.PushMessage
	for rows.Next() {
		var deviceID, token string
		if err := rows.Scan(&deviceID, &token); err != nil {
			rows.Close()
			return err
		}
		deviceIDs = append(deviceIDs, deviceID)
		messages = append(messages, alertMessage(token, sub, assessment))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	tickets, err := n.Push.Send(ctx, messages)
	if err != nil {
		return err
	}

	delivered := false
	for i, ticket := range tickets {
		switch {
		case ticket.Status == "ok":
			metrics.ObservePushTicket("ok")
			delivered = true
		case ticket.DeviceGone():
			metrics.ObservePushTicket("device_gone")
			if _, err := n.DB.ExecContext(ctx, `
				UPDATE devices SET push_token = NULL, push_consented_at = NULL WHERE id = $1
			`, deviceIDs[i]); err != nil {
				log.Printf("⚠️ Failed to clear stale push token for device %s: %v", deviceIDs[i], err)
			}
		default:
			metrics.ObservePushTicket("error")
			log.Printf("⚠️ Push to device %s failed: %s %s", deviceIDs[i], ticket.Details.Error, ticket.Message)
		}
	}
	if delivered {
		n.setLastNotified(ctx, sub.ID, assessment.Level, true)
	}
	return nil
}

func (n *Notifier) setLastNotified(ctx context.Context, subID, level string, touchTime bool) {
	_, err := n.DB.ExecContext(ctx, `
		UPDATE location_subscriptions
		SET last_notified_level = $2,
			last_notified_at = CASE WHEN $3 THEN NOW() ELSE last_notified_at END
		WHERE id = $1
	`, subID, level, touchTime)
	if err != nil {
		log.Printf("⚠️ Failed to update subscription %s: %v", subID, err)
	}
}

func alertMessage(token string, sub notifierSubscription, assessment *models.RiskAssessment) services.PushMessage {
	body := fmt.Sprintf("Flood risk is %s (%d/100).", assessment.Level, assessment.Score)
	if len(assessment.Tips) > 0 {
		body += " " + assessment.Tips[0]
	}
	return services.PushMessage{
		To:        token,
		Title:     fmt.Sprintf("%s: %s", assessment.Level, sub.Name),
		Body:      body,
		Sound:     "default",
		Priority:  "high",
		ChannelID: services.RiskAlertChannelID,
		Data: map[string]any{
			"type":            "risk_alert",
			"subscription_id": sub.ID,
			"level":           assessment.Level,
			"score":           assessment.Score,
			"lat":             sub.Lat,
			"lon":             sub.Lon,
		},
	}
}
