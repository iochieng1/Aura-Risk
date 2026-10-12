package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// captureLogs sends slog output to a buffer as JSON for the test's duration.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		records = append(records, rec)
	}
	return records
}

func TestAccessLogIncludesRequestID(t *testing.T) {
	buf := captureLogs(t)
	r := gin.New()
	r.Use(RequestID(), AccessLog(), Recovery())
	r.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	req := httptest.NewRequest(http.MethodGet, "/items/7", nil)
	req.Header.Set(RequestIDHeader, "trace-abc")
	r.ServeHTTP(httptest.NewRecorder(), req)

	records := logRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("got %d records: %v", len(records), records)
	}
	rec := records[0]
	want := map[string]any{
		"level": "INFO", "msg": "request", "request_id": "trace-abc",
		"method": "GET", "path": "/items/7", "route": "/items/:id", "status": float64(418),
	}
	for key, value := range want {
		if rec[key] != value {
			t.Errorf("%s = %v, want %v", key, rec[key], value)
		}
	}
}

func TestPanicIsOneErrorRecordWithRequestID(t *testing.T) {
	buf := captureLogs(t)
	r := gin.New()
	r.Use(RequestID(), AccessLog(), Recovery())
	r.GET("/boom", func(c *gin.Context) { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set(RequestIDHeader, "trace-panic")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}

	records := logRecords(t, buf)
	if len(records) != 2 {
		t.Fatalf("got %d records: %v", len(records), records)
	}
	for _, rec := range records {
		if rec["level"] != "ERROR" || rec["request_id"] != "trace-panic" {
			t.Errorf("record = %v, want ERROR with request_id", rec)
		}
	}
	if records[0]["panic"] != "kaboom" || !strings.Contains(records[0]["stack"].(string), "goroutine") {
		t.Errorf("panic record = %v", records[0])
	}
}
