package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"testing"

	"aurarisk-backend/internal/models"
)

// GET /api/reports uses a true radius. At 60°N the old box (which assumed
// ~25°) reached 1.9× too far east and west, and its corners went further.
func TestReportsRadiusIsTrueDistance(t *testing.T) {
	r := curationRouter(t)
	lat, lon := 59+rand.Float64()*2, -170+rand.Float64()*340
	tag := rand.Int()
	metersPerDegLon := 111_320 * math.Cos(lat*math.Pi/180)

	ids := map[string]string{}
	for name, meters := range map[string]float64{"near": 900, "far": 1100, "corner": 0} {
		dLat, dLon := 0.0, meters/metersPerDegLon
		if name == "corner" {
			// 1.27 km away diagonally: inside a 1 km box, outside the radius.
			dLat, dLon = 900/111_320.0, 900/metersPerDegLon
		}
		w := call(t, r, http.MethodPost, "/api/reports", "", map[string]any{
			"location": map[string]any{"name": "Test", "lat": lat + dLat, "lon": lon + dLon},
			"category": "flooding", "note": fmt.Sprintf("%s report %d", name, tag),
		})
		var rep models.CommunityReport
		json.Unmarshal(w.Body.Bytes(), &rep)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, w.Code, w.Body)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM community_reports WHERE id = $1`, rep.ID) })
		ids[rep.ID] = name
	}

	w := call(t, r, http.MethodGet, fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=1", lat, lon), "", nil)
	var reps []models.CommunityReport
	json.Unmarshal(w.Body.Bytes(), &reps)
	got := map[string]bool{}
	for _, rep := range reps {
		if name, ok := ids[rep.ID]; ok {
			got[name] = true
		}
	}
	if w.Code != http.StatusOK || !got["near"] || got["far"] || got["corner"] {
		t.Fatalf("radius=1 at lat %.1f: %d, listed %v; want only near", lat, w.Code, got)
	}

	for _, q := range []string{"radius=0", "radius=-1", "radius=201", "radius=NaN", "radius=x"} {
		w := call(t, r, http.MethodGet, fmt.Sprintf("/api/reports?lat=%f&lon=%f&%s", lat, lon, q), "", nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, w.Code)
		}
	}
	for _, q := range []string{"lat=91&lon=0", "lat=0&lon=-181"} {
		w := call(t, r, http.MethodGet, "/api/reports?"+q, "", nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, w.Code)
		}
	}
}

// The public listing and duplicate lookup use the GiST index rather than a
// sequential scan.
func TestReportsQueryUsesSpatialIndex(t *testing.T) {
	curationRouter(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(`EXPLAIN SELECT id FROM community_reports
		WHERE moderation_status = ANY('{pending,verified}') AND duplicate_of IS NULL
		AND ST_DWithin(geog, ST_SetSRID(ST_MakePoint(34.75, -0.1), 4326)::geography, 10000)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	plan := ""
	for rows.Next() {
		var line string
		rows.Scan(&line)
		plan += line + "\n"
	}
	if !strings.Contains(plan, "_geog") {
		t.Fatalf("plan does not use the spatial index:\n%s", plan)
	}
}
