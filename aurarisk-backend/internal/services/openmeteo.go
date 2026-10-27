package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aurarisk-backend/internal/metrics"
)

// historicalDays of hourly history are requested: the score's longest
// antecedent-rain window (7 days) needs all of them. forecastDays covers the
// score's forecast window (forecastHours ahead) even late in the local day,
// since Open-Meteo forecast days end at local midnight.
const (
	historicalDays = 7
	forecastDays   = 2
)

const (
	// Free API, non-commercial use only (https://open-meteo.com/en/terms).
	OpenMeteoFreeURL = "https://api.open-meteo.com/v1/forecast"
	// Paid plans use this host with an apikey parameter.
	OpenMeteoCustomerURL = "https://customer-api.open-meteo.com/v1/forecast"

	// Ground elevation (Copernicus DEM) and GloFAS river discharge.
	OpenMeteoElevationFreeURL     = "https://api.open-meteo.com/v1/elevation"
	OpenMeteoElevationCustomerURL = "https://customer-api.open-meteo.com/v1/elevation"
	OpenMeteoFloodFreeURL         = "https://flood-api.open-meteo.com/v1/flood"
	OpenMeteoFloodCustomerURL     = "https://customer-flood-api.open-meteo.com/v1/flood"
)

type OpenMeteoResponse struct {
	// The grid point Open-Meteo used, close to the requested coordinates.
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	UTCOffsetSeconds int     `json:"utc_offset_seconds"`
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
	URL    string // defaults to OpenMeteoFreeURL, or the customer URL when APIKey is set
	APIKey string // paid plans only
	// Elevation and Flood API endpoints, defaulting like URL.
	ElevationURL string
	FloodURL     string
	HTTP         *http.Client
	Attempts     int           // total attempts, default 3
	Timeout      time.Duration // per attempt, default 5s
	Backoff      time.Duration // first retry delay, doubled each time, default 300ms
}

func (c *OpenMeteoClient) endpoint() string {
	return c.pick(c.URL, OpenMeteoFreeURL, OpenMeteoCustomerURL)
}

func (c *OpenMeteoClient) pick(configured, free, customer string) string {
	switch {
	case configured != "":
		return configured
	case c.APIKey != "":
		return customer
	default:
		return free
	}
}

// Fetch returns current conditions, historicalDays of hourly history, and
// forecastDays of hourly forecast.
func (c *OpenMeteoClient) Fetch(ctx context.Context, lat, lon float64) (*OpenMeteoResponse, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.6f", lat))
	q.Set("longitude", fmt.Sprintf("%.6f", lon))
	q.Set("past_days", fmt.Sprint(historicalDays))
	q.Set("forecast_days", fmt.Sprint(forecastDays))
	q.Set("current", "temperature_2m,relative_humidity_2m,precipitation,rain,weather_code")
	q.Set("hourly", "precipitation,rain,soil_moisture_0_to_7cm")
	q.Set("timezone", "auto")

	var data OpenMeteoResponse
	if err := c.get(ctx, "forecast", c.endpoint(), q, &data); err != nil {
		return nil, err
	}
	if observedAt, ok := data.ObservedAt(); ok {
		metrics.ObserveWeatherAge(time.Since(observedAt))
	}
	return &data, nil
}

// get calls one Open-Meteo API and decodes its JSON answer into out,
// retrying transient failures. operation labels the provider metrics.
func (c *OpenMeteoClient) get(ctx context.Context, operation, endpoint string, q url.Values, out any) error {
	attempts := c.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	backoff := c.Backoff
	if backoff <= 0 {
		backoff = 300 * time.Millisecond
	}
	if c.APIKey != "" {
		q.Set("apikey", c.APIKey)
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		retryable, err := c.getOnce(ctx, operation, endpoint, q, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || attempt == attempts {
			break
		}
		// Full jitter keeps instances from retrying in lockstep.
		delay := time.Duration(rand.Int63n(int64(backoff) << (attempt - 1)))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func (c *OpenMeteoClient) getOnce(ctx context.Context, operation, endpoint string, q url.Values, out any) (retryable bool, err error) {
	defer metrics.ObserveProvider(metrics.ProviderOpenMeteo, operation, time.Now(), &err)

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return false, err
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
			urlErr.URL = endpoint
		}
		// The caller giving up is final; a timeout or network error may pass.
		if errors.Is(err, context.Canceled) {
			return false, err
		}
		return true, fmt.Errorf("failed to fetch %s data: %w", operation, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return false, fmt.Errorf("%w: %s", ErrRateLimited, errorReason(resp.Body))
	case resp.StatusCode >= 500:
		return true, fmt.Errorf("%s API returned status %d: %s", operation, resp.StatusCode, errorReason(resp.Body))
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("%s API returned status %d: %s", operation, resp.StatusCode, errorReason(resp.Body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return true, fmt.Errorf("failed to decode %s data: %w", operation, err)
	}
	return false, nil
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

// errorReason extracts Open-Meteo's {"error":true,"reason":"..."} message,
// falling back to the start of the body, so logs say why a call failed.
func errorReason(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, 512))
	var parsed struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(raw, &parsed) == nil && parsed.Reason != "" {
		return parsed.Reason
	}
	if text := strings.TrimSpace(string(raw)); text != "" {
		return strings.Join(strings.Fields(text), " ")
	}
	return "no reason given"
}
