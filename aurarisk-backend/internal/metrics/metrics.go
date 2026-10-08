// Package metrics exposes Prometheus metrics for the API, the database pool,
// external providers, and background workers. Dashboards and alert rules in
// monitoring/ depend on these names; rename them together.
package metrics

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// External providers. Keep these in sync with the alert rules.
const (
	ProviderOpenMeteo = "open_meteo"
	ProviderExpoPush  = "expo_push"
	ProviderS3        = "s3"
	ProviderClamAV    = "clamav"
)

const (
	outcomeSuccess = "success"
	outcomeError   = "error"
)

// Buckets span fast DB-only routes up to /api/risk, which waits on Open-Meteo
// with a 10s timeout.
var latencyBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15}

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aurarisk_http_requests_total",
		Help: "HTTP requests handled, by route template and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aurarisk_http_request_duration_seconds",
		Help:    "HTTP request latency, by route template.",
		Buckets: latencyBuckets,
	}, []string{"method", "route"})

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aurarisk_http_requests_in_flight",
		Help: "HTTP requests currently being served.",
	})

	providerRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aurarisk_provider_requests_total",
		Help: "Calls to external providers, by outcome.",
	}, []string{"provider", "operation", "outcome"})

	providerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aurarisk_provider_request_duration_seconds",
		Help:    "Latency of calls to external providers.",
		Buckets: latencyBuckets,
	}, []string{"provider", "operation"})

	providerLastSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aurarisk_provider_last_success_timestamp_seconds",
		Help: "Unix time of the last successful call to each provider.",
	}, []string{"provider"})

	weatherObservationAge = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aurarisk_weather_observation_age_seconds",
		Help:    "Age of Open-Meteo's current-conditions timestamp when it was fetched.",
		Buckets: []float64{300, 900, 1800, 3600, 2 * 3600, 3 * 3600, 6 * 3600, 12 * 3600, 24 * 3600},
	})

	weatherLookups = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aurarisk_weather_lookups_total",
		Help: "Weather lookups by how they were served: hit, fetched, stale, throttled, unavailable.",
	}, []string{"result"})

	circuitOpen = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aurarisk_provider_circuit_open",
		Help: "1 while calls to the provider are suspended after repeated failures.",
	}, []string{"provider"})

	pushTickets = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aurarisk_push_tickets_total",
		Help: "Expo push tickets returned, by status (ok, device_gone, error).",
	}, []string{"status"})

	workerRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aurarisk_worker_runs_total",
		Help: "Background worker runs, by outcome.",
	}, []string{"worker", "outcome"})

	workerLastSuccess = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aurarisk_worker_last_success_timestamp_seconds",
		Help: "Unix time of each worker's last successful run.",
	}, []string{"worker"})

	moderationQueue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aurarisk_moderation_queue_reports",
		Help: "Pending, non-duplicate reports awaiting verification or review.",
	})

	reportsVerified = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aurarisk_reports_auto_verified_total",
		Help: "Reports verified automatically by the verification algorithm.",
	})

	retentionDeleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aurarisk_retention_deleted_total",
		Help: "Rows removed by the retention worker, by kind and reason.",
	}, []string{"kind", "reason"})

	workerInterval = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aurarisk_worker_interval_seconds",
		Help: "Configured run interval of each worker.",
	}, []string{"worker"})
)

// RegisterDB exports connection pool statistics as go_sql_* metrics.
func RegisterDB(db *sql.DB, name string) {
	prometheus.MustRegister(collectors.NewDBStatsCollector(db, name))
}

// Middleware records request counts and latency. Routes are labelled by
// their template (/api/photos/:id), never the raw path, to bound cardinality.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		httpRequests.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Inc()
		httpDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

// Handler serves the default registry for Prometheus to scrape.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveProvider records one call to an external provider. Use it as
// defer metrics.ObserveProvider(provider, op, time.Now(), &err).
func ObserveProvider(provider, operation string, start time.Time, errp *error) {
	providerDuration.WithLabelValues(provider, operation).Observe(time.Since(start).Seconds())

	var err error
	if errp != nil {
		err = *errp
	}
	// A caller cancelling (shutdown, client gone) says nothing about the provider.
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		providerRequests.WithLabelValues(provider, operation, outcomeError).Inc()
		return
	}
	providerRequests.WithLabelValues(provider, operation, outcomeSuccess).Inc()
	providerLastSuccess.WithLabelValues(provider).SetToCurrentTime()
}

// ObserveWeatherAge records how old the fetched current conditions are.
func ObserveWeatherAge(age time.Duration) {
	if age < 0 {
		age = 0
	}
	weatherObservationAge.Observe(age.Seconds())
}

func ObservePushTicket(status string) {
	pushTickets.WithLabelValues(status).Inc()
}

// SetWorkerInterval lets alerts compare a worker's last success with its schedule.
func SetWorkerInterval(worker string, interval time.Duration) {
	workerInterval.WithLabelValues(worker).Set(interval.Seconds())
}

func ObserveWorkerRun(worker string, err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		workerRuns.WithLabelValues(worker, outcomeError).Inc()
		return
	}
	if err == nil {
		workerRuns.WithLabelValues(worker, outcomeSuccess).Inc()
		workerLastSuccess.WithLabelValues(worker).SetToCurrentTime()
	}
}

func SetModerationQueue(n int) {
	moderationQueue.Set(float64(n))
}

func AddReportsAutoVerified(n int) {
	reportsVerified.Add(float64(n))
}

// AddRetentionDeleted counts rows removed by retention, e.g. ("report", "rejected").
func AddRetentionDeleted(kind, reason string, n int) {
	if n > 0 {
		retentionDeleted.WithLabelValues(kind, reason).Add(float64(n))
	}
}

// ObserveWeatherLookup records how a weather lookup was served.
func ObserveWeatherLookup(result string) {
	weatherLookups.WithLabelValues(result).Inc()
}

func SetCircuitOpen(provider string, open bool) {
	v := 0.0
	if open {
		v = 1
	}
	circuitOpen.WithLabelValues(provider).Set(v)
}

var rateLimited = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aurarisk_rate_limited_total",
	Help: "Requests rejected with 429 by the per-client rate limiter, by limiter.",
}, []string{"limiter"})

// ObserveRateLimited counts a request rejected by the named rate limiter.
func ObserveRateLimited(limiter string) {
	rateLimited.WithLabelValues(limiter).Inc()
}
