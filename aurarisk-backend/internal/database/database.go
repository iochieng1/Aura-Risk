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
