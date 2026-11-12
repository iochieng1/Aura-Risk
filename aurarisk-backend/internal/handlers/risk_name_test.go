package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"aurarisk-backend/internal/models"
	"aurarisk-backend/internal/services"
)

// Place names and coordinate checks for /api/risk (#85).

// fakeWeather serves weatherJSON for any point, so GET /api/risk never
// reaches Open-Meteo.
type fakeWeather struct{}

func (fakeWeather) Fetch(_ context.Context, lat, lon float64) (*services.OpenMeteoResponse, error) {
	var r services.OpenMeteoResponse
	err := json.Unmarshal([]byte(weatherJSON(lat, lon)), &r)
	return &r, err
}

type fakeGeocoder struct {
	mu    sync.Mutex
	calls int
	name  string
	err   error
}

func (f *fakeGeocoder) ReverseGeocode(context.Context, float64, float64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.name, f.err
}

func (f *fakeGeocoder) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// withRiskServices points risk lookups at fakes for one test.
func withRiskServices(t *testing.T, g services.ReverseGeocoder) {
	t.Helper()
	services.SetWeatherService(services.NewWeatherService(fakeWeather{}, services.WeatherConfig{}))
	if g != nil {
		services.SetGeocodeService(services.NewGeocodeService(g, services.GeocodeConfig{}))
	}
	t.Cleanup(func() {
		services.SetWeatherService(services.NewWeatherService(&services.OpenMeteoClient{}, services.WeatherConfig{}))
		services.SetGeocodeService(nil)
	})
}

type riskBody struct {
	Location struct{ Name string }
	Summary  string
}

func getRisk(t *testing.T, query string) (*httptest.ResponseRecorder, riskBody) {
	t.Helper()
	r := newRouter()
	r.GET("/api/risk", GetRisk)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/risk?"+query, nil))
	var body riskBody
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return w, body
}

func TestGetRiskRejectsOutOfRangeCoordinates(t *testing.T) {
	withRiskServices(t, nil)
	for _, q := range []string{
		"lat=90.0001&lon=0", "lat=-90.0001&lon=0", "lat=91&lon=34.77", "lat=-1000&lon=34.77",
		"lat=0&lon=180.0001", "lat=0&lon=-180.0001", "lat=0&lon=181", "lat=0&lon=-360",
		"lat=NaN&lon=0", "lat=0&lon=NaN", "lat=Inf&lon=0", "lat=0&lon=-Inf",
	} {
		w, _ := getRisk(t, q)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, body %s", q, w.Code, w.Body)
			continue
		}
		if e := decodeError(t, w); e.Code != "bad_request" || !strings.Contains(e.Error, "must be between") {
			t.Errorf("%s: error %+v", q, e)
		}
	}
	// The bounds themselves are valid.
	for _, q := range []string{"lat=90&lon=180", "lat=-90&lon=-180"} {
		if w, _ := getRisk(t, q); w.Code != http.StatusOK {
			t.Errorf("%s: status %d, body %s", q, w.Code, w.Body)
		}
	}
}

func TestGetRiskUsesClientName(t *testing.T) {
	g := &fakeGeocoder{name: "Somewhere Else"}
	withRiskServices(t, g)

	w, body := getRisk(t, "lat=-1.31&lon=36.79&name="+url.QueryEscape("  Kibera, Nairobi "))
	if w.Code != http.StatusOK || body.Location.Name != "Kibera, Nairobi" ||
		!strings.HasPrefix(body.Summary, "Flood risk for Kibera, Nairobi is") {
		t.Fatalf("%d %+v", w.Code, body)
	}
	sww, sw := getRisk(t, "lat=-1.31&lon=36.79&lang=sw&name=Kibera")
	if !strings.HasPrefix(sw.Summary, "Hatari ya mafuriko kwa Kibera kwa sasa") {
		t.Errorf("Swahili summary %q", sw.Summary)
	}
	if g.count() != 0 {
		t.Errorf("geocoded %d times although the client named the place", g.count())
	}
	if strings.Contains(w.Body.String()+sww.Body.String(), "Selected Location") {
		t.Errorf("placeholder in %s %s", w.Body, sww.Body)
	}
	// An empty name counts as none.
	if _, body := getRisk(t, "lat=-1.31&lon=36.79&name=%20"); body.Location.Name != "Somewhere Else" {
		t.Errorf("blank name: %+v", body)
	}
}

func TestGetRiskGeocodesUnnamedPoints(t *testing.T) {
	g := &fakeGeocoder{name: "Nyalenda, Kisumu"}
	withRiskServices(t, g)

	w, body := getRisk(t, "lat=-0.11&lon=34.77")
	if w.Code != http.StatusOK || body.Location.Name != "Nyalenda, Kisumu" ||
		!strings.Contains(body.Summary, "Nyalenda, Kisumu") || strings.Contains(w.Body.String(), "Selected Location") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// Same grid cell: answered from the cache.
	_, sw := getRisk(t, "lat=-0.111&lon=34.771&lang=sw")
	if g.count() != 1 {
		t.Errorf("%d geocoder calls, want 1", g.count())
	}
	if !strings.HasPrefix(sw.Summary, "Hatari ya mafuriko kwa Nyalenda, Kisumu kwa sasa") {
		t.Errorf("Swahili summary %q", sw.Summary)
	}
}

