package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"

	"aurarisk-backend/internal/models"
)

func TestValidateCountyWard(t *testing.T) {
	county, ward, msg := validateCountyWard(" kisumu county ", "  Nyalenda A ")
	if msg != "" || *county != "Kisumu" || *ward != "Nyalenda A" {
		t.Fatalf("got %v %v %q", county, ward, msg)
	}
	if county, ward, msg := validateCountyWard("", ""); county != nil || ward != nil || msg != "" {
		t.Fatalf("empty: %v %v %q", county, ward, msg)
	}
	for name, in := range map[string][2]string{
		"unknown county":         {"Nyanza", ""},
		"ward requires a county": {"", "Nyalenda A"},
		"ward is too long":       {"Kisumu", strings.Repeat("x", 101)},
	} {
		if _, _, msg := validateCountyWard(in[0], in[1]); msg != name {
			t.Errorf("%v: msg = %q, want %q", in, msg, name)
		}
	}
}

func TestReportsCountyAndWard(t *testing.T) {
	r := curationRouter(t)
	lat, lon := -60+rand.Float64()*120, -170+rand.Float64()*340
	ward := fmt.Sprintf("Test ward %d", rand.Int())

	create := func(body map[string]any) (*httptestResult, models.CommunityReport) {
		t.Helper()
		body["location"] = map[string]any{"name": "Test", "lat": lat, "lon": lon}
		body["category"] = "flooding"
		w := call(t, r, http.MethodPost, "/api/reports", "", body)
		var rep models.CommunityReport
		json.Unmarshal(w.Body.Bytes(), &rep)
		if rep.ID != "" {
			t.Cleanup(func() { db.Exec(`DELETE FROM community_reports WHERE id = $1`, rep.ID) })
		}
		return &httptestResult{w.Code, w.Body.String()}, rep
	}

	res, rep := create(map[string]any{"note": "Water over the road " + ward, "county": "kisumu", "ward": ward})
	if res.code != http.StatusCreated || rep.County == nil || *rep.County != "Kisumu" || *rep.Ward != ward {
		t.Fatalf("create: %d %s", res.code, res.body)
	}
	if res, _ := create(map[string]any{"note": "x", "county": "Atlantis"}); res.code != http.StatusBadRequest {
		t.Fatalf("unknown county: %d", res.code)
	}

	// Listed by location, and by county alone.
	for _, path := range []string{
		fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=1", lat, lon),
		fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=1&county=Kisumu", lat, lon),
		"/api/reports?county=Kisumu%20County",
	} {
		w := call(t, r, http.MethodGet, path, "", nil)
		var reps []models.CommunityReport
		json.Unmarshal(w.Body.Bytes(), &reps)
		found := false
		for _, got := range reps {
			if got.ID == rep.ID {
				found = got.Ward != nil && *got.Ward == ward
			}
		}
		if w.Code != http.StatusOK || !found {
			t.Fatalf("%s: %d, report listed with ward: %v", path, w.Code, found)
		}
	}

	// Another county's filter leaves it out.
	w := call(t, r, http.MethodGet, fmt.Sprintf("/api/reports?lat=%f&lon=%f&radius=1&county=Nairobi", lat, lon), "", nil)
	if strings.Contains(w.Body.String(), rep.ID) {
		t.Fatal("report listed under the wrong county")
	}
	if w := call(t, r, http.MethodGet, "/api/reports?county=Atlantis", "", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown county filter: %d", w.Code)
	}
}

type httptestResult struct {
	code int
	body string
}
