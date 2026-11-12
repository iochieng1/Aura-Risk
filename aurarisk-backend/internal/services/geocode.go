package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"aurarisk-backend/internal/metrics"
)

// Place names for risk summaries (#85). A client that knows the place (a
// search result) sends its name; otherwise the point is reverse-geocoded
// with OpenStreetMap Nominatim, and when that fails the summary names the
// coordinates instead. Names are only ever displayed.

// MaxPlaceNameRunes bounds a place name, whether sent by a client or
// returned by the geocoder.
const MaxPlaceNameRunes = 100

var (
	ErrPlaceNameTooLong = fmt.Errorf("name must be at most %d characters", MaxPlaceNameRunes)
	ErrPlaceNameInvalid = errors.New("name must be valid text without control characters")
)

// ValidatePlaceName checks a client-supplied place name and returns it
// trimmed. An empty result means no name was given.
func ValidatePlaceName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrPlaceNameInvalid
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > MaxPlaceNameRunes {
		return "", ErrPlaceNameTooLong
	}
	if strings.IndexFunc(name, disallowedInName) >= 0 {
		return "", ErrPlaceNameInvalid
	}
	return name, nil
}

// disallowedInName matches control characters, including the bidirectional
// overrides that can make displayed text read differently from what it is.
func disallowedInName(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
}

