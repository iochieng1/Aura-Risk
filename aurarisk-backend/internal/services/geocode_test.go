package services

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeGeocoder struct {
	mu     sync.Mutex
	calls  int
	points [][2]float64
	name   string
	err    error
}

func (f *fakeGeocoder) ReverseGeocode(ctx context.Context, lat, lon float64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.points = append(f.points, [2]float64{lat, lon})
	return f.name, f.err
}

func (f *fakeGeocoder) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestGeocodeService(g ReverseGeocoder, cfg GeocodeConfig) (*GeocodeService, *clock) {
	c := &clock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	s := NewGeocodeService(g, cfg)
	s.now = c.now
	return s, c
}

func TestValidatePlaceName(t *testing.T) {
	ok := map[string]string{
		"":                       "",
		"   ":                    "",
		" Kibera ":               "Kibera",
		"Murang'a, Kenya":        "Murang'a, Kenya",
		strings.Repeat("é", 100): strings.Repeat("é", 100), // runes, not bytes
	}
	for in, want := range ok {
		if got, err := ValidatePlaceName(in); err != nil || got != want {
			t.Errorf("ValidatePlaceName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := map[string]error{
		strings.Repeat("a", 101): ErrPlaceNameTooLong,
		"Kibera\x00":             ErrPlaceNameInvalid,
		"Kib\nera":               ErrPlaceNameInvalid,
		"Kibera\u202e":           ErrPlaceNameInvalid, // right-to-left override
		"Kibera\xff":             ErrPlaceNameInvalid, // invalid UTF-8
	}
	for in, want := range bad {
		if _, err := ValidatePlaceName(in); !errors.Is(err, want) {
			t.Errorf("ValidatePlaceName(%q) err = %v, want %v", in, err, want)
		}
	}
}

func TestCoordinateLabel(t *testing.T) {
	if got := CoordinateLabel(-1.29207, 36.82195); got != "-1.2921, 36.8219" && got != "-1.2921, 36.8220" {
		t.Fatalf("CoordinateLabel = %q", got)
	}
}

func TestPlaceLabel(t *testing.T) {
	cases := []struct {
		r    nominatimReverse
		want string
	}{
		{nominatimReverse{Address: map[string]string{"suburb": "Kibera", "city": "Nairobi", "state": "Nairobi County"}}, "Kibera, Nairobi"},
		{nominatimReverse{Address: map[string]string{"village": "Nyalenda", "state": "Kisumu County"}}, "Nyalenda, Kisumu County"},
		{nominatimReverse{Address: map[string]string{"city": "Kisumu", "state": "Kisumu County"}}, "Kisumu, Kisumu County"},
		{nominatimReverse{Address: map[string]string{"state": "Turkana County"}}, "Turkana County"},
		{nominatimReverse{DisplayName: "Lake Victoria, Kenya, East Africa"}, "Lake Victoria, Kenya"},
		{nominatimReverse{Address: map[string]string{"suburb": "Bad\u202eName\x07"}}, "Bad Name"},
	}
	for _, c := range cases {
		if got := placeLabel(c.r); got != c.want {
			t.Errorf("placeLabel(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
	if got := placeLabel(nominatimReverse{Name: strings.Repeat("x", 300)}); len([]rune(got)) != MaxPlaceNameRunes {
		t.Errorf("long name not truncated: %d runes", len([]rune(got)))
	}
}

func TestNominatimClient(t *testing.T) {
	var gotUA, gotPath, gotQuery string
	status, body := http.StatusOK, `{"name":"Kibera","display_name":"Kibera, Nairobi, Kenya","address":{"suburb":"Kibera","city":"Nairobi"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotPath, gotQuery = r.UserAgent(), r.URL.Path, r.URL.RawQuery
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &NominatimClient{URL: srv.URL + "/", UserAgent: NominatimUserAgent("https://example.org/aurarisk")}

	name, err := c.ReverseGeocode(context.Background(), -1.31, 36.79)
	if err != nil || name != "Kibera, Nairobi" {
		t.Fatalf("ReverseGeocode = %q, %v", name, err)
	}
	if gotPath != "/reverse" || !strings.Contains(gotQuery, "format=jsonv2") || !strings.Contains(gotQuery, "lat=-1.310000") ||
		!strings.Contains(gotQuery, "lon=36.790000") || !strings.Contains(gotQuery, "accept-language=en") {
		t.Errorf("request %s?%s", gotPath, gotQuery)
	}
	if gotUA != "AuraRisk/1.0 (+https://example.org/aurarisk)" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if NominatimUserAgent("") != "AuraRisk/1.0 (+"+DefaultGeocodeContact+")" {
		t.Errorf("default User-Agent = %q", NominatimUserAgent(""))
	}

	for _, c2 := range []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusTooManyRequests, ``, ErrGeocodeRateLimited},
		{http.StatusForbidden, ``, ErrGeocodeRateLimited},
		{http.StatusOK, `{"error":"Unable to geocode"}`, ErrNoPlace},
		{http.StatusOK, `{"address":{}}`, ErrNoPlace},
	} {
		status, body = c2.status, c2.body
		if _, err := c.ReverseGeocode(context.Background(), 0, 0); !errors.Is(err, c2.want) {
			t.Errorf("HTTP %d %s: err = %v, want %v", c2.status, c2.body, err, c2.want)
		}
	}
	for _, c2 := range []struct {
		status int
		body   string
	}{{http.StatusInternalServerError, `{}`}, {http.StatusOK, `not json`}} {
		status, body = c2.status, c2.body
		if name, err := c.ReverseGeocode(context.Background(), 0, 0); err == nil {
			t.Errorf("HTTP %d %s: got %q, want an error", c2.status, c2.body, name)
		}
	}
}

func TestGeocodeServiceCachesPerCell(t *testing.T) {
	g := &fakeGeocoder{name: "Kibera, Nairobi"}
	s, clk := newTestGeocodeService(g, GeocodeConfig{})

	if name, ok := s.Lookup(context.Background(), -1.3101, 36.7901); !ok || name != "Kibera, Nairobi" {
		t.Fatalf("Lookup = %q, %v", name, ok)
	}
	// The cell's centre was geocoded.
	if p := g.points[0]; math.Abs(p[0]+1.31) > 1e-9 || math.Abs(p[1]-36.79) > 1e-9 {
		t.Errorf("geocoded %v, want the cell centre", p)
	}
	clk.advance(5 * time.Second)
	// Same 0.02° cell: served from the cache.
	if name, ok := s.Lookup(context.Background(), -1.3199, 36.7801); !ok || name != "Kibera, Nairobi" || g.count() != 1 {
		t.Fatalf("cached Lookup = %q, %v after %d calls", name, ok, g.count())
	}
	// Another cell needs its own lookup.
	s.Lookup(context.Background(), -0.09, 34.77)
	if g.count() != 2 {
		t.Fatalf("%d calls, want 2", g.count())
	}
	// Names expire after CacheFor.
	clk.advance(8 * 24 * time.Hour)
	s.Lookup(context.Background(), -1.31, 36.79)
	if g.count() != 3 {
		t.Fatalf("%d calls after expiry, want 3", g.count())
	}
}

func TestGeocodeServiceRateLimit(t *testing.T) {
	g := &fakeGeocoder{name: "Somewhere"}
	s, clk := newTestGeocodeService(g, GeocodeConfig{MinInterval: 10 * time.Millisecond}) // raised to 1s

	s.Lookup(context.Background(), 0.01, 0.01)
	// A second cell within the same second is skipped, not queued.
	clk.advance(999 * time.Millisecond)
	if name, ok := s.Lookup(context.Background(), 1.01, 1.01); ok || g.count() != 1 {
		t.Fatalf("within 1s: %q, %v, %d calls", name, ok, g.count())
	}
	// A skipped lookup isn't cached as a failure.
	clk.advance(time.Millisecond)
	if _, ok := s.Lookup(context.Background(), 1.01, 1.01); !ok || g.count() != 2 {
		t.Fatalf("after 1s: %v, %d calls", ok, g.count())
	}
}

func TestGeocodeServiceFailures(t *testing.T) {
	g := &fakeGeocoder{err: errors.New("boom")}
	s, clk := newTestGeocodeService(g, GeocodeConfig{})

	if _, ok := s.Lookup(context.Background(), 0.01, 0.01); ok {
		t.Fatal("failed lookup reported a name")
	}
	// The failure is remembered briefly...
	clk.advance(5 * time.Minute)
	g.err, g.name = nil, "Recovered"
	if _, ok := s.Lookup(context.Background(), 0.01, 0.01); ok || g.count() != 1 {
		t.Fatalf("negative cache: %v, %d calls", ok, g.count())
	}
	// ...then retried.
	clk.advance(6 * time.Minute)
	if name, ok := s.Lookup(context.Background(), 0.01, 0.01); !ok || name != "Recovered" {
		t.Fatalf("after negative TTL: %q, %v", name, ok)
	}

	// The provider refusing us pauses every lookup for a while.
	g.err = ErrGeocodeRateLimited
	clk.advance(2 * time.Second)
	s.Lookup(context.Background(), 2.01, 2.01)
	calls := g.count()
	g.err = nil
	clk.advance(30 * time.Second)
	if _, ok := s.Lookup(context.Background(), 3.01, 3.01); ok || g.count() != calls {
		t.Fatalf("during pause: %v, %d calls (was %d)", ok, g.count(), calls)
	}
	clk.advance(31 * time.Second)
	if _, ok := s.Lookup(context.Background(), 3.01, 3.01); !ok {
		t.Fatal("pause did not end")
	}
}

type slowGeocoder struct{}

func (slowGeocoder) ReverseGeocode(ctx context.Context, _, _ float64) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestGeocodeServiceTimeout(t *testing.T) {
	s := NewGeocodeService(slowGeocoder{}, GeocodeConfig{Timeout: 20 * time.Millisecond})
	start := time.Now()
	if _, ok := s.Lookup(context.Background(), 0, 0); ok {
		t.Fatal("timed-out lookup reported a name")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("lookup took %s", d)
	}
}

func TestPlaceNameFallsBackToCoordinates(t *testing.T) {
	t.Cleanup(func() { SetGeocodeService(nil) })

	SetGeocodeService(nil)
	if got := PlaceName(context.Background(), -1.2921, 36.8219); got != "-1.2921, 36.8219" {
		t.Errorf("disabled: %q", got)
	}
	SetGeocodeService(NewGeocodeService(&fakeGeocoder{err: errors.New("down")}, GeocodeConfig{}))
	if got := PlaceName(context.Background(), -1.2921, 36.8219); got != "-1.2921, 36.8219" {
		t.Errorf("failing: %q", got)
	}
	SetGeocodeService(NewGeocodeService(&fakeGeocoder{name: "Kibera, Nairobi"}, GeocodeConfig{}))
	if got := PlaceName(context.Background(), -1.31, 36.79); got != "Kibera, Nairobi" {
		t.Errorf("working: %q", got)
	}
}

func TestAssessmentUsesPlaceName(t *testing.T) {
	t.Cleanup(func() { SetGeocodeService(nil) })
	weather := &OpenMeteoResponse{}
	now := time.Now()

	for _, c := range []struct {
		name, geocoded, want string
	}{
		{"Kibera", "", "Kibera"},
		{"", "Nyalenda, Kisumu", "Nyalenda, Kisumu"},
		{"", "", "-0.0700, 34.7700"},
	} {
		g := &fakeGeocoder{name: c.geocoded}
		if c.geocoded == "" {
			g.err = errors.New("unavailable")
		}
		SetGeocodeService(NewGeocodeService(g, GeocodeConfig{}))
		risk := AssessClientWeather(context.Background(), -0.07, 34.77, c.name, weather, Hydrology{}, now)
		if risk.Location.Name != c.want || !strings.HasPrefix(risk.Summary, "Flood risk for "+c.want+" is") {
			t.Errorf("%+v: name %q, summary %q", c, risk.Location.Name, risk.Summary)
		}
		if c.name != "" && g.count() != 0 {
			t.Errorf("%+v: geocoded although a name was given", c)
		}
		LocalizeRisk(risk, LangSwahili)
		if !strings.HasPrefix(risk.Summary, "Hatari ya mafuriko kwa "+c.want+" ") {
			t.Errorf("%+v: Swahili summary %q", c, risk.Summary)
		}
		if strings.Contains(risk.Summary, "Selected Location") {
			t.Errorf("placeholder in %q", risk.Summary)
		}
	}
}
