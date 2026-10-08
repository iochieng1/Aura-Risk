package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter() *gin.Engine {
	r := gin.New()
	r.Use(RequestID(), Recovery())
	r.HandleMethodNotAllowed = true
	r.NoRoute(NotFound)
	r.NoMethod(MethodNotAllowed)
	return r
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %q", w.Body.String())
	}
	return body
}

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	r := newRouter()
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, GetRequestID(c)) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	id := w.Header().Get(RequestIDHeader)
	if len(id) != 24 || w.Body.String() != id {
		t.Fatalf("generated id %q, handler saw %q", id, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "edge-abc.123")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get(RequestIDHeader); got != "edge-abc.123" {
		t.Fatalf("caller id not kept: %q", got)
	}

	// Unsafe IDs (log injection, oversized) are replaced.
	for _, bad := range []string{"a\nb", strings.Repeat("x", 65), "<script>"} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(RequestIDHeader, bad)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Get(RequestIDHeader); got == bad || len(got) != 24 {
			t.Fatalf("bad id %q was kept as %q", bad, got)
		}
	}
}

func TestErrorsAreStructured(t *testing.T) {
	r := newRouter()
	r.GET("/missing", func(c *gin.Context) { respondError(c, http.StatusNotFound, "report not found") })
	r.GET("/panic", func(c *gin.Context) { panic("boom") })
	r.GET("/internal", func(c *gin.Context) { internalError(c, sql.ErrConnDone) })

	cases := []struct {
		method, path string
		status       int
		code, msg    string
	}{
		{http.MethodGet, "/missing", 404, "not_found", "report not found"},
		{http.MethodGet, "/nowhere", 404, "not_found", "not found"},
		{http.MethodPost, "/missing", 405, "method_not_allowed", "method not allowed"},
		{http.MethodGet, "/panic", 500, "internal_server_error", "internal server error"},
		{http.MethodGet, "/internal", 500, "internal_server_error", "internal server error"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		body := decodeError(t, w)
		if w.Code != tc.status || body.Code != tc.code || body.Error != tc.msg || body.RequestID != w.Header().Get(RequestIDHeader) {
			t.Errorf("%s %s: %d %+v", tc.method, tc.path, w.Code, body)
		}
		if strings.Contains(w.Body.String(), "connection") {
			t.Errorf("%s leaks the internal error: %s", tc.path, w.Body.String())
		}
	}
}

func TestBadRequestBodyDistinguishesTooLarge(t *testing.T) {
	r := newRouter()
	r.POST("/x", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16)
		var v map[string]any
		if err := c.ShouldBindJSON(&v); err != nil {
			badRequestBody(c, err)
		}
	})
	for body, want := range map[string]int{
		`{"note":"` + strings.Repeat("x", 64) + `"}`: http.StatusRequestEntityTooLarge,
		`{not json`: http.StatusBadRequest,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))
		if w.Code != want {
			t.Errorf("body %.20q: status %d, want %d", body, w.Code, want)
		}
	}
}

func TestRateLimiterPerClient(t *testing.T) {
	l := NewRateLimiter("test", 2)
	clock := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return clock }

	r := newRouter()
	r.GET("/x", l.Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	get := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	get("10.0.0.1")
	get("10.0.0.1")
	w := get("10.0.0.1")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "30" || decodeError(t, w).Code != "too_many_requests" {
		t.Fatalf("third request: %d Retry-After=%q %s", w.Code, w.Header().Get("Retry-After"), w.Body)
	}
	if get("10.0.0.2").Code != http.StatusOK {
		t.Fatal("another client was limited")
	}
	clock = clock.Add(30 * time.Second) // one token back at 2/minute
	if get("10.0.0.1").Code != http.StatusOK {
		t.Fatal("not refilled")
	}

	clock = clock.Add(2 * time.Minute)
	get("10.0.0.3")
	if len(l.clients) != 1 {
		t.Fatalf("idle clients not swept: %d left", len(l.clients))
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	if NewRateLimiter("off", 0) != nil {
		t.Fatal("0 should disable the limiter")
	}
	r := newRouter()
	r.GET("/x", NewRateLimiter("off", 0).Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
}

func TestReadyFailsWithoutDatabase(t *testing.T) {
	unreachable, err := sql.Open("postgres", "postgres://u:p@127.0.0.1:1/x?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer unreachable.Close()
	prev := db
	SetDatabase(unreachable)
	defer SetDatabase(prev)

	r := newRouter()
	r.GET("/health/live", Live)
	r.GET("/health/ready", Ready)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("live: %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusServiceUnavailable || body.Checks["database"] != checkFailing || body.Checks["weather"] != checkOK {
		t.Fatalf("ready: %d %s", w.Code, w.Body)
	}
}
