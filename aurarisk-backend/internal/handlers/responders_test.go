package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"testing"

	"aurarisk-backend/config"
	"aurarisk-backend/internal/models"
)

func TestSetRespondersRejectsUnknownCounty(t *testing.T) {
	if err := SetResponders(map[string]config.Responder{"x": {County: "Nyanza", Token: "t"}}); err == nil {
		t.Fatal("expected error")
	}
	t.Cleanup(func() { SetResponders(nil) })
}

func TestResponderFlow(t *testing.T) {
	r := curationRouter(t)
	if err := SetResponders(map[string]config.Responder{
		"siaya-desk": {County: "siaya county", Token: "siaya-token"},
		"busia-desk": {County: "Busia", Token: "busia-token"},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetResponders(nil) })
	rg := r.Group("/api/responder", RequireResponder())
	rg.GET("/reports", ListResponderReports)
	rg.POST("/reports/:id", UpdateReportResponse)
	rg.GET("/reports/:id/events", ListResponseEvents)

	lat, lon := -60+rand.Float64()*120, -170+rand.Float64()*340
	insert := func(status, county string) string {
		t.Helper()
		var id string
		if err := db.QueryRow(`
			INSERT INTO community_reports (location_name, lat, lon, category, note, moderation_status, county)
			VALUES ('Test', $1, $2, 'flooding', $3, $4, $5) RETURNING id
		`, lat, lon, fmt.Sprintf("responder test %d", rand.Int()), status, county).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM community_reports WHERE id = $1`, id) })
		return id
	}
	report := insert("verified", "Siaya")
	pending := insert("pending", "Siaya")

	listed := func(token, status, id string) bool {
		t.Helper()
		w := call(t, r, http.MethodGet, "/api/responder/reports?status="+status, token, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("list %s: %d %s", status, w.Code, w.Body)
		}
		var reps []models.CommunityReport
		json.Unmarshal(w.Body.Bytes(), &reps)
		for _, rep := range reps {
			if rep.ID == id {
				return true
			}
		}
		return false
	}
	if !listed("siaya-token", "open", report) || listed("siaya-token", "open", pending) || listed("busia-token", "open", report) {
		t.Fatal("open list: want only the verified report, only for its county")
	}

	update := func(token, id, status string) int {
		t.Helper()
		return call(t, r, http.MethodPost, "/api/responder/reports/"+id, token,
			map[string]string{"status": status, "note": "Team dispatched"}).Code
	}
	for name, got := range map[string]int{
		"other county":   update("busia-token", report, "acknowledged"),
		"unverified":     update("siaya-token", pending, "acknowledged"),
		"bad status":     update("siaya-token", report, "open"),
		"no token":       update("", report, "acknowledged"),
		"wrong token":    update("alice-token", report, "acknowledged"),
		"malformed uuid": update("siaya-token", "x", "acknowledged"),
	} {
		want := map[string]int{"other county": 404, "unverified": 404, "bad status": 400, "no token": 401, "wrong token": 401, "malformed uuid": 404}[name]
		if got != want {
			t.Errorf("%s: %d, want %d", name, got, want)
		}
	}

	if code := update("siaya-token", report, "acknowledged"); code != http.StatusOK {
		t.Fatalf("acknowledge: %d", code)
	}
	if code := update("siaya-token", report, "acknowledged"); code != http.StatusConflict {
		t.Fatalf("acknowledge twice: %d", code)
	}
	if code := update("siaya-token", report, "resolved"); code != http.StatusOK {
		t.Fatalf("resolve: %d", code)
	}
	if code := update("siaya-token", report, "acknowledged"); code != http.StatusConflict {
		t.Fatalf("move back: %d", code)
	}
	if !listed("siaya-token", "resolved", report) || listed("siaya-token", "open", report) {
		t.Fatal("resolved list")
	}

	w := call(t, r, http.MethodGet, "/api/responder/reports/"+report+"/events", "siaya-token", nil)
	var events []models.ResponseEvent
	json.Unmarshal(w.Body.Bytes(), &events)
	if w.Code != http.StatusOK || len(events) != 2 || events[0].ToStatus != "acknowledged" ||
		events[1].FromStatus != "acknowledged" || events[1].Actor != "siaya-desk" || *events[1].Note != "Team dispatched" {
		t.Fatalf("events: %d %s", w.Code, w.Body)
	}
	if w := call(t, r, http.MethodGet, "/api/responder/reports/"+report+"/events", "busia-token", nil); w.Code != http.StatusNotFound {
		t.Fatalf("other county's events: %d", w.Code)
	}

	// The public listing shows the response status and when it changed.
	w = call(t, r, http.MethodGet, fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=1", lat, lon), "", nil)
	var reps []models.CommunityReport
	json.Unmarshal(w.Body.Bytes(), &reps)
	found := false
	for _, rep := range reps {
		if rep.ID == report {
			found = rep.ResponseStatus == "resolved" && rep.ResponseUpdatedAt != nil
		}
		if rep.ID == pending && (rep.ResponseStatus != "open" || rep.ResponseUpdatedAt != nil) {
			t.Fatalf("pending report: %+v", rep)
		}
	}
	if !found {
		t.Fatalf("public listing lacks the response status: %s", w.Body)
	}
	var acknowledged, resolved bool
	db.QueryRow(`SELECT acknowledged_at IS NOT NULL, resolved_at IS NOT NULL FROM community_reports WHERE id = $1`, report).Scan(&acknowledged, &resolved)
	if !acknowledged || !resolved {
		t.Fatal("timestamps not recorded")
	}
}
