package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"aurarisk-backend/internal/database"
	"aurarisk-backend/internal/models"
	"github.com/gin-gonic/gin"
)

// Requires TEST_DATABASE_URL pointing at a disposable database.
func curationRouter(t *testing.T) *gin.Engine {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", dsn)
	conn := database.Connect()
	t.Cleanup(func() { conn.Close() })
	if err := database.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	SetDatabase(conn)
	SetModerators(map[string]string{"alice": "alice-token"})
	t.Cleanup(func() { SetModerators(nil) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/reports", GetReports)
	r.POST("/api/reports", OptionalAuth(), CreateReport)
	m := r.Group("/api/moderation", RequireModerator())
	m.GET("/reports", ListModerationQueue)
	m.POST("/reports/:id", ModerateReport)
	m.GET("/reports/:id/events", ListModerationEvents)
	return r
}

func call(t *testing.T, r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestReportCurationFlow(t *testing.T) {
	r := curationRouter(t)
	lat, lon := -60+rand.Float64()*120, -170+rand.Float64()*340
	note := fmt.Sprintf("Water is over the footbridge near the clinic %d", rand.Int())

	newReport := func(note string, dLon float64) models.CommunityReport {
		t.Helper()
		w := call(t, r, http.MethodPost, "/api/reports", "", map[string]any{
			"location": map[string]any{"name": "Test", "lat": lat, "lon": lon + dLon},
			"category": "flooding", "note": note,
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body)
		}
		var rep models.CommunityReport
		json.Unmarshal(w.Body.Bytes(), &rep)
		t.Cleanup(func() { db.Exec(`DELETE FROM community_reports WHERE id = $1`, rep.ID) })
		return rep
	}

	original := newReport(note, 0)
	if original.Status != "pending" || original.DuplicateOf != nil {
		t.Fatalf("original: %+v", original)
	}
	copied := newReport(note+"!!", 0.001) // ~100 m away, same words
	if copied.DuplicateOf == nil || *copied.DuplicateOf != original.ID {
		t.Fatalf("copy not detected as duplicate: %+v", copied)
	}
	independent := newReport("Market road under knee-deep water", 0.002)
	if independent.DuplicateOf != nil {
		t.Fatalf("independent report flagged as duplicate")
	}

	listed := func(query string) map[string]models.CommunityReport {
		t.Helper()
		w := call(t, r, http.MethodGet, fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=2%s", lat, lon, query), "", nil)
		var reps []models.CommunityReport
		json.Unmarshal(w.Body.Bytes(), &reps)
		out := map[string]models.CommunityReport{}
		for _, rep := range reps {
			out[rep.ID] = rep
		}
		return out
	}
	public := listed("")
	if _, ok := public[copied.ID]; ok {
		t.Fatal("duplicate is publicly listed")
	}
	if rep, ok := public[original.ID]; !ok || rep.Status != "pending" {
		t.Fatalf("pending original should be listed with its status: %+v", rep)
	}

	// Moderation API authentication.
	if w := call(t, r, http.MethodGet, "/api/moderation/reports", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call(t, r, http.MethodGet, "/api/moderation/reports", "wrong", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", w.Code)
	}

	// Rejecting needs a reason.
	path := "/api/moderation/reports/" + independent.ID
	if w := call(t, r, http.MethodPost, path, "alice-token", map[string]any{"status": "rejected"}); w.Code != http.StatusBadRequest {
		t.Fatalf("reject without reason: %d", w.Code)
	}
	w := call(t, r, http.MethodPost, path, "alice-token", map[string]any{"status": "rejected", "reason": "not a real location"})
	var decided models.ModerationReport
	json.Unmarshal(w.Body.Bytes(), &decided)
	if w.Code != http.StatusOK || decided.Status != "rejected" || decided.ModeratedBy == nil || *decided.ModeratedBy != "alice" {
		t.Fatalf("reject: %d %s", w.Code, w.Body)
	}

	// Verify the original and clear the wrong duplicate flag on the copy.
	call(t, r, http.MethodPost, "/api/moderation/reports/"+original.ID, "alice-token", map[string]any{"status": "verified"})
	call(t, r, http.MethodPost, "/api/moderation/reports/"+copied.ID, "alice-token", map[string]any{"status": "pending", "not_duplicate": true})

	verifiedOnly := listed("&verified_only=true")
	if len(verifiedOnly) != 1 || verifiedOnly[original.ID].Status != "verified" {
		t.Fatalf("verified_only listing: %+v", verifiedOnly)
	}
	public = listed("")
	if _, ok := public[independent.ID]; ok {
		t.Fatal("rejected report is publicly listed")
	}
	if _, ok := public[copied.ID]; !ok {
		t.Fatal("report cleared as not-duplicate should be listed")
	}

	// Queue shows rejected reports with the decision, and the audit trail.
	w = call(t, r, http.MethodGet, "/api/moderation/reports?status=rejected&limit=100", "alice-token", nil)
	var queue []models.ModerationReport
	json.Unmarshal(w.Body.Bytes(), &queue)
	found := false
	for _, q := range queue {
		if q.ID == independent.ID {
			found = q.ModerationReason != nil && *q.ModerationReason == "not a real location"
		}
	}
	if !found {
		t.Fatalf("rejected report missing from queue: %s", w.Body)
	}

	w = call(t, r, http.MethodGet, "/api/moderation/reports/"+copied.ID+"/events", "alice-token", nil)
	var events []models.ModerationEvent
	json.Unmarshal(w.Body.Bytes(), &events)
	if len(events) != 1 || events[0].Actor != "alice" || events[0].Reason == nil {
		t.Fatalf("events: %s", w.Body)
	}

	if w := call(t, r, http.MethodPost, "/api/moderation/reports/not-a-uuid", "alice-token", map[string]any{"status": "verified"}); w.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestModerationDisabledWithoutTokens(t *testing.T) {
	r := curationRouter(t)
	SetModerators(nil)
	if w := call(t, r, http.MethodGet, "/api/moderation/reports", "anything", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", w.Code)
	}
}
