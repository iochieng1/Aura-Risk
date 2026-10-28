package handlers

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Uses Mandera in March 2001, which no other test touches.
func TestCountyStatsAndOpenData(t *testing.T) {
	r := curationRouter(t)
	r.GET("/api/stats/counties", GetCountyStats)
	r.GET("/api/open-data/reports.csv", GetOpenDataReports("csv"))
	r.GET("/api/open-data/reports.json", GetOpenDataReports("json"))

	db.Exec(`DELETE FROM community_reports WHERE county = 'Mandera' AND created_at < '2002-01-01'`)
	insert := func(status, response, ward string, ackHours, resolveHours float64) {
		t.Helper()
		var id string
		err := db.QueryRow(`
			INSERT INTO community_reports (location_name, lat, lon, category, note, moderation_status, county, ward,
				created_at, response_status, acknowledged_at, resolved_at)
			VALUES ('Secret place', 3.937512, 41.856749, 'flooding', 'private note', $1, 'Mandera', NULLIF($3, ''),
				'2001-03-10 12:00', $2,
				CASE WHEN $4 > 0 THEN '2001-03-10 12:00'::timestamp::timestamptz + make_interval(secs => $4 * 3600) END,
				CASE WHEN $5 > 0 THEN '2001-03-10 12:00'::timestamp::timestamptz + make_interval(secs => $5 * 3600) END)
			RETURNING id
		`, status, response, ward, ackHours, resolveHours).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM community_reports WHERE id = $1`, id) })
	}
	insert("verified", "open", "=HYPERLINK(\"x\")", 0, 0)
	insert("verified", "acknowledged", "", 2, 0)
	insert("verified", "resolved", "", 4, 10)
	insert("verified", "resolved", "", 6, 20)
	insert("pending", "open", "", 0, 0)
	insert("rejected", "open", "", 0, 0)

	w := call(t, r, http.MethodGet, "/api/stats/counties?from=2001-03-01&to=2001-03-31", "", nil)
	var stats struct {
		From, To string
		Counties []CountyStats
	}
	json.Unmarshal(w.Body.Bytes(), &stats)
	if w.Code != http.StatusOK || len(stats.Counties) != 1 {
		t.Fatalf("stats: %d %s", w.Code, w.Body)
	}
	s := stats.Counties[0]
	if s.County != "Mandera" || s.Verified != 4 || s.Open != 1 || s.Acknowledged != 1 || s.Resolved != 2 ||
		*s.MedianHoursToAcknowledge != 4 || *s.MedianHoursToResolve != 15 {
		t.Fatalf("Mandera: %+v (ack %v, resolve %v)", s, *s.MedianHoursToAcknowledge, *s.MedianHoursToResolve)
	}

	for q, want := range map[string]int{
		"from=2001-03-31&to=2001-03-01": 400,
		"from=2000-01-01&to=2001-03-01": 400,
		"from=yesterday":                400,
		"":                              200,
	} {
		if w := call(t, r, http.MethodGet, "/api/stats/counties?"+q, "", nil); w.Code != want {
			t.Errorf("%q: %d, want %d", q, w.Code, want)
		}
	}

	w = call(t, r, http.MethodGet, "/api/open-data/reports.csv?from=2001-03-01&to=2001-03-31&county=mandera", "", nil)
	records, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil || w.Code != http.StatusOK || len(records) != 5 {
		t.Fatalf("csv: %d %v\n%s", w.Code, err, w.Body)
	}
	if strings.Join(records[0], ",") != strings.Join(openDataColumns, ",") {
		t.Fatalf("header %v", records[0])
	}
	if row := records[1]; row[4] != "3.94" || row[5] != "41.86" || row[3] != `'=HYPERLINK("x")` || row[0] == "" {
		t.Fatalf("row %v", row)
	}
	body := w.Body.String()
	if strings.Contains(body, "private note") || strings.Contains(body, "Secret place") {
		t.Fatal("export leaks the note or place name")
	}
	if !strings.Contains(w.Header().Get("Link"), "creativecommons.org/licenses/by/4.0") {
		t.Fatalf("license header %q", w.Header().Get("Link"))
	}

	w = call(t, r, http.MethodGet, "/api/open-data/reports.json?from=2001-03-01&to=2001-03-31&county=Mandera", "", nil)
	var export struct {
		License string
		Reports []OpenDataReport
	}
	json.Unmarshal(w.Body.Bytes(), &export)
	if w.Code != http.StatusOK || export.License != "CC BY 4.0" || len(export.Reports) != 4 || export.Reports[0].Lat != 3.94 {
		t.Fatalf("json: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "private note") {
		t.Fatal("json export leaks the note")
	}
	if w := call(t, r, http.MethodGet, "/api/open-data/reports.json?county=Atlantis", "", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown county: %d", w.Code)
	}
}
