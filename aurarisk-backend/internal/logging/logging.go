// Package logging configures process-wide log output so every line can be
// shipped to a central store (Loki) and searched by request ID.
package logging

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	FormatText = "text"
	FormatJSON = "json"
)

// CheckFormat validates a LOG_FORMAT value.
func CheckFormat(format string) error {
	switch strings.ToLower(format) {
	case FormatText, FormatJSON:
		return nil
	default:
		return fmt.Errorf("LOG_FORMAT must be %q or %q, got %q", FormatText, FormatJSON, format)
	}
}

// Setup sends all logs to stderr and, if file is not empty, appends them to
// that file too (for a log shipper to tail). In JSON format every line is one
// JSON object, including lines written with the standard log package. The
// returned function closes the file.
func Setup(format, file string) (func() error, error) {
	if err := CheckFormat(format); err != nil {
		return nil, err
	}

	var out io.Writer = os.Stderr
	closeFn := func() error { return nil }
	if file != "" {
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			return nil, fmt.Errorf("LOG_FILE: %w", err)
		}
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
		if err != nil {
			return nil, fmt.Errorf("LOG_FILE: %w", err)
		}
		out = io.MultiWriter(os.Stderr, f)
		closeFn = f.Close
	}

	gin.DefaultWriter = out
	gin.DefaultErrorWriter = out
	if strings.ToLower(format) == FormatJSON {
		// SetDefault also routes the standard log package through this
		// handler, so existing log.Printf calls become JSON lines too.
		slog.SetDefault(slog.New(slog.NewJSONHandler(out, nil)))
	} else {
		log.SetOutput(out)
	}
	return closeFn, nil
}
