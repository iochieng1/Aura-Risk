package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMiddlewareLabelsByRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/api/photos/:id", func(c *gin.Context) { c.Status(http.StatusNotFound) })

	for _, path := range []string{"/api/photos/a", "/api/photos/b", "/nope"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	if got := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "/api/photos/:id", "404")); got != 2 {
		t.Fatalf("templated route count = %v, want 2", got)
	}
	if got := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "unmatched", "404")); got != 1 {
		t.Fatalf("unmatched route count = %v, want 1", got)
	}
}

func TestObserveProvider(t *testing.T) {
	const provider = "test_provider"
	start := time.Now()

	failure := errors.New("boom")
	ObserveProvider(provider, "op", start, &failure)
	ObserveProvider(provider, "op", start, nil)
	canceled := context.Canceled
	ObserveProvider(provider, "op", start, &canceled)

	if got := testutil.ToFloat64(providerRequests.WithLabelValues(provider, "op", outcomeError)); got != 1 {
		t.Fatalf("errors = %v, want 1 (cancellation must not count)", got)
	}
	if got := testutil.ToFloat64(providerRequests.WithLabelValues(provider, "op", outcomeSuccess)); got != 1 {
		t.Fatalf("successes = %v, want 1", got)
	}
	if got := testutil.ToFloat64(providerLastSuccess.WithLabelValues(provider)); got < float64(start.Unix()) {
		t.Fatalf("last success = %v, want >= %d", got, start.Unix())
	}
}

func TestObserveWorkerRun(t *testing.T) {
	ObserveWorkerRun("test_worker", errors.New("db down"))
	ObserveWorkerRun("test_worker", context.Canceled)

	if got := testutil.ToFloat64(workerRuns.WithLabelValues("test_worker", outcomeError)); got != 1 {
		t.Fatalf("errors = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(workerLastSuccess, "aurarisk_worker_last_success_timestamp_seconds"); got != 0 {
		t.Fatalf("last success series = %d, want none before a successful run", got)
	}
}
