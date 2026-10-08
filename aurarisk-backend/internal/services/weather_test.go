package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeFetcher struct {
	mu    sync.Mutex
	calls int
	err   error
	block chan struct{} // when set, Fetch waits for it to close
}

func (f *fakeFetcher) Fetch(_ context.Context, lat, lon float64) (*OpenMeteoResponse, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	r := &OpenMeteoResponse{}
	r.Current.Precipitation = lat // lets tests tell responses apart
	return r, nil
}

func (f *fakeFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeFetcher) fail(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestService(f WeatherFetcher, cfg WeatherConfig) (*WeatherService, *clock) {
	c := &clock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	s := NewWeatherService(f, cfg)
	s.now = c.now
	return s, c
}

func TestWeatherCachesByGridCell(t *testing.T) {
	f := &fakeFetcher{}
	s, clk := newTestService(f, WeatherConfig{})
	ctx := context.Background()

	if _, err := s.Get(ctx, -1.2900, 36.8200); err != nil {
		t.Fatal(err)
	}
	// About 500 m away, same 0.02° cell: served from cache.
	r, err := s.Get(ctx, -1.2950, 36.8230)
	if err != nil || r.Stale || f.count() != 1 {
		t.Fatalf("same cell: calls=%d stale=%v err=%v", f.count(), r.Stale, err)
	}
	// A different cell is fetched separately.
	s.Get(ctx, -1.35, 36.82)
	if f.count() != 2 {
		t.Fatalf("other cell: calls=%d, want 2", f.count())
	}
	// After the fresh window the cell is fetched again.
	clk.advance(16 * time.Minute)
	s.Get(ctx, -1.29, 36.82)
	if f.count() != 3 {
		t.Fatalf("after expiry: calls=%d, want 3", f.count())
	}
}

func TestWeatherFetchesCellCentre(t *testing.T) {
	f := &fakeFetcher{}
	s, _ := newTestService(f, WeatherConfig{GridDegrees: 0.02})
	r, _ := s.Get(context.Background(), 0.001, 0.001)
	if got := r.Data.Current.Precipitation; got < 0.0099 || got > 0.0101 {
		t.Fatalf("fetched lat %v, want the cell centre 0.01", got)
	}
}

func TestWeatherConcurrentRequestsShareOneFetch(t *testing.T) {
	f := &fakeFetcher{block: make(chan struct{})}
	s, _ := newTestService(f, WeatherConfig{})

	var wg sync.WaitGroup
	var errs atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Get(context.Background(), 10, 10); err != nil {
				errs.Add(1)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond) // let every caller join the in-flight fetch
	close(f.block)
	wg.Wait()
	if f.count() != 1 || errs.Load() != 0 {
		t.Fatalf("calls=%d errors=%d, want 1 call and no errors", f.count(), errs.Load())
	}
}

func TestWeatherWaiterCanGiveUp(t *testing.T) {
	f := &fakeFetcher{block: make(chan struct{})}
	defer close(f.block)
	s, _ := newTestService(f, WeatherConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.Get(ctx, 10, 10); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestWeatherServesStaleWithinWindow(t *testing.T) {
	f := &fakeFetcher{}
	s, clk := newTestService(f, WeatherConfig{StaleFor: 3 * time.Hour})
	ctx := context.Background()
	first, _ := s.Get(ctx, 5, 5)

	f.fail(errors.New("provider down"))
	clk.advance(2 * time.Hour)
	r, err := s.Get(ctx, 5, 5)
	if err != nil || !r.Stale || !r.FetchedAt.Equal(first.FetchedAt) {
		t.Fatalf("within window: stale=%v fetchedAt=%v err=%v", r.Stale, r.FetchedAt, err)
	}

	clk.advance(2 * time.Hour) // now 4h old
	if _, err := s.Get(ctx, 5, 5); !errors.Is(err, ErrWeatherUnavailable) {
		t.Fatalf("past window: err = %v, want ErrWeatherUnavailable", err)
	}
}

func TestWeatherStaleDisabled(t *testing.T) {
	f := &fakeFetcher{}
	s, clk := newTestService(f, WeatherConfig{StaleFor: -1})
	s.Get(context.Background(), 5, 5)
	f.fail(errors.New("down"))
	clk.advance(16 * time.Minute)
	if _, err := s.Get(context.Background(), 5, 5); !errors.Is(err, ErrWeatherUnavailable) {
		t.Fatalf("err = %v, want ErrWeatherUnavailable with stale serving off", err)
	}
}

func TestWeatherCircuitBreaker(t *testing.T) {
	f := &fakeFetcher{err: errors.New("down")}
	s, clk := newTestService(f, WeatherConfig{BreakerThreshold: 3, BreakerCooldown: 30 * time.Second})
	ctx := context.Background()

	// Different cells so the cache never answers.
	for i := 0; i < 3; i++ {
		s.Get(ctx, float64(i), 0)
	}
	if f.count() != 3 {
		t.Fatalf("calls before opening = %d", f.count())
	}
	// Open: fail fast without calling the provider.
	for i := 10; i < 15; i++ {
		if _, err := s.Get(ctx, float64(i), 0); !errors.Is(err, ErrWeatherUnavailable) {
			t.Fatalf("open breaker: err = %v", err)
		}
	}
	if f.count() != 3 {
		t.Fatalf("provider called while open: calls=%d", f.count())
	}

	// After the cooldown one probe goes through; failing reopens it.
	clk.advance(31 * time.Second)
	s.Get(ctx, 20, 0)
	s.Get(ctx, 21, 0)
	if f.count() != 4 {
		t.Fatalf("failed probe: calls=%d, want exactly one probe", f.count())
	}

	// A successful probe closes it.
	f.fail(nil)
	clk.advance(31 * time.Second)
	if _, err := s.Get(ctx, 30, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, 31, 0); err != nil || f.count() != 6 {
		t.Fatalf("closed again: calls=%d err=%v", f.count(), err)
	}
}

func TestWeatherCallBudget(t *testing.T) {
	f := &fakeFetcher{}
	s, clk := newTestService(f, WeatherConfig{MaxRequestsPerMinute: 2})
	ctx := context.Background()

	s.Get(ctx, 1, 0)
	s.Get(ctx, 2, 0)
	if _, err := s.Get(ctx, 3, 0); !errors.Is(err, ErrWeatherUnavailable) {
		t.Fatalf("over budget: err = %v", err)
	}
	// Cached cells are still served while throttled.
	if _, err := s.Get(ctx, 1, 0); err != nil {
		t.Fatalf("cached cell while throttled: %v", err)
	}
	clk.advance(30 * time.Second) // refills one token at 2/minute
	if _, err := s.Get(ctx, 3, 0); err != nil || f.count() != 3 {
		t.Fatalf("after refill: calls=%d err=%v", f.count(), err)
	}
}

func TestWeatherEvictsOldestWhenFull(t *testing.T) {
	f := &fakeFetcher{}
	s, clk := newTestService(f, WeatherConfig{MaxEntries: 2})
	ctx := context.Background()
	s.Get(ctx, 1, 0)
	clk.advance(time.Minute)
	s.Get(ctx, 2, 0)
	clk.advance(time.Minute)
	s.Get(ctx, 3, 0) // evicts cell 1
	if len(s.cache) != 2 {
		t.Fatalf("cache size %d, want 2", len(s.cache))
	}
	s.Get(ctx, 1, 0)
	if f.count() != 4 {
		t.Fatalf("evicted cell not refetched: calls=%d", f.count())
	}
}

func openMeteoServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, attempt int)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, int(attempts.Add(1)))
	}))
	t.Cleanup(srv.Close)
	return srv, &attempts
}

