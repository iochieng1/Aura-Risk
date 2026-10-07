package workers

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"os"
	"testing"
	"time"

	"aurarisk-backend/internal/database"
	"aurarisk-backend/internal/storage"
)

// curationDB connects to TEST_DATABASE_URL and migrates it, or skips.
func curationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", dsn)
	db := database.Connect()
	t.Cleanup(func() { db.Close() })
	database.Migrate(db)
	return db
}

// testSite returns coordinates no other test uses, so tests sharing a
// database cannot corroborate each other's reports.
func testSite() (float64, float64) {
	return -60 + rand.Float64()*120, -170 + rand.Float64()*340
}

type testReport struct {
	account  string
	category string
	note     string
	lat, lon float64
	age      time.Duration
	status   string
	by       string
	dupOf    string
}

func insertReport(t *testing.T, db *sql.DB, r testReport) string {
	t.Helper()
	if r.category == "" {
		r.category = "flooding"
	}
	if r.status == "" {
		r.status = "pending"
	}
	var id string
	err := db.QueryRow(`
		INSERT INTO community_reports (location_name, lat, lon, category, note, account_id,
			moderation_status, moderated_by, duplicate_of, created_at, status_updated_at)
		VALUES ('Test', $1, $2, $3, $4, NULLIF($5, '')::uuid, $6, NULLIF($7, ''), NULLIF($8, '')::uuid,
			NOW() - make_interval(secs => $9), NOW() - make_interval(secs => $9))
		RETURNING id
	`, r.lat, r.lon, r.category, r.note+" "+t.Name(), r.account, r.status, r.by, r.dupOf, r.age.Seconds()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM community_reports WHERE id = $1`, id) })
	return id
}

func newAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`INSERT INTO accounts DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM accounts WHERE id = $1`, id) })
	return id
}

func reportState(t *testing.T, db *sql.DB, id string) (status string, by sql.NullString, score, corroborations int) {
	t.Helper()
	err := db.QueryRow(`
		SELECT moderation_status, moderated_by, verification_score, corroboration_count
		FROM community_reports WHERE id = $1
	`, id).Scan(&status, &by, &score, &corroborations)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestReportVerifierRunOnce(t *testing.T) {
	db := curationDB(t)
	lat, lon := testSite()
	alice, bob, carol := newAccount(t, db), newAccount(t, db), newAccount(t, db)

	// Three different people report water within ~200 m and an hour.
	first := insertReport(t, db, testReport{account: alice, note: "street flooded", lat: lat, lon: lon, age: time.Hour})
	insertReport(t, db, testReport{account: bob, category: "water_rising", note: "river over the bank", lat: lat + 0.001, lon: lon, age: 30 * time.Minute})
	insertReport(t, db, testReport{account: carol, note: "houses taking water", lat: lat, lon: lon + 0.001, age: 10 * time.Minute})
	// Unrelated category and far away reports must not corroborate.
	lonely := insertReport(t, db, testReport{account: alice, category: "road_blocked", note: "tree down", lat: lat, lon: lon, age: time.Hour})
	insertReport(t, db, testReport{account: bob, category: "road_blocked", note: "far tree", lat: lat + 0.05, lon: lon, age: time.Hour})
	// A moderator's decision is final.
	decided := insertReport(t, db, testReport{account: bob, note: "same flood, rejected", lat: lat, lon: lon, age: time.Hour, status: "rejected", by: "alice-mod"})

	if err := (&ReportVerifier{DB: db}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	status, by, score, corroborations := reportState(t, db, first)
	if status != "verified" || by.String != "auto" || score < 50 || corroborations != 2 {
		t.Fatalf("first report: status=%s by=%v score=%d corroborations=%d", status, by, score, corroborations)
	}
	var events int
	db.QueryRow(`SELECT COUNT(*) FROM report_moderation_events WHERE report_id = $1 AND actor = 'auto'`, first).Scan(&events)
	if events != 1 {
		t.Fatalf("auto verification events = %d, want 1", events)
	}

	status, _, score, corroborations = reportState(t, db, lonely)
	if status != "pending" || corroborations != 0 || score != 10 {
		t.Fatalf("uncorroborated report: status=%s score=%d corroborations=%d", status, score, corroborations)
	}

	status, by, _, _ = reportState(t, db, decided)
	if status != "rejected" || by.String != "alice-mod" {
		t.Fatalf("moderated report changed: status=%s by=%v", status, by)
	}
}

func TestReportVerifierUsesModeratorHistoryOnly(t *testing.T) {
	db := curationDB(t)
	lat, lon := testSite()
	reporter := newAccount(t, db)

	// Auto-verified history must not raise reputation; moderator rejections lower it.
	for i := 0; i < 3; i++ {
		insertReport(t, db, testReport{account: reporter, category: "drainage_issue", note: "old", lat: lat + 1, lon: lon, age: 30 * 24 * time.Hour, status: "verified", by: "auto"})
	}
	insertReport(t, db, testReport{account: reporter, category: "drainage_issue", note: "spam", lat: lat + 1, lon: lon, age: 30 * 24 * time.Hour, status: "rejected", by: "mod"})
	id := insertReport(t, db, testReport{account: reporter, category: "drainage_issue", note: "drain blocked", lat: lat, lon: lon})

	if err := (&ReportVerifier{DB: db}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Signed in (+10), one moderator rejection (-25), clamped at 0.
	if _, _, score, _ := reportState(t, db, id); score != 0 {
		t.Fatalf("score = %d, want 0", score)
	}
}

type fakeStore struct {
	storage.ObjectStore
	deleted []string
	fail    map[string]bool
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	if f.fail[key] {
		return errors.New("storage unavailable")
	}
	f.deleted = append(f.deleted, key)
	return nil
}

func addPhoto(t *testing.T, db *sql.DB, reportID, status, key string, age time.Duration) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO report_photos (report_id, status, content_type, size_bytes, quarantine_key, large_key, thumb_key, created_at, updated_at)
		VALUES ($1, $2, 'image/jpeg', 10, $3 || '/q', CASE WHEN $2 = 'ready' THEN $3 || '/l' END, CASE WHEN $2 = 'ready' THEN $3 || '/t' END,
			NOW() - make_interval(secs => $4), NOW() - make_interval(secs => $4))
		RETURNING id
	`, reportID, status, key, age.Seconds()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func exists(t *testing.T, db *sql.DB, table, id string) bool {
	t.Helper()
	var ok bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestRetentionCleanerRunOnce(t *testing.T) {
	db := curationDB(t)
	lat, lon := testSite()
	day := 24 * time.Hour

	oldRejected := insertReport(t, db, testReport{note: "spam", lat: lat, lon: lon, age: 40 * day, status: "rejected", by: "mod"})
	addPhoto(t, db, oldRejected, "ready", "rej", 40*day)
	recentRejected := insertReport(t, db, testReport{note: "spam", lat: lat, lon: lon, age: 5 * day, status: "rejected", by: "mod"})
	oldPending := insertReport(t, db, testReport{note: "old", lat: lat, lon: lon, age: 200 * day})
	// A fresh duplicate of the expired original goes with it, photos included.
	dupOfOld := insertReport(t, db, testReport{note: "old again", lat: lat, lon: lon, age: day, dupOf: oldPending})
	addPhoto(t, db, dupOfOld, "ready", "dup", day)
	keptVerified := insertReport(t, db, testReport{note: "real flood", lat: lat, lon: lon, age: 200 * day, status: "verified", by: "mod"})
	// Object storage failure keeps the row so no object is orphaned.
	stuck := insertReport(t, db, testReport{note: "stuck", lat: lat, lon: lon, age: 40 * day, status: "rejected", by: "mod"})
	addPhoto(t, db, stuck, "ready", "stuck", 40*day)

	live := insertReport(t, db, testReport{note: "live", lat: lat, lon: lon, age: day})
	abandoned := addPhoto(t, db, live, "pending_upload", "abandoned", 2*day)
	inFlight := addPhoto(t, db, live, "pending_upload", "inflight", time.Hour)
	failed := addPhoto(t, db, live, "failed", "failed", 40*day)

	store := &fakeStore{fail: map[string]bool{"stuck/l": true}}
	cleaner := &RetentionCleaner{DB: db, Store: store, Policy: DefaultRetentionPolicy()}
	if err := cleaner.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]struct {
		table, id string
		want      bool
	}{
		"old rejected report":      {"community_reports", oldRejected, false},
		"recent rejected report":   {"community_reports", recentRejected, true},
		"expired pending report":   {"community_reports", oldPending, false},
		"duplicate of expired":     {"community_reports", dupOfOld, false},
		"verified within 2 years":  {"community_reports", keptVerified, true},
		"report blocked on S3":     {"community_reports", stuck, true},
		"live report":              {"community_reports", live, true},
		"abandoned upload":         {"report_photos", abandoned, false},
		"upload still in progress": {"report_photos", inFlight, true},
		"old failed photo":         {"report_photos", failed, false},
	} {
		if got := exists(t, db, c.table, c.id); got != c.want {
			t.Errorf("%s: exists=%v, want %v", name, got, c.want)
		}
	}

	deleted := map[string]bool{}
	for _, k := range store.deleted {
		deleted[k] = true
	}
	for _, k := range []string{"rej/q", "rej/l", "rej/t", "dup/q", "dup/l", "dup/t", "abandoned/q", "failed/q"} {
		if !deleted[k] {
			t.Errorf("object %s was not deleted", k)
		}
	}

	// With every rule disabled nothing is removed.
	keep := &RetentionCleaner{DB: db, Store: store, Policy: RetentionPolicy{}}
	if err := keep.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !exists(t, db, "community_reports", recentRejected) {
		t.Fatal("zero policy deleted data")
	}
}
