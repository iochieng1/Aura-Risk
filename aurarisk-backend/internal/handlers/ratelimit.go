package handlers

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"aurarisk-backend/internal/metrics"
	"github.com/gin-gonic/gin"
)

// RateLimiter allows each client a number of requests per minute, with
// bursts up to that number, keyed by client IP. Limits are per instance:
// with N instances behind a balancer a client can reach N times the limit.
//
// Client IPs come from gin's ClientIP, which only honours X-Forwarded-For
// from trusted proxies (TRUSTED_PROXIES). Without that, every client behind
// a proxy shares the proxy's address and its limit.
type RateLimiter struct {
	name      string
	perMinute float64
	now       func() time.Time

	mu        sync.Mutex
	clients   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter returns nil when perMinute <= 0; a nil limiter allows all.
func NewRateLimiter(name string, perMinute int) *RateLimiter {
	if perMinute <= 0 {
		return nil
	}
	return &RateLimiter{
		name:      name,
		perMinute: float64(perMinute),
		now:       time.Now,
		clients:   map[string]*bucket{},
	}
}

// allow takes a token for key, or reports how long until one is available.
func (l *RateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweepLocked(now)

	b, ok := l.clients[key]
	if !ok {
		b = &bucket{tokens: l.perMinute, last: now}
		l.clients[key] = b
	}
	b.tokens = math.Min(l.perMinute, b.tokens+now.Sub(b.last).Minutes()*l.perMinute)
	b.last = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) / l.perMinute * float64(time.Minute))
	}
	b.tokens--
	return true, 0
}

// sweepLocked drops clients whose bucket has refilled, at most once a minute,
// so memory stays bounded by recently active clients.
func (l *RateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for k, b := range l.clients {
		if now.Sub(b.last) >= time.Minute {
			delete(l.clients, k)
		}
	}
}

// Middleware rejects over-limit requests with 429 and Retry-After.
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if l == nil {
			c.Next()
			return
		}
		ok, wait := l.allow(c.ClientIP())
		if !ok {
			metrics.ObserveRateLimited(l.name)
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			respondError(c, http.StatusTooManyRequests, "too many requests, slow down")
			return
		}
		c.Next()
	}
}
