package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"aurarisk-backend/config"
	"github.com/gin-gonic/gin"
)

// dbFailureRouter serves every database-backed route against conn. It needs
// no real database.
func dbFailureRouter(t *testing.T, conn *sql.DB, timeout time.Duration) *gin.Engine {
	t.Helper()
	previous := db
	SetDatabase(conn)
	t.Cleanup(func() { SetDatabase(previous); conn.Close() })
	SetModerators(map[string]string{"alice": "alice-token"})
	t.Cleanup(func() { SetModerators(nil) })
	if err := SetResponders(map[string]config.Responder{"desk": {County: "Kisumu", Token: "desk-token"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetResponders(nil) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/health/ready", Ready)
	api := r.Group("/api", RequestTimeout(timeout))
	api.GET("/reports", GetReports)
	api.POST("/reports", OptionalAuth(), CreateReport)
	api.GET("/stats/counties", GetCountyStats)
	api.GET("/open-data/reports.csv", GetOpenDataReports("csv"))
	api.POST("/auth/device", RegisterDevice)
	api.POST("/auth/refresh", RefreshTokens)
	api.GET("/subscriptions", RequireAuth(), ListSubscriptions)
	api.GET("/moderation/reports", RequireModerator(), ListModerationQueue)
	api.GET("/responder/reports", RequireResponder(), ListResponderReports)
	return r
}

type dbFailureCase struct {
	method, path, token string
	body                any
}

var dbFailureCases = []dbFailureCase{
	{http.MethodGet, "/api/reports?lat=-0.09&lon=34.77", "", nil},
	{http.MethodGet, "/api/reports?county=Kisumu", "", nil},
	{http.MethodPost, "/api/reports", "", map[string]any{
		"location": map[string]any{"name": "Nyalenda", "lat": -0.1, "lon": 34.75}, "category": "flooding", "note": "Water rising",
	}},
	{http.MethodPost, "/api/reports", "some-access-token", map[string]any{}},
	{http.MethodGet, "/api/stats/counties", "", nil},
	{http.MethodGet, "/api/open-data/reports.csv", "", nil},
	{http.MethodPost, "/api/auth/device", "", map[string]any{"platform": "android"}},
	{http.MethodPost, "/api/auth/refresh", "", map[string]any{"refresh_token": "x"}},
	{http.MethodGet, "/api/subscriptions", "some-access-token", nil},
	{http.MethodGet, "/api/moderation/reports", "alice-token", nil},
	{http.MethodGet, "/api/responder/reports", "desk-token", nil},
}

// checkUnavailable expects every route, and readiness, to answer 503 with a
// JSON error and Retry-After, within limit.
func checkUnavailable(t *testing.T, r *gin.Engine, limit time.Duration) {
	t.Helper()
	for _, tc := range dbFailureCases {
		start := time.Now()
		w := call(t, r, tc.method, tc.path, tc.token, tc.body)
		elapsed := time.Since(start)
		var body struct {
			Error string `json:"error"`
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		name := tc.method + " " + tc.path
		if w.Code != http.StatusServiceUnavailable || body.Error == "" || w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d %s, want 503 with an error and Retry-After", name, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "dial") || strings.Contains(w.Body.String(), "127.0.0.1") {
			t.Errorf("%s: error leaks connection details: %s", name, w.Body)
		}
		if elapsed > limit {
			t.Errorf("%s: took %s, want under %s", name, elapsed, limit)
		}
	}
	if w := call(t, r, http.MethodGet, "/health/ready", "", nil); w.Code != http.StatusServiceUnavailable ||
		!strings.Contains(w.Body.String(), `"database":"failing"`) {
		t.Errorf("ready: %d %s", w.Code, w.Body)
	}
}

// Postgres is down: connections are refused.
func TestHandlersWhenDatabaseIsDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens there now

	conn, err := sql.Open("postgres", fmt.Sprintf("postgres://u:p@%s/x?sslmode=disable", addr))
	if err != nil {
		t.Fatal(err)
	}
	checkUnavailable(t, dbFailureRouter(t, conn, 5*time.Second), 2*time.Second)
}

// Postgres accepts connections but never answers: requests give up at the
// request deadline instead of hanging.
func TestHandlersWhenDatabaseHangs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		var held []net.Conn
		defer func() {
			for _, c := range held {
				c.Close()
			}
		}()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, c)
		}
	}()

	// connect_timeout as database.Connect adds it; lib/pq ignores the context
	// during the handshake.
	conn, err := sql.Open("postgres", fmt.Sprintf("postgres://u:p@%s/x?sslmode=disable&connect_timeout=1", ln.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	checkUnavailable(t, dbFailureRouter(t, conn, 300*time.Millisecond), 2*time.Second)
}
