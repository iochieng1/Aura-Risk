package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Ground and river inputs to the score, from Open-Meteo's Elevation API
// (Copernicus DEM) and Flood API (GloFAS). See docs/RISK_DATA_SOURCES.md.
const (
	// Elevation is compared with eight points this far away (~1 km).
	terrainNeighbourDegrees = 0.01
	// GloFAS cells are ~5 km. The nearest cell is often a small side stream,
	// so the neighbouring cells are searched too and the one carrying the
	// most water is taken as the local main river.
	riverNeighbourDegrees = 0.05
	// The baseline is the median daily discharge over the past riverPastDays;
	// the peak is the highest forecast for today and the next riverPeakDays-1.
	riverPastDays     = 92
	riverForecastDays = 7
	riverPeakDays     = 3
)

// Terrain describes the ground around a point.
type Terrain struct {
	// Elevation of the point, in metres.
	Elevation float64
	// How far the point lies below the average of the ground ~1 km around
	// it, in metres. Negative when it sits higher than its surroundings.
	Depression float64
}

// RiverFlow is GloFAS discharge for the main river near a point.
type RiverFlow struct {
	// The GloFAS cell used.
	Lat, Lon float64
	// Median daily discharge over the past riverPastDays, in m³/s.
	Baseline float64
	// Highest daily discharge forecast over the next riverPeakDays, in m³/s.
	Peak float64
}

// Hydrology holds the ground and river inputs. Either may be nil when its
// provider is unavailable; the score then leaves that term out.
type Hydrology struct {
	Terrain *Terrain
	River   *RiverFlow
}

// FetchTerrain returns the elevation of the point and of eight points around
// it, in one Elevation API call.
func (c *OpenMeteoClient) FetchTerrain(ctx context.Context, lat, lon float64) (*Terrain, error) {
	lats, lons := neighbourhood(lat, lon, terrainNeighbourDegrees)
	q := url.Values{}
	q.Set("latitude", strings.Join(lats, ","))
	q.Set("longitude", strings.Join(lons, ","))

	var data struct {
		Elevation []float64 `json:"elevation"`
	}
	if err := c.get(ctx, "elevation", c.pick(c.ElevationURL, OpenMeteoElevationFreeURL, OpenMeteoElevationCustomerURL), q, &data); err != nil {
		return nil, err
	}
	if len(data.Elevation) != len(lats) {
		return nil, fmt.Errorf("elevation API returned %d values for %d points", len(data.Elevation), len(lats))
	}
	return terrainFrom(data.Elevation), nil
}

// terrainFrom turns neighbourhood elevations (centre first) into a Terrain.
func terrainFrom(elevations []float64) *Terrain {
	centre, around := elevations[0], elevations[1:]
	sum := 0.0
	for _, e := range around {
		sum += e
	}
	return &Terrain{Elevation: centre, Depression: sum/float64(len(around)) - centre}
}

// FetchRiver returns discharge for the main river near the point, in one
// Flood API call covering the point's GloFAS cell and its neighbours.
func (c *OpenMeteoClient) FetchRiver(ctx context.Context, lat, lon float64) (*RiverFlow, error) {
	lats, lons := neighbourhood(lat, lon, riverNeighbourDegrees)
	q := url.Values{}
	q.Set("latitude", strings.Join(lats, ","))
	q.Set("longitude", strings.Join(lons, ","))
	q.Set("daily", "river_discharge")
	q.Set("past_days", fmt.Sprint(riverPastDays))
	q.Set("forecast_days", fmt.Sprint(riverForecastDays))

	// Several locations come back as an array, one object per location.
	var cells []floodCell
	if err := c.get(ctx, "flood", c.pick(c.FloodURL, OpenMeteoFloodFreeURL, OpenMeteoFloodCustomerURL), q, &cells); err != nil {
		return nil, err
	}
	return mainRiver(cells)
}

type floodCell struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Daily     struct {
		// Null where GloFAS has no value; decoded as nil.
		RiverDischarge []*float64 `json:"river_discharge"`
	} `json:"daily"`
}

var errNoRiverData = errors.New("flood API returned no usable discharge series")

// mainRiver picks the cell with the highest baseline discharge and summarises
// it. Past days come first in each series, then today and the forecast.
func mainRiver(cells []floodCell) (*RiverFlow, error) {
	var best *RiverFlow
	for _, cell := range cells {
		series := cell.Daily.RiverDischarge
		if len(series) != riverPastDays+riverForecastDays {
			continue
		}
		past := values(series[:riverPastDays])
		ahead := values(series[riverPastDays : riverPastDays+riverPeakDays])
		if len(past) == 0 || len(ahead) == 0 {
			continue
		}
		flow := &RiverFlow{Lat: cell.Latitude, Lon: cell.Longitude, Baseline: median(past), Peak: slices.Max(ahead)}
		if best == nil || flow.Baseline > best.Baseline {
			best = flow
		}
	}
	if best == nil {
		return nil, errNoRiverData
	}
	return best, nil
}

func values(series []*float64) []float64 {
	var out []float64
	for _, v := range series {
		if v != nil {
			out = append(out, *v)
		}
	}
	return out
}

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// neighbourhood returns the point followed by the eight points step degrees
// away, formatted for a multi-location Open-Meteo request.
func neighbourhood(lat, lon, step float64) (lats, lons []string) {
	add := func(la, lo float64) {
		lats = append(lats, fmt.Sprintf("%.4f", la))
		lons = append(lons, fmt.Sprintf("%.4f", lo))
	}
	add(lat, lon)
	for _, dy := range []float64{-1, 0, 1} {
		for _, dx := range []float64{-1, 0, 1} {
			if dy != 0 || dx != 0 {
				add(lat+dy*step, lon+dx*step)
			}
		}
	}
	return lats, lons
}

