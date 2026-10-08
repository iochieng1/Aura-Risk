package handlers

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"aurarisk-backend/internal/services"
	"github.com/gin-gonic/gin"
)

const (
	checkOK          = "ok"
	checkFailing     = "failing"
	checkCircuitOpen = "circuit_open"
)

var draining atomic.Bool

// SetDraining marks the instance as shutting down: readiness fails so load
// balancers stop sending new traffic while in-flight requests finish.
func SetDraining() {
	draining.Store(true)
}

// Live reports that the process is up and serving HTTP. It checks no
// dependencies, so a database or provider outage never gets it restarted.
func Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready reports whether this instance should receive traffic: 503 when the
// database is unreachable or the instance is draining. A failing weather
// provider only marks it "degraded", since risk lookups can still be served
// from cache and every instance shares the same provider.
func Ready(c *gin.Context) {
	checks := gin.H{}
	status, code := "ok", http.StatusOK

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		checks["database"] = checkFailing
		status, code = "unavailable", http.StatusServiceUnavailable
	} else {
		checks["database"] = checkOK
	}

	if services.WeatherCircuitOpen() {
		checks["weather"] = checkCircuitOpen
		if code == http.StatusOK {
			status = "degraded"
		}
	} else {
		checks["weather"] = checkOK
	}

	if draining.Load() {
		checks["shutdown"] = "draining"
		status, code = "unavailable", http.StatusServiceUnavailable
	}

	c.JSON(code, gin.H{"status": status, "checks": checks})
}
