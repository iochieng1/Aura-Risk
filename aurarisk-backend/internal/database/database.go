package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func Connect() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5432/aurarisk?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}

	var pingErr error
	for i := 0; i < 20; i++ {
		pingErr = db.Ping()
		if pingErr == nil {
			break
		}
		log.Printf("Waiting for database to become ready (%d/20): %v", i+1, pingErr)
		time.Sleep(1 * time.Second)
	}
	if pingErr != nil {
		log.Fatalf("Failed to ping database after retries: %v", pingErr)
	}

	log.Println("✅ Database connected")
	return db
}

func Migrate(db *sql.DB) {
	migration := `
	CREATE EXTENSION IF NOT EXISTS pgcrypto;

	CREATE TABLE IF NOT EXISTS community_reports (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		location_name TEXT NOT NULL,
		lat DECIMAL(10, 6) NOT NULL,
		lon DECIMAL(10, 6) NOT NULL,
		category TEXT NOT NULL,
		note TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_reports_location ON community_reports(lat, lon);

	CREATE TABLE IF NOT EXISTS accounts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS devices (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		platform TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
		app_version TEXT,
		push_token TEXT UNIQUE,
		push_consented_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_devices_account ON devices(account_id);

	CREATE TABLE IF NOT EXISTS auth_tokens (
		token_hash BYTEA PRIMARY KEY,
		device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
		kind TEXT NOT NULL CHECK (kind IN ('access', 'refresh')),
		expires_at TIMESTAMPTZ NOT NULL,
		revoked_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_auth_tokens_device ON auth_tokens(device_id);

	CREATE TABLE IF NOT EXISTS location_subscriptions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		lat DOUBLE PRECISION NOT NULL CHECK (lat BETWEEN -90 AND 90),
		lon DOUBLE PRECISION NOT NULL CHECK (lon BETWEEN -180 AND 180),
		min_level TEXT NOT NULL CHECK (min_level IN ('Advisory', 'Alert', 'Emergency')),
		quiet_start TEXT,
		quiet_end TEXT,
		quiet_timezone TEXT,
		allow_emergency_during_quiet BOOLEAN NOT NULL DEFAULT FALSE,
		last_notified_level TEXT,
		last_notified_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_subscriptions_account ON location_subscriptions(account_id);

	ALTER TABLE community_reports ADD COLUMN IF NOT EXISTS account_id UUID REFERENCES accounts(id) ON DELETE SET NULL;

	CREATE TABLE IF NOT EXISTS report_photos (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		report_id UUID NOT NULL REFERENCES community_reports(id) ON DELETE CASCADE,
		account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
		status TEXT NOT NULL CHECK (status IN ('pending_upload', 'processing', 'ready', 'rejected', 'failed')),
		content_type TEXT NOT NULL,
		size_bytes BIGINT NOT NULL,
		quarantine_key TEXT NOT NULL,
		large_key TEXT,
		thumb_key TEXT,
		attempts INT NOT NULL DEFAULT 0,
		locked_until TIMESTAMPTZ,
		rejection_reason TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_report_photos_report ON report_photos(report_id);
	CREATE INDEX IF NOT EXISTS idx_report_photos_processing ON report_photos(updated_at) WHERE status = 'processing';
`

	_, err := db.Exec(migration)
	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("✅ Migrations complete")
}

func HealthCheck() error {
	if _, err := os.Stat(".env"); err == nil {
		return nil
	}
	return fmt.Errorf("database not initialized")
}
