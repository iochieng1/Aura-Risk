package services

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"aurarisk-backend/internal/metrics"
)

// ErrWeatherUnavailable means no fresh data could be fetched and no cached
// data is recent enough to serve under the stale-data policy.
var ErrWeatherUnavailable = errors.New("weather data is temporarily unavailable")

// WeatherFetcher fetches weather for a point. *OpenMeteoClient satisfies it.
type WeatherFetcher interface {
	Fetch(ctx context.Context, lat, lon float64) (*OpenMeteoResponse, error)
}

// WeatherResult is weather data plus where it came from.
type WeatherResult struct {
	Data      *OpenMeteoResponse
	FetchedAt time.Time
	// Stale is set when the provider could not be reached and an older
	// cached copy within the stale-data window was served instead.
	Stale bool
}

// WeatherConfig tunes caching and failure handling. Zero values use defaults.
type WeatherConfig struct {
	// Cell size in degrees; points in the same cell share one fetch. 0.02° is
	// about 2 km, finer than Open-Meteo's ~1-11 km model grids.
	GridDegrees float64
	// How long a fetch is served without asking again. Open-Meteo updates
	// current conditions every 15 minutes.
	FreshFor time.Duration
	// How long after a fetch its data may still be served, marked stale,
	// while the provider is failing. Negative disables stale serving.
	StaleFor time.Duration
	// Bound on cached cells; the oldest are evicted first.
	MaxEntries int
	// Consecutive failures before calls are suspended, and for how long.
	BreakerThreshold int
	BreakerCooldown  time.Duration
	// Outgoing call budget per instance. The free API allows 600/minute.
	MaxRequestsPerMinute int
	// Upper bound on one fetch, retries included. It runs detached from the
	// requests waiting on it so one cancelled caller doesn't fail the rest.
	FetchTimeout time.Duration
}

func (c WeatherConfig) withDefaults() WeatherConfig {
	if c.GridDegrees <= 0 {
		c.GridDegrees = 0.02
	}
	if c.FreshFor <= 0 {
		c.FreshFor = 15 * time.Minute
	}
	if c.StaleFor < 0 {
		c.StaleFor = 0
	} else if c.StaleFor == 0 {
		c.StaleFor = 3 * time.Hour
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = 5000
	}
	if c.BreakerThreshold <= 0 {
		c.BreakerThreshold = 5
	}
	if c.BreakerCooldown <= 0 {
		c.BreakerCooldown = 30 * time.Second
	}
	if c.MaxRequestsPerMinute <= 0 {
		c.MaxRequestsPerMinute = 300
	}
	if c.FetchTimeout <= 0 {
		c.FetchTimeout = 15 * time.Second
	}
	return c
}

type cellKey struct{ lat, lon int64 }

type cachedWeather struct {
	data      *OpenMeteoResponse
	fetchedAt time.Time
}

type inflight struct {
	done      chan struct{}
	data      *OpenMeteoResponse
	fetchedAt time.Time
	err       error
}

// WeatherService caches weather by grid cell and time window, shares one
// fetch among concurrent requests for a cell, suspends calls to a failing
// provider (circuit breaker), stays within an outgoing call budget, and
// serves recent cached data when fresh data cannot be had.
type WeatherService struct {
	fetcher WeatherFetcher
	cfg     WeatherConfig
	now     func() time.Time

	mu       sync.Mutex
	cache    map[cellKey]cachedWeather
	inflight map[cellKey]*inflight

	// Circuit breaker.
	failures  int
	openUntil time.Time
	probing   bool

	// Token bucket for outgoing calls.
	tokens     float64
	lastRefill time.Time
}

func NewWeatherService(fetcher WeatherFetcher, cfg WeatherConfig) *WeatherService {
	cfg = cfg.withDefaults()
	return &WeatherService{
		fetcher:  fetcher,
		cfg:      cfg,
		now:      time.Now,
		cache:    map[cellKey]cachedWeather{},
		inflight: map[cellKey]*inflight{},
		tokens:   float64(cfg.MaxRequestsPerMinute),
	}
}

func (s *WeatherService) cell(lat, lon float64) (cellKey, float64, float64) {
	g := s.cfg.GridDegrees
	k := cellKey{int64(math.Floor(lat / g)), int64(math.Floor(lon / g))}
	// Fetch at the cell's centre so the cached data represents the whole cell.
	return k, (float64(k.lat) + 0.5) * g, (float64(k.lon) + 0.5) * g
}

