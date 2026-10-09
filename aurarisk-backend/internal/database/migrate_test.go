package database

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"slices"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	list, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no migrations embedded")
	}
	for i, m := range list {
		if m.version != i+1 || m.sql == "" {
			t.Errorf("migration %d: got version %d (%s), empty=%v", i, m.version, m.name, m.sql == "")
		}
	}
}

// Requires TEST_DATABASE_URL pointing at a disposable server where the user
// may create databases. The test uses its own database so packages running
// in parallel against TEST_DATABASE_URL do not interfere.
func TestMigrateFromLegacySchemaVersion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("migrate_test_%d", rand.Int64())
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`) })

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Recreate a database set up by the old startup migration at version 3.
	list, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list[:3] {
		if _, err := db.Exec(m.sql); err != nil {
			t.Fatalf("%s: %v", m.name, err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INT NOT NULL); INSERT INTO schema_version VALUES (3)`); err != nil {
		t.Fatal(err)
	}

	pending, err := PendingMigrations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != len(list)-3 {
		t.Fatalf("before migrate: got pending %v, want %d", pending, len(list)-3)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if pending, err := PendingMigrations(db); err != nil || len(pending) != 0 {
		t.Fatalf("pending after migrate: %v, %v", pending, err)
	}

	rows, err := db.Query(`SELECT name FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	want := []string{"legacy schema_version 1", "legacy schema_version 2", "legacy schema_version 3"}
	for _, m := range list[3:] {
		want = append(want, m.name)
	}
	if !slices.Equal(names, want) {
		t.Errorf("schema_migrations = %v, want %v", names, want)
	}

	// Running again is a no-op.
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
}
