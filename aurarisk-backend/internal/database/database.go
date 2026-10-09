package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"aurarisk-backend/migrations"
	_ "github.com/lib/pq"
)

func Connect() *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
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

// migrationLockID is an arbitrary constant identifying the migration lock.
const migrationLockID = 7_311_000

type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads the embedded NNN_name.sql files in version order.
func loadMigrations() ([]migration, error) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var list []migration
	for i, name := range files {
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %s: name must start with a version number", name)
		}
		if version != i+1 {
			return nil, fmt.Errorf("migration %s: expected version %d", name, i+1)
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return nil, err
		}
		list = append(list, migration{version: version, name: name, sql: string(body)})
	}
	return list, nil
}

// Migrate applies pending migrations in order, each in its own transaction
// under an advisory lock, so concurrent runs apply each migration once.
func Migrate(db *sql.DB) error {
	list, err := loadMigrations()
	if err != nil {
		return err
	}
	if err := bootstrap(db); err != nil {
		return fmt.Errorf("prepare schema_migrations: %w", err)
	}
	for _, m := range list {
		applied, err := apply(db, m)
		if err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if applied {
			log.Printf("✅ Applied migration %s", m.name)
		}
	}
	return nil
}

// PendingMigrations lists migrations not yet applied to the database.
func PendingMigrations(db *sql.DB) ([]string, error) {
	list, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	applied := map[int]bool{}
	var hasTable, hasLegacy bool
	if err := db.QueryRow(`SELECT to_regclass('schema_migrations') IS NOT NULL, to_regclass('schema_version') IS NOT NULL`).Scan(&hasTable, &hasLegacy); err != nil {
		return nil, err
	}
	if hasLegacy {
		// Not yet recorded in schema_migrations; see bootstrap.
		var legacy int
		if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&legacy); err != nil {
			return nil, err
		}
		for v := 1; v <= legacy; v++ {
			applied[v] = true
		}
	}
	if hasTable {
		rows, err := db.Query(`SELECT version FROM schema_migrations`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var v int
			if err := rows.Scan(&v); err != nil {
				return nil, err
			}
			applied[v] = true
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	var pending []string
	for _, m := range list {
		if !applied[m.version] {
			pending = append(pending, m.name)
		}
	}
	return pending, nil
}

// bootstrap creates schema_migrations. Databases set up by the old startup
// migration record a single schema_version number instead; versions up to
// it map one-to-one onto the migration files and are marked applied.
func bootstrap(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INT PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return err
	}
	var hasLegacy bool
	if err := tx.QueryRow(`SELECT to_regclass('schema_version') IS NOT NULL`).Scan(&hasLegacy); err != nil {
		return err
	}
	if hasLegacy {
		if _, err := tx.Exec(`
			INSERT INTO schema_migrations (version, name)
			SELECT v, 'legacy schema_version ' || v
			FROM generate_series(1, (SELECT COALESCE(MAX(version), 0) FROM schema_version)) AS v
			ON CONFLICT (version) DO NOTHING
		`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func apply(db *sql.DB, m migration) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return false, err
	}
	var done bool
	if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, m.version).Scan(&done); err != nil {
		return false, err
	}
	if done {
		return false, nil
	}
	if _, err := tx.Exec(m.sql); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func HealthCheck() error {
	if _, err := os.Stat(".env"); err == nil {
		return nil
	}
	return fmt.Errorf("database not initialized")
}
