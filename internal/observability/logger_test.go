package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerRespectsConfiguredLevel(t *testing.T) {
	var infoBuffer bytes.Buffer
	infoLogger := NewLoggerWithWriter(LoggerConfig{Level: "info"}, &infoBuffer)

	infoLogger.DebugContext(context.Background(), "debug hidden")
	infoLogger.InfoContext(context.Background(), "info visible")

	output := infoBuffer.String()
	if strings.Contains(output, "debug hidden") {
		t.Fatalf("expected debug log to be suppressed at info level, got %s", output)
	}
	if !strings.Contains(output, "info visible") {
		t.Fatalf("expected info log to be emitted, got %s", output)
	}

	var debugBuffer bytes.Buffer
	debugLogger := NewLoggerWithWriter(LoggerConfig{Level: "debug"}, &debugBuffer)
	debugLogger.DebugContext(context.Background(), "debug visible", slog.String("component", "test"))

	if !strings.Contains(debugBuffer.String(), "debug visible") {
		t.Fatalf("expected debug log to be emitted, got %s", debugBuffer.String())
	}
}
