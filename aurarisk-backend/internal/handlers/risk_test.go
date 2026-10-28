package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func postClientWeather(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := newRouter()
	r.POST("/api/risk", PostRiskWithWeather)
	req := httptest.NewRequest(http.MethodPost, "/api/risk", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// weatherJSON is a minimal Open-Meteo response observed now, at UTC.
func weatherJSON(lat, lon float64) string {
	now := time.Now().UTC()
	var times, values []string
	start := now.Truncate(time.Hour).Add(-24 * time.Hour)
	for i := range 48 {
		times = append(times, fmt.Sprintf("%q", start.Add(time.Duration(i)*time.Hour).Format("2006-01-02T15:04")))
		values = append(values, "0.2")
	}
	series := "[" + strings.Join(values, ",") + "]"
	return fmt.Sprintf(`{"latitude":%v,"longitude":%v,"utc_offset_seconds":0,
		"current":{"time":%q,"weather_code":3},
		"hourly":{"time":[%s],"precipitation":%s,"rain":%s,"soil_moisture_0_to_7cm":%s}}`,
		lat, lon, now.Format("2006-01-02T15:04"), strings.Join(times, ","), series, series, series)
}

func TestPostRiskWithWeather(t *testing.T) {
	w := postClientWeather(t, fmt.Sprintf(`{"lat":-0.07,"lon":34.77,"weather":%s}`, weatherJSON(-0.1, 34.8)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	var body struct {
		Score      int
		Level      string
		Confidence struct{ Reasons []string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Level == "" || !strings.Contains(strings.Join(body.Confidence.Reasons, " "), "fetched by your device") {
		t.Errorf("body = %+v", body)
	}
}

func TestRiskLanguage(t *testing.T) {
	body := fmt.Sprintf(`{"lat":-0.07,"lon":34.77,"weather":%s}`, weatherJSON(-0.1, 34.8))
	send := func(path, acceptLanguage string) (*httptest.ResponseRecorder, string, []string) {
		t.Helper()
		r := newRouter()
		r.POST("/api/risk", PostRiskWithWeather)
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if acceptLanguage != "" {
			req.Header.Set("Accept-Language", acceptLanguage)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var risk struct {
			Summary string
			Tips    []string
		}
		json.Unmarshal(w.Body.Bytes(), &risk)
		return w, risk.Summary, risk.Tips
	}

	for _, c := range []struct{ path, accept, want, prefix string }{
		{"/api/risk", "", "en", "Flood risk for"},
		{"/api/risk?lang=sw", "", "sw", "Hatari ya mafuriko kwa"},
		{"/api/risk", "sw-KE,en;q=0.5", "sw", "Hatari ya mafuriko kwa"},
		{"/api/risk?lang=en", "sw", "en", "Flood risk for"},
	} {
		w, summary, tips := send(c.path, c.accept)
		if w.Code != http.StatusOK || w.Header().Get("Content-Language") != c.want ||
			w.Header().Get("Vary") != "Accept-Language" || !strings.HasPrefix(summary, c.prefix) || len(tips) == 0 {
			t.Errorf("%s (Accept-Language %q): %d, Content-Language %q, summary %q", c.path, c.accept, w.Code,
				w.Header().Get("Content-Language"), summary)
		}
	}
}

func TestPostRiskWithWeatherRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"missing lat":    `{"lon":34.77,"weather":` + weatherJSON(0, 34.8) + `}`,
		"lat range":      `{"lat":91,"lon":34.77,"weather":` + weatherJSON(0, 34.8) + `}`,
		"lon range":      `{"lat":0,"lon":181,"weather":` + weatherJSON(0, 34.8) + `}`,
		"no weather":     `{"lat":0,"lon":34.77}`,
		"other location": `{"lat":10,"lon":34.77,"weather":` + weatherJSON(0, 34.8) + `}`,
		"not json":       `{`,
	}
	for name, body := range cases {
		if w := postClientWeather(t, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body %s", name, w.Code, w.Body)
		}
	}
}
