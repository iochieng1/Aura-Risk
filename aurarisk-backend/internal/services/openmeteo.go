package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"aurarisk-backend/internal/metrics"
)

const historicalDays = 7

const (
	// Free API, non-commercial use only (https://open-meteo.com/en/terms).
	OpenMeteoFreeURL = "https://api.open-meteo.com/v1/forecast"
	// Paid plans use this host with an apikey parameter.
	OpenMeteoCustomerURL = "https://customer-api.open-meteo.com/v1/forecast"
)

type OpenMeteoResponse struct {
	UTCOffsetSeconds int `json:"utc_offset_seconds"`
	Current          struct {
		Time             string  `json:"time"`
		Temperature2m    float64 `json:"temperature_2m"`
		RelativeHumidity float64 `json:"relative_humidity_2m"`
		Precipitation    float64 `json:"precipitation"`
		Rain             float64 `json:"rain"`
		WeatherCode      int     `json:"weather_code"`
	} `json:"current"`
	Hourly struct {
		Time          []string  `json:"time"`
		Precipitation []float64 `json:"precipitation"`
		Rain          []float64 `json:"rain"`
		SoilMoisture  []float64 `json:"soil_moisture_0_to_7cm"`
	} `json:"hourly"`
}

// ErrRateLimited means Open-Meteo answered 429. Retrying immediately would
// only make it worse, so it is returned without further attempts.
var ErrRateLimited = errors.New("weather API rate limit reached")

// OpenMeteoClient fetches forecasts with per-attempt timeouts and retries
// transient failures (network errors and 5xx) with jittered backoff.
type OpenMeteoClient struct {
	URL      string // defaults to OpenMeteoFreeURL, or the customer URL when APIKey is set
	APIKey   string // paid plans only
	HTTP     *http.Client
	Attempts int           // total attempts, default 3
	Timeout  time.Duration // per attempt, default 5s
	Backoff  time.Duration // first retry delay, doubled each time, default 300ms
}

func (c *OpenMeteoClient) endpoint() string {
	switch {
	case c.URL != "":
		return c.URL
	case c.APIKey != "":
		return OpenMeteoCustomerURL
	default:
		return OpenMeteoFreeURL
	}
}

// Fetch returns current conditions, 7 days of history, and today's forecast.
func (c *OpenMeteoClient) Fetch(ctx context.Context, lat, lon float64) (*OpenMeteoResponse, error) {
	attempts := c.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	backoff := c.Backoff
	if backoff <= 0 {
		backoff = 300 * time.Millisecond
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		data, retryable, err := c.fetchOnce(ctx, lat, lon)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable || attempt == attempts {
			break
		}
		// Full jitter keeps instances from retrying in lockstep.
		delay := time.Duration(rand.Int63n(int64(backoff) << (attempt - 1)))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func (c *OpenMeteoClient) fetchOnce(ctx context.Context, lat, lon float64) (_ *OpenMeteoResponse, retryable bool, err error) {
	defer metrics.ObserveProvider(metrics.ProviderOpenMeteo, "forecast", time.Now(), &err)

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.6f", lat))
	q.Set("longitude", fmt.Sprintf("%.6f", lon))
	q.Set("past_days", fmt.Sprint(historicalDays))
	q.Set("forecast_days", "1")
	q.Set("current", "temperature_2m,relative_humidity_2m,precipitation,rain,weather_code")
	q.Set("hourly", "precipitation,rain,soil_moisture_0_to_7cm")
	q.Set("timezone", "auto")
	if c.APIKey != "" {
		q.Set("apikey", c.APIKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint()+"?"+q.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "AuraRisk/1.0 (+https://github.com/iochieng1/Aura-Risk)")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		// Transport errors embed the request URL; keep the API key out of logs.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			urlErr.URL = c.endpoint()
		}
		// The caller giving up is final; a timeout or network error may pass.
		if errors.Is(err, context.Canceled) {
			return nil, false, err
		}
		return nil, true, fmt.Errorf("failed to fetch weather data: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, false, ErrRateLimited
	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	var data OpenMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, true, fmt.Errorf("failed to decode weather data: %w", err)
	}

	if observedAt, ok := data.ObservedAt(); ok {
		metrics.ObserveWeatherAge(time.Since(observedAt))
	}
	return &data, false, nil
}

// ObservedAt returns when the current conditions were valid. Open-Meteo
// reports local time without an offset when timezone=auto is requested.
func (r *OpenMeteoResponse) ObservedAt() (time.Time, bool) {
	local, err := time.Parse("2006-01-02T15:04", r.Current.Time)
	if err != nil {
		return time.Time{}, false
	}
	return local.Add(-time.Duration(r.UTCOffsetSeconds) * time.Second), true
}
