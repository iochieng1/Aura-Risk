package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Only this test uses Lamu and Kwale, so other tests' reports can't be sent.
func TestCountyWebhookDelivery(t *testing.T) {
	db := curationDB(t)
	lat, lon := testSite()

	var mu sync.Mutex
	var bodies [][]byte
	var headers []http.Header
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		bodies, headers = append(bodies, body), append(headers, r.Header.Clone())
		if fail {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	calls := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(bodies)
	}

	setCounty := func(id, county string) {
		if _, err := db.Exec(`UPDATE community_reports SET county = $2 WHERE id = $1`, id, county); err != nil {
			t.Fatal(err)
		}
	}
	verified := insertReport(t, db, testReport{lat: lat, lon: lon, note: "Verified", status: "verified", by: "alice"})
	setCounty(verified, "Lamu")
	pending := insertReport(t, db, testReport{lat: lat, lon: lon, note: "Pending"})
	setCounty(pending, "Lamu")
	old := insertReport(t, db, testReport{lat: lat, lon: lon, note: "Old", status: "verified", by: "alice", age: 48 * time.Hour})
	setCounty(old, "Lamu")
	elsewhere := insertReport(t, db, testReport{lat: lat, lon: lon, note: "No webhook", status: "verified", by: "alice"})
	setCounty(elsewhere, "Kwale")

	w := &CountyWebhooks{DB: db, URLs: map[string]string{"Lamu": srv.URL}, Secret: "test-secret"}
	ctx := context.Background()

	// First attempt fails and is rescheduled.
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var delivered sql.NullTime
	var lastError sql.NullString
	state := func() {
		t.Helper()
		if err := db.QueryRow(`SELECT attempts, delivered_at, last_error FROM report_webhook_deliveries WHERE report_id = $1`,
			verified).Scan(&attempts, &delivered, &lastError); err != nil {
			t.Fatal(err)
		}
	}
	state()
	if attempts != 1 || delivered.Valid || lastError.String != "status 502" || calls() != 1 {
		t.Fatalf("after failure: attempts=%d delivered=%v error=%q calls=%d", attempts, delivered.Valid, lastError.String, calls())
	}

	// Not retried before its backoff.
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls() != 1 {
		t.Fatalf("retried before backoff: %d calls", calls())
	}

	db.Exec(`UPDATE report_webhook_deliveries SET next_attempt_at = NOW() WHERE report_id = $1`, verified)
	mu.Lock()
	fail = false
	mu.Unlock()
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	state()
	if attempts != 2 || !delivered.Valid || calls() != 2 {
		t.Fatalf("after success: attempts=%d delivered=%v calls=%d", attempts, delivered.Valid, calls())
	}

	// Delivered once: later runs send nothing.
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls() != 2 {
		t.Fatalf("redelivered: %d calls", calls())
	}

	mu.Lock()
	defer mu.Unlock()
	var payload WebhookPayload
	if err := json.Unmarshal(bodies[1], &payload); err != nil {
		t.Fatal(err)
	}
	h := headers[1]
	if payload.Event != "report.verified" || payload.Report.ID != verified || *payload.Report.County != "Lamu" ||
		payload.Report.ResponseStatus != "open" || h.Get("X-AuraRisk-Delivery") != verified {
		t.Fatalf("payload %+v headers %v", payload, h)
	}
	if want := "sha256=" + SignWebhook("test-secret", h.Get("X-AuraRisk-Timestamp"), bodies[1]); h.Get("X-AuraRisk-Signature") != want {
		t.Fatalf("signature %q, want %q", h.Get("X-AuraRisk-Signature"), want)
	}

	var queued int
	db.QueryRow(`SELECT COUNT(*) FROM report_webhook_deliveries WHERE report_id IN ($1, $2, $3)`, pending, old, elsewhere).Scan(&queued)
	if queued != 0 {
		t.Fatalf("%d pending, old or unconfigured-county reports were queued", queued)
	}
}