func TestRiskGeocodingFailureFallsBackToCoordinates(t *testing.T) {
	withRiskServices(t, &fakeGeocoder{err: errors.New("nominatim down")})

	w, body := getRisk(t, "lat=-1.2921&lon=36.8219")
	if w.Code != http.StatusOK || body.Location.Name != "-1.2921, 36.8219" ||
		!strings.HasPrefix(body.Summary, "Flood risk for -1.2921, 36.8219 is") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// Device weather, with geocoding disabled.
	services.SetGeocodeService(nil)
	pw := postClientWeather(t, fmt.Sprintf(`{"lat":-0.07,"lon":34.77,"weather":%s}`, weatherJSON(-0.1, 34.8)))
	if pw.Code != http.StatusOK || !strings.Contains(pw.Body.String(), `"name":"-0.0700, 34.7700"`) {
		t.Fatalf("POST: %d %s", pw.Code, pw.Body)
	}
}

func TestPostRiskUsesClientName(t *testing.T) {
	g := &fakeGeocoder{name: "Somewhere Else"}
	withRiskServices(t, g)

	body := fmt.Sprintf(`{"lat":-0.07,"lon":34.77,"name":"Nyalenda","weather":%s}`, weatherJSON(-0.1, 34.8))
	w := postClientWeather(t, body)
	var risk riskBody
	json.Unmarshal(w.Body.Bytes(), &risk)
	if w.Code != http.StatusOK || risk.Location.Name != "Nyalenda" || !strings.HasPrefix(risk.Summary, "Flood risk for Nyalenda is") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	r := newRouter()
	r.POST("/api/risk", PostRiskWithWeather)
	req := httptest.NewRequest(http.MethodPost, "/api/risk?lang=sw", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	sw := httptest.NewRecorder()
	r.ServeHTTP(sw, req)
	json.Unmarshal(sw.Body.Bytes(), &risk)
	if !strings.HasPrefix(risk.Summary, "Hatari ya mafuriko kwa Nyalenda kwa sasa") {
		t.Errorf("Swahili summary %q", risk.Summary)
	}
	if g.count() != 0 || strings.Contains(w.Body.String()+sw.Body.String(), "Selected Location") {
		t.Errorf("geocoded %d times; bodies %s %s", g.count(), w.Body, sw.Body)
	}
}

func TestRiskRejectsInvalidName(t *testing.T) {
	withRiskServices(t, nil)
	for _, name := range []string{strings.Repeat("a", 101), "Kibera\x00", "Kib\nera", "Kibera\u202e", "\x1b[31mred"} {
		w, _ := getRisk(t, "lat=-1.31&lon=36.79&name="+url.QueryEscape(name))
		if w.Code != http.StatusBadRequest || decodeError(t, w).Code != "bad_request" {
			t.Errorf("GET name %q: %d %s", name, w.Code, w.Body)
		}
		quoted, _ := json.Marshal(name)
		pw := postClientWeather(t, fmt.Sprintf(`{"lat":-0.07,"lon":34.77,"name":%s,"weather":%s}`, quoted, weatherJSON(-0.1, 34.8)))
		if pw.Code != http.StatusBadRequest || decodeError(t, pw).Code != "bad_request" {
			t.Errorf("POST name %q: %d %s", name, pw.Code, pw.Body)
		}
	}
	// A non-string name is a malformed body.
	if w := postClientWeather(t, `{"lat":0,"lon":0,"name":7}`); w.Code != http.StatusBadRequest {
		t.Errorf("numeric name: %d", w.Code)
	}
	// 100 characters is fine, counted in characters rather than bytes.
	if w, _ := getRisk(t, "lat=-1.31&lon=36.79&name="+url.QueryEscape(strings.Repeat("é", 100))); w.Code != http.StatusOK {
		t.Errorf("100-character name: %d %s", w.Code, w.Body)
	}
}

// SMS and USSD name the county, in English and Swahili.
func TestCountyRiskMessageNamesCounty(t *testing.T) {
	var asked string
	SetChannels(&ChannelConfig{
		CallbackSecret: "callback-secret",
		Assess: func(_ context.Context, lat, lon float64, name string) (*models.RiskAssessment, error) {
			asked = name
			return &models.RiskAssessment{Location: models.Location{Name: name}, Score: 62, Level: "Alert",
				Tips: []string{"Avoid walking or driving through floodwater."}}, nil
		},
	})
	t.Cleanup(func() { SetChannels(nil) })

	for lang, want := range map[string]string{
		"en": "AuraRisk Homa Bay: Alert flood risk (62/100).",
		"sw": "AuraRisk Homa Bay: hatari ya mafuriko ni Onyo (62/100).",
	} {
		got := countyRiskMessage(context.Background(), lang, "Homa Bay")
		if !strings.HasPrefix(got, want) || strings.Contains(got, "Selected Location") {
			t.Errorf("%s: %q", lang, got)
		}
	}
	if asked != "Homa Bay" {
		t.Errorf("assessed as %q", asked)
	}
}