// cleanPlaceName makes geocoder output safe to show: control characters are
// dropped, whitespace collapsed, and the length bounded.
func cleanPlaceName(name string) string {
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if disallowedInName(r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if runes := []rune(name); len(runes) > MaxPlaceNameRunes {
		name = strings.TrimSpace(string(runes[:MaxPlaceNameRunes]))
	}
	return name
}

// CoordinateLabel names a point by its coordinates, e.g. "-1.2921, 36.8219",
// for when no place name is known.
func CoordinateLabel(lat, lon float64) string {
	return fmt.Sprintf("%.4f, %.4f", lat, lon)
}

// ReverseGeocoder names the place at a point. *NominatimClient satisfies it.
type ReverseGeocoder interface {
	ReverseGeocode(ctx context.Context, lat, lon float64) (string, error)
}

var (
	// ErrNoPlace means the geocoder knows no place at the point (e.g. open
	// water).
	ErrNoPlace = errors.New("no place name at this point")
	// ErrGeocodeRateLimited means the geocoder refused the request for its
	// rate or usage limits (HTTP 429 or 403).
	ErrGeocodeRateLimited = errors.New("geocoding rate limit reached")
)

const (
	NominatimDefaultURL = "https://nominatim.openstreetmap.org"
	// DefaultGeocodeContact goes in the User-Agent when none is configured.
	DefaultGeocodeContact = "https://github.com/iochieng1/Aura-Risk"
)

// NominatimUserAgent identifies the application to Nominatim, as its usage
// policy requires: the app name and a contact (URL or email address).
func NominatimUserAgent(contact string) string {
	contact = strings.TrimSpace(contact)
	if contact == "" {
		contact = DefaultGeocodeContact
	}
	return "AuraRisk/1.0 (+" + contact + ")"
}

// NominatimClient reverse-geocodes with the Nominatim API
// (https://nominatim.org/release-docs/latest/api/Reverse/). It makes one
// attempt per call; GeocodeService adds caching and the rate limit.
type NominatimClient struct {
	URL       string // base URL, default NominatimDefaultURL
	UserAgent string // default NominatimUserAgent("")
	Language  string // accept-language, default "en"
	HTTP      *http.Client
}

type nominatimReverse struct {
	Error       string            `json:"error"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Address     map[string]string `json:"address"`
}

func (c *NominatimClient) ReverseGeocode(ctx context.Context, lat, lon float64) (name string, err error) {
	defer metrics.ObserveProvider(metrics.ProviderNominatim, "reverse", time.Now(), &err)

	base := strings.TrimRight(c.URL, "/")
	if base == "" {
		base = NominatimDefaultURL
	}
	lang := c.Language
	if lang == "" {
		lang = LangEnglish
	}
	q := url.Values{}
	q.Set("format", "jsonv2")
	q.Set("lat", fmt.Sprintf("%.6f", lat))
	q.Set("lon", fmt.Sprintf("%.6f", lon))
	q.Set("zoom", "14") // suburb level
	q.Set("addressdetails", "1")
	q.Set("accept-language", lang)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/reverse?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = NominatimUserAgent("")
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("%w (HTTP %d)", ErrGeocodeRateLimited, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("nominatim returned HTTP %d", resp.StatusCode)
	}

	var body nominatimReverse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding nominatim response: %w", err)
	}
	if body.Error != "" {
		return "", ErrNoPlace
	}
	if name := placeLabel(body); name != "" {
		return name, nil
	}
	return "", ErrNoPlace
}

// placeLabel builds a short name such as "Kibera, Nairobi" from a reverse
// geocoding result: the most local named area, then the town or county.
func placeLabel(r nominatimReverse) string {
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := cleanPlaceName(r.Address[k]); v != "" {
				return v
			}
		}
		return ""
	}
	local := first("suburb", "neighbourhood", "quarter", "village", "hamlet", "town", "city_district", "city", "municipality")
	if local == "" {
		local = cleanPlaceName(r.Name)
	}
	var region string
	for _, k := range []string{"city", "town", "municipality", "state", "county"} {
		if v := cleanPlaceName(r.Address[k]); v != "" && v != local {
			region = v
			break
		}
	}
	switch {
	case local != "" && region != "":
		return cleanPlaceName(local + ", " + region)
	case local != "":
		return local
	case region != "":
		return region
	}
	// Last resort: the first two parts of the full address.
	parts := strings.SplitN(r.DisplayName, ",", 3)
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return cleanPlaceName(strings.Join(parts, ","))
}

// GeocodeConfig tunes GeocodeService. Zero values use defaults.
type GeocodeConfig struct {
	// Cell size in degrees; points in one cell share a name. Same default
	// as the weather cache (about 2 km).
	GridDegrees float64
	// How long a name is reused. Place names rarely change, and Nominatim's
	// policy asks for caching.
	CacheFor time.Duration
	// How long a failed lookup is remembered before trying the cell again.
	NegativeCacheFor time.Duration
	// Minimum gap between outgoing requests from this instance. Nominatim
	// allows at most one per second.
	MinInterval time.Duration
	// Pause after the provider refuses a request for its limits.
	RateLimitedPause time.Duration
	// Upper bound on one lookup.
	Timeout time.Duration
	// Bound on cached cells.
	MaxEntries int
}

func (c GeocodeConfig) withDefaults() GeocodeConfig {
	if c.GridDegrees <= 0 {
		c.GridDegrees = defaultGridDegrees
	}
	if c.CacheFor <= 0 {
		c.CacheFor = 7 * 24 * time.Hour
	}
	if c.NegativeCacheFor <= 0 {
		c.NegativeCacheFor = 10 * time.Minute
	}
	if c.MinInterval < time.Second {
		c.MinInterval = time.Second
	}
	if c.RateLimitedPause <= 0 {
		c.RateLimitedPause = time.Minute
	}
	if c.Timeout <= 0 {
		c.Timeout = 2 * time.Second
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = 10000
	}
	return c
}

type cachedPlace struct {
	name    string // "" for a failed lookup
	expires time.Time
}

// GeocodeService caches reverse-geocoded names by grid cell and keeps
// outgoing requests within the provider's rate limit. It never blocks a
// caller waiting for the limit: a lookup that would exceed it is skipped,
// and the caller shows coordinates instead.
type GeocodeService struct {
	geocoder ReverseGeocoder
	cfg      GeocodeConfig
	now      func() time.Time

	mu          sync.Mutex
	cache       map[cellKey]cachedPlace
	nextCall    time.Time
	pausedUntil time.Time
}

func NewGeocodeService(g ReverseGeocoder, cfg GeocodeConfig) *GeocodeService {
	return &GeocodeService{
		geocoder: g,
		cfg:      cfg.withDefaults(),
		now:      time.Now,
		cache:    map[cellKey]cachedPlace{},
	}
}

// Lookup returns the place name for the point's grid cell, or false when
// none is known (geocoding failed, was rate limited, or found nothing).
func (s *GeocodeService) Lookup(ctx context.Context, lat, lon float64) (string, bool) {
	if s == nil || s.geocoder == nil {
		return "", false
	}
	key, cellLat, cellLon := gridCell(lat, lon, s.cfg.GridDegrees)

	s.mu.Lock()
	now := s.now()
	if c, ok := s.cache[key]; ok && now.Before(c.expires) {
		s.mu.Unlock()
		metrics.ObserveGeocodeLookup("hit")
		return c.name, c.name != ""
	}
	if now.Before(s.pausedUntil) || now.Before(s.nextCall) {
		s.mu.Unlock()
		metrics.ObserveGeocodeLookup("throttled")
		return "", false
	}
	s.nextCall = now.Add(s.cfg.MinInterval)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	// Geocode the cell's centre so the cached name represents the cell.
	name, err := s.geocoder.ReverseGeocode(ctx, cellLat, cellLon)
	if err == nil {
		name = cleanPlaceName(name)
		if name == "" {
			err = ErrNoPlace
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now = s.now()
	switch {
	case err == nil:
		s.storeLocked(key, cachedPlace{name: name, expires: now.Add(s.cfg.CacheFor)})
		metrics.ObserveGeocodeLookup("fetched")
		return name, true
	case errors.Is(err, ErrGeocodeRateLimited):
		s.pausedUntil = now.Add(s.cfg.RateLimitedPause)
		metrics.ObserveGeocodeLookup("rate_limited")
	case errors.Is(err, context.Canceled):
		// The caller went away; that says nothing about the cell.
		return "", false
	default:
		s.storeLocked(key, cachedPlace{expires: now.Add(s.cfg.NegativeCacheFor)})
		metrics.ObserveGeocodeLookup("failed")
	}
	if !errors.Is(err, ErrNoPlace) {
		slog.Warn("reverse geocoding failed", "lat", cellLat, "lon", cellLon, "error", err.Error())
	}
	return "", false
}

func (s *GeocodeService) storeLocked(key cellKey, entry cachedPlace) {
	if _, exists := s.cache[key]; !exists && len(s.cache) >= s.cfg.MaxEntries {
		// Drop expired entries, then the one expiring soonest if still full.
		now := s.now()
		var soonestKey cellKey
		var soonest time.Time
		for k, v := range s.cache {
			if !now.Before(v.expires) {
				delete(s.cache, k)
				continue
			}
			if soonest.IsZero() || v.expires.Before(soonest) {
				soonestKey, soonest = k, v.expires
			}
		}
		if len(s.cache) >= s.cfg.MaxEntries {
			delete(s.cache, soonestKey)
		}
	}
	s.cache[key] = entry
}

// geocodeService names points without a client-supplied name; nil (the
// default, and in tests) disables reverse geocoding.
var geocodeService *GeocodeService

// SetGeocodeService sets the reverse geocoder. Call it before serving.
func SetGeocodeService(s *GeocodeService) {
	geocodeService = s
}

// PlaceName names a point for display: its reverse-geocoded place when one
// can be had, otherwise its coordinates. It never fails.
func PlaceName(ctx context.Context, lat, lon float64) string {
	if name, ok := geocodeService.Lookup(ctx, lat, lon); ok {
		return name
	}
	return CoordinateLabel(lat, lon)
}
