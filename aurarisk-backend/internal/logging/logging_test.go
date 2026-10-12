package logging

import (
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupJSONWritesStdlibLogsToFile(t *testing.T) {
	prevSlog, prevGin, prevGinErr := slog.Default(), gin.DefaultWriter, gin.DefaultErrorWriter
	t.Cleanup(func() {
		slog.SetDefault(prevSlog)
		gin.DefaultWriter, gin.DefaultErrorWriter = prevGin, prevGinErr
		log.SetOutput(os.Stderr)
	})

	file := filepath.Join(t.TempDir(), "logs", "backend.log")
	closeLog, err := Setup("json", file)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("legacy line %d", 1)
	slog.Info("structured", "request_id", "abc")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), data)
	}
	var first, second map[string]any
	if json.Unmarshal([]byte(lines[0]), &first) != nil || json.Unmarshal([]byte(lines[1]), &second) != nil {
		t.Fatalf("lines are not JSON: %q", data)
	}
	if first["msg"] != "legacy line 1" || second["request_id"] != "abc" {
		t.Errorf("records = %v, %v", first, second)
	}
}

func TestCheckFormat(t *testing.T) {
	for _, ok := range []string{"text", "json", "JSON"} {
		if err := CheckFormat(ok); err != nil {
			t.Errorf("CheckFormat(%q) = %v", ok, err)
		}
	}
	if err := CheckFormat("xml"); err == nil {
		t.Error("CheckFormat(xml) = nil")
	}
}
