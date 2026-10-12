package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader carries the request ID in both directions. A caller (or the
// reverse proxy) may supply one; otherwise the backend generates it.
const RequestIDHeader = "X-Request-ID"

const requestIDKey = "request_id"

// Accept caller IDs only if they are short and log-safe.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// ErrorResponse is the body of every error the API returns. Error stays a
// plain string so existing clients keep working.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

// RequestID assigns each request an ID, echoes it in the response header,
// and makes it available to logs and error bodies.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GetRequestID returns the current request's ID, or "" outside RequestID.
func GetRequestID(c *gin.Context) string {
	return c.GetString(requestIDKey)
}

// respondError aborts the request with a structured error. The code is
// derived from the status, e.g. 404 -> "not_found".
func respondError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Error:     message,
		Code:      errorCode(status),
		RequestID: GetRequestID(c),
	})
}

func errorCode(status int) string {
	text := http.StatusText(status)
	if text == "" {
		return "error"
	}
	return strings.ReplaceAll(strings.ToLower(text), " ", "_")
}

// internalError logs err with the request ID and returns a generic 500, so
// internal details never reach the client but can be found from the ID.
func internalError(c *gin.Context, err error) {
	slog.Error("internal error", "request_id", GetRequestID(c), "method", c.Request.Method,
		"route", c.FullPath(), "error", err.Error())
	respondError(c, http.StatusInternalServerError, "internal server error")
}

// badRequestBody reports a body that could not be read or decoded, telling
// an oversized body (see the body size limit in main) apart from bad JSON.
func badRequestBody(c *gin.Context, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		respondError(c, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	respondError(c, http.StatusBadRequest, "invalid request body")
}

// Recovery turns a panic into a logged, structured 500. The stack is kept in
// one log record rather than gin's multi-line dump, so it ships as one entry.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "request_id", GetRequestID(c), "method", c.Request.Method,
			"path", c.Request.URL.Path, "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
		respondError(c, http.StatusInternalServerError, "internal server error")
	})
}

// NotFound and MethodNotAllowed replace gin's plain-text defaults.
func NotFound(c *gin.Context) {
	respondError(c, http.StatusNotFound, "not found")
}

func MethodNotAllowed(c *gin.Context) {
	respondError(c, http.StatusMethodNotAllowed, "method not allowed")
}

// AccessLog logs one structured record per request, keyed by request ID.
// Server errors are logged at error level so they can be filtered on.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.LogAttrs(c.Request.Context(), level, "request",
			slog.String("request_id", GetRequestID(c)),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("route", c.FullPath()),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("client_ip", c.ClientIP()),
		)
	}
}