// Get returns weather for the point's grid cell.
func (s *WeatherService) Get(ctx context.Context, lat, lon float64) (WeatherResult, error) {
	key, cellLat, cellLon := s.cell(lat, lon)

	s.mu.Lock()
	now := s.now()
	cached, hasCached := s.cache[key]
	if hasCached && now.Sub(cached.fetchedAt) < s.cfg.FreshFor {
		s.mu.Unlock()
		metrics.ObserveWeatherLookup("hit")
		return WeatherResult{Data: cached.data, FetchedAt: cached.fetchedAt}, nil
	}

	call, joined := s.inflight[key]
	if !joined {
		if !s.allowCallLocked(now) {
			s.mu.Unlock()
			return s.fallback(cached, hasCached, "throttled")
		}
		call = &inflight{done: make(chan struct{})}
		s.inflight[key] = call
		go s.fetch(key, cellLat, cellLon, call)
	}
	s.mu.Unlock()

	select {
	case <-call.done:
	case <-ctx.Done():
		return WeatherResult{}, ctx.Err()
	}
	if call.err != nil {
		return s.fallback(cached, hasCached, "unavailable")
	}
	metrics.ObserveWeatherLookup("fetched")
	return WeatherResult{Data: call.data, FetchedAt: call.fetchedAt}, nil
}

// fallback applies the stale-data policy after a failed or skipped fetch.
func (s *WeatherService) fallback(cached cachedWeather, ok bool, reason string) (WeatherResult, error) {
	if ok && s.now().Sub(cached.fetchedAt) <= s.cfg.StaleFor {
		metrics.ObserveWeatherLookup("stale")
		return WeatherResult{Data: cached.data, FetchedAt: cached.fetchedAt, Stale: true}, nil
	}
	metrics.ObserveWeatherLookup(reason)
	return WeatherResult{}, ErrWeatherUnavailable
}

func (s *WeatherService) fetch(key cellKey, lat, lon float64, call *inflight) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.FetchTimeout)
	defer cancel()
	data, err := s.fetcher.Fetch(ctx, lat, lon)

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, key)
	call.data, call.err, call.fetchedAt = data, err, s.now()
	if err == nil {
		s.storeLocked(key, cachedWeather{data: data, fetchedAt: call.fetchedAt})
	}
	s.recordResultLocked(err)
	close(call.done)
}

func (s *WeatherService) storeLocked(key cellKey, entry cachedWeather) {
	if _, exists := s.cache[key]; !exists && len(s.cache) >= s.cfg.MaxEntries {
		// Drop everything past the stale window, then the oldest if still full.
		now := s.now()
		var oldestKey cellKey
		var oldest time.Time
		for k, v := range s.cache {
			if now.Sub(v.fetchedAt) > s.cfg.StaleFor {
				delete(s.cache, k)
				continue
			}
			if oldest.IsZero() || v.fetchedAt.Before(oldest) {
				oldestKey, oldest = k, v.fetchedAt
			}
		}
		if len(s.cache) >= s.cfg.MaxEntries {
			delete(s.cache, oldestKey)
		}
	}
	s.cache[key] = entry
}

// allowCallLocked applies the circuit breaker and the call budget.
func (s *WeatherService) allowCallLocked(now time.Time) bool {
	if now.Before(s.openUntil) {
		return false
	}
	if !s.openUntil.IsZero() {
		// Cooldown over: let one call through to probe the provider.
		if s.probing {
			return false
		}
		s.probing = true
	}

	perSecond := float64(s.cfg.MaxRequestsPerMinute) / 60
	if !s.lastRefill.IsZero() {
		s.tokens = math.Min(float64(s.cfg.MaxRequestsPerMinute), s.tokens+now.Sub(s.lastRefill).Seconds()*perSecond)
	}
	s.lastRefill = now
	if s.tokens < 1 {
		s.probing = false
		return false
	}
	s.tokens--
	return true
}

func (s *WeatherService) recordResultLocked(err error) {
	s.probing = false
	if err == nil {
		s.failures = 0
		s.openUntil = time.Time{}
		metrics.SetCircuitOpen(metrics.ProviderOpenMeteo, false)
		return
	}
	s.failures++
	// A failed probe reopens at once; otherwise open after the threshold.
	if !s.openUntil.IsZero() || s.failures >= s.cfg.BreakerThreshold {
		s.openUntil = s.now().Add(s.cfg.BreakerCooldown)
		metrics.SetCircuitOpen(metrics.ProviderOpenMeteo, true)
	}
}

// CircuitOpen reports whether calls to the provider are currently suspended.
func (s *WeatherService) CircuitOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.now().Before(s.openUntil)
}