// HydrologyFetcher fetches ground and river inputs. *OpenMeteoClient
// satisfies it.
type HydrologyFetcher interface {
	FetchTerrain(ctx context.Context, lat, lon float64) (*Terrain, error)
	FetchRiver(ctx context.Context, lat, lon float64) (*RiverFlow, error)
}

// HydrologyConfig tunes HydrologyService. Zero values use defaults.
type HydrologyConfig struct {
	// Longest a risk request waits for these inputs before scoring without
	// them. Default 4s.
	Timeout time.Duration
	// How long river discharge is reused. GloFAS updates once a day.
	// Default 6h.
	RiverFreshFor time.Duration
	// How long older discharge may still be used while the Flood API is
	// failing. Default 48h.
	RiverStaleFor time.Duration
	// After a failed fetch, the cell isn't retried for this long, so an
	// outage doesn't add a timeout to every request. Default 10m.
	RetryAfter time.Duration
	// Bound on cached cells of each kind. Default 20000.
	MaxEntries int
}

func (c HydrologyConfig) withDefaults() HydrologyConfig {
	if c.Timeout <= 0 {
		c.Timeout = 4 * time.Second
	}
	if c.RiverFreshFor <= 0 {
		c.RiverFreshFor = 6 * time.Hour
	}
	if c.RiverStaleFor <= 0 {
		c.RiverStaleFor = 48 * time.Hour
	}
	if c.RetryAfter <= 0 {
		c.RetryAfter = 10 * time.Minute
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = 20000
	}
	return c
}

// Terrain is cached by ~500 m cell and never expires (the ground doesn't
// change); river discharge by GloFAS-sized cell.
const (
	terrainGridDegrees = 0.005
	riverGridDegrees   = 0.05
)

type hydrologyEntry[T any] struct {
	value     *T
	fetchedAt time.Time
	failedAt  time.Time
}

// HydrologyService caches ground and river inputs per grid cell. Failures
// never fail a risk request: the input is left out and the confidence says so.
type HydrologyService struct {
	fetcher HydrologyFetcher
	cfg     HydrologyConfig
	now     func() time.Time

	mu      sync.Mutex
	terrain map[cellKey]*hydrologyEntry[Terrain]
	river   map[cellKey]*hydrologyEntry[RiverFlow]
}

func NewHydrologyService(fetcher HydrologyFetcher, cfg HydrologyConfig) *HydrologyService {
	return &HydrologyService{
		fetcher: fetcher,
		cfg:     cfg.withDefaults(),
		now:     time.Now,
		terrain: map[cellKey]*hydrologyEntry[Terrain]{},
		river:   map[cellKey]*hydrologyEntry[RiverFlow]{},
	}
}

// Get returns whatever inputs are available for the point within the
// configured timeout. Terrain and river are fetched concurrently.
func (s *HydrologyService) Get(ctx context.Context, lat, lon float64) Hydrology {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	var h Hydrology
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		h.Terrain = lookup(ctx, s, s.terrain, terrainGridDegrees, lat, lon, 0, 0, "elevation", s.fetcher.FetchTerrain)
	}()
	go func() {
		defer wg.Done()
		h.River = lookup(ctx, s, s.river, riverGridDegrees, lat, lon, s.cfg.RiverFreshFor, s.cfg.RiverStaleFor, "river discharge", s.fetcher.FetchRiver)
	}()
	wg.Wait()
	return h
}

// lookup serves a cached value while fresh (freshFor 0 means forever),
// otherwise fetches at the cell centre, falling back to a cached value up to
// staleFor old.
func lookup[T any](ctx context.Context, s *HydrologyService, cache map[cellKey]*hydrologyEntry[T], grid, lat, lon float64,
	freshFor, staleFor time.Duration, what string, fetch func(context.Context, float64, float64) (*T, error)) *T {
	key := cellKey{int64(math.Floor(lat / grid)), int64(math.Floor(lon / grid))}
	cellLat, cellLon := (float64(key.lat)+0.5)*grid, (float64(key.lon)+0.5)*grid

	s.mu.Lock()
	now := s.now()
	entry := cache[key]
	var cached *T
	if entry != nil && entry.value != nil {
		age := now.Sub(entry.fetchedAt)
		if freshFor == 0 || age < freshFor {
			s.mu.Unlock()
			return entry.value
		}
		if age <= staleFor {
			cached = entry.value
		}
	}
	if entry != nil && now.Sub(entry.failedAt) < s.cfg.RetryAfter {
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	value, err := fetch(ctx, cellLat, cellLon)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("hydrology fetch failed", "input", what, "lat", cellLat, "lon", cellLon, "error", err.Error())
		}
		if entry == nil {
			entry = &hydrologyEntry[T]{}
			storeBounded(cache, key, entry, s.cfg.MaxEntries)
		}
		entry.failedAt = s.now()
		return cached
	}
	storeBounded(cache, key, &hydrologyEntry[T]{value: value, fetchedAt: s.now()}, s.cfg.MaxEntries)
	return value
}

// storeBounded adds an entry, first dropping an arbitrary one when full.
// Terrain never changes and discharge is cheap to refetch, so which one goes
// doesn't matter much.
func storeBounded[T any](cache map[cellKey]*hydrologyEntry[T], key cellKey, entry *hydrologyEntry[T], max int) {
	if _, exists := cache[key]; !exists && len(cache) >= max {
		for k := range cache {
			delete(cache, k)
			break
		}
	}
	cache[key] = entry
}
