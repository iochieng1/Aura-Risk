package workers

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"aurarisk-backend/internal/database"
	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
)

type fakePush struct {
	sent    []services.PushMessage
	tickets func(services.PushMessage) services.PushTicket
}

func (f *fakePush) Send(_ context.Context, messages []services.PushMessage) ([]services.PushTicket, error) {
	f.sent = append(f.sent, messages...)
	tickets := make([]services.PushTicket, len(messages))
	for i, m := range messages {
		tickets[i] = f.tickets(m)
	}
	return tickets, nil
}

// Requires TEST_DATABASE_URL pointing at a disposable database.
func TestNotifierRunOnce(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", dsn)
	db := database.Connect()
	defer db.Close()
	database.Migrate(db)

	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var accountID string
	if err := db.QueryRow(`INSERT INTO accounts DEFAULT VALUES RETURNING id`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM accounts WHERE id = $1`, accountID)

	mustExec(`INSERT INTO devices (account_id, platform, push_token, push_consented_at) VALUES ($1, 'ios', 'ExponentPushToken[good]', NOW())`, accountID)
	mustExec(`INSERT INTO devices (account_id, platform, push_token, push_consented_at) VALUES ($1, 'android', 'ExponentPushToken[gone]', NOW())`, accountID)
	// Token present but no consent: must never receive a push.
	mustExec(`INSERT INTO devices (account_id, platform, push_token) VALUES ($1, 'android', 'ExponentPushToken[noconsent]')`, accountID)
	mustExec(`INSERT INTO location_subscriptions (account_id, name, lat, lon, min_level) VALUES ($1, 'River', 1.5, 2.5, 'Alert')`, accountID)

	push := &fakePush{tickets: func(m services.PushMessage) services.PushTicket {
		if m.To == "ExponentPushToken[gone]" {
			t := services.PushTicket{Status: "error"}
			t.Details.Error = "DeviceNotRegistered"
			return t
		}
		return services.PushTicket{Status: "ok", ID: "ticket"}
	}}
	level := "Emergency"
	n := &Notifier{
		DB:   db,
		Push: push,
		Assess: func(_ context.Context, lat, lon float64, name string) (*models.RiskAssessment, error) {
			return &models.RiskAssessment{Level: level, Score: 90, Tips: []string{"Move to higher ground."}}, nil
		},
	}

	if err := n.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(push.sent) != 2 {
		t.Fatalf("sent %d messages, want 2 (consenting devices only)", len(push.sent))
	}
	for _, m := range push.sent {
		if m.To == "ExponentPushToken[noconsent]" {
			t.Fatal("pushed to a device without consent")
		}
		if m.ChannelID != services.RiskAlertChannelID || m.Data["type"] != "risk_alert" {
			t.Fatalf("unexpected message %+v", m)
		}
	}

	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM devices WHERE account_id = $1 AND push_token = 'ExponentPushToken[gone]'`, accountID).Scan(&remaining)
	if remaining != 0 {
		t.Fatal("DeviceNotRegistered token was not cleared")
	}

	var lastLevel sql.NullString
	var lastAt sql.NullTime
	db.QueryRow(`SELECT last_notified_level, last_notified_at FROM location_subscriptions WHERE account_id = $1`, accountID).Scan(&lastLevel, &lastAt)
	if lastLevel.String != "Emergency" || !lastAt.Valid || time.Since(lastAt.Time) > time.Minute {
		t.Fatalf("last notified = %v at %v", lastLevel, lastAt)
	}

	// Same level again: deduplicated.
	push.sent = nil
	if err := n.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(push.sent) != 0 {
		t.Fatalf("sent %d duplicate messages", len(push.sent))
	}

	// Risk drops below threshold: state resets so the next rise notifies.
	level = "Normal"
	n.RunOnce(context.Background())
	db.QueryRow(`SELECT last_notified_level FROM location_subscriptions WHERE account_id = $1`, accountID).Scan(&lastLevel)
	if lastLevel.Valid {
		t.Fatalf("expected reset, got %v", lastLevel.String)
	}
}

// Requires TEST_DATABASE_URL pointing at a disposable database.
func TestNotifierSkipsStaleWeather(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", dsn)
	db := database.Connect()
	defer db.Close()
	database.Migrate(db)

	var accountID string
	if err := db.QueryRow(`INSERT INTO accounts DEFAULT VALUES RETURNING id`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM accounts WHERE id = $1`, accountID)
	if _, err := db.Exec(`INSERT INTO devices (account_id, platform, push_token, push_consented_at) VALUES ($1, 'ios', 'ExponentPushToken[stale-test]', NOW())`, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO location_subscriptions (account_id, name, lat, lon, min_level) VALUES ($1, 'Stale', 3.5, 4.5, 'Advisory')`, accountID); err != nil {
		t.Fatal(err)
	}

	push := &fakePush{tickets: func(services.PushMessage) services.PushTicket { return services.PushTicket{Status: "ok"} }}
	n := &Notifier{
		DB:   db,
		Push: push,
		Assess: func(_ context.Context, lat, lon float64, name string) (*models.RiskAssessment, error) {
			return &models.RiskAssessment{Level: "Emergency", Score: 95, Stale: true}, nil
		},
	}
	if err := n.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(push.sent) != 0 {
		t.Fatalf("sent %d alerts from stale weather, want none", len(push.sent))
	}
	var level sql.NullString
	db.QueryRow(`SELECT last_notified_level FROM location_subscriptions WHERE account_id = $1`, accountID).Scan(&level)
	if level.Valid {
		t.Fatalf("subscription marked notified (%s) from stale data", level.String)
	}
}