const okBody = `{"utc_offset_seconds":0,"current":{"time":"2026-10-07T12:00","precipitation":1.5},"hourly":{"precipitation":[0,1]}}`

func TestOpenMeteoClientRetriesServerErrors(t *testing.T) {
	srv, attempts := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte(okBody))
	})
	c := &OpenMeteoClient{URL: srv.URL, Backoff: time.Millisecond}
	data, err := c.Fetch(context.Background(), 1, 2)
	if err != nil || attempts.Load() != 3 || data.Current.Precipitation != 1.5 {
		t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
	}
}

func TestOpenMeteoClientDoesNotRetryClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests} {
		srv, attempts := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
			w.WriteHeader(status)
		})
		c := &OpenMeteoClient{URL: srv.URL, Backoff: time.Millisecond}
		_, err := c.Fetch(context.Background(), 1, 2)
		if err == nil || attempts.Load() != 1 {
			t.Fatalf("status %d: attempts=%d err=%v", status, attempts.Load(), err)
		}
		if status == http.StatusTooManyRequests && !errors.Is(err, ErrRateLimited) {
			t.Fatalf("429: err = %v, want ErrRateLimited", err)
		}
	}
}

func TestOpenMeteoClientRequest(t *testing.T) {
	var got *http.Request
	srv, _ := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		got = r
		w.Write([]byte(okBody))
	})
	c := &OpenMeteoClient{URL: srv.URL, APIKey: "secret-key"}
	if _, err := c.Fetch(context.Background(), -1.29, 36.82); err != nil {
		t.Fatal(err)
	}
	q := got.URL.Query()
	if q.Get("apikey") != "secret-key" || q.Get("latitude") != "-1.290000" || q.Get("past_days") != "7" {
		t.Fatalf("query = %v", q)
	}
	if !strings.HasPrefix(got.UserAgent(), "AuraRisk/") {
		t.Fatalf("User-Agent = %q", got.UserAgent())
	}

	if (&OpenMeteoClient{APIKey: "k"}).endpoint() != OpenMeteoCustomerURL || (&OpenMeteoClient{}).endpoint() != OpenMeteoFreeURL {
		t.Fatal("endpoint selection by API key is wrong")
	}
}

func TestOpenMeteoClientKeepsAPIKeyOutOfErrors(t *testing.T) {
	// Nothing listens here, so the transport fails with the URL in the error.
	c := &OpenMeteoClient{URL: "http://127.0.0.1:1/v1/forecast", APIKey: "secret-key", Attempts: 1}
	_, err := c.Fetch(context.Background(), 1, 2)
	if err == nil || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenMeteoClientDecodesResponse(t *testing.T) {
	srv, _ := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Write([]byte(`{"utc_offset_seconds":10800,"current":{"time":"2026-10-07T15:00","precipitation":2,"weather_code":95},
			"hourly":{"time":["a","b"],"precipitation":[0.5,1],"soil_moisture_0_to_7cm":[0.3,0.31]}}`))
	})
	data, err := (&OpenMeteoClient{URL: srv.URL}).Fetch(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	obs, ok := data.ObservedAt()
	if !ok || !obs.Equal(time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)) || data.Current.WeatherCode != 95 || len(data.Hourly.SoilMoisture) != 2 {
		t.Fatalf("decoded %+v observed %v", data, obs)
	}

	bad, _ := openMeteoServer(t, func(w http.ResponseWriter, r *http.Request, n int) { w.Write([]byte("{not json")) })
	if _, err := (&OpenMeteoClient{URL: bad.URL, Backoff: time.Millisecond}).Fetch(context.Background(), 1, 2); err == nil {
		t.Fatal("expected a decode error")
	}
}
