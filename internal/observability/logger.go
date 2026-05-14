package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type LoggerConfig struct {
	Level string
}

func NewLogger(config LoggerConfig) *slog.Logger {
	return NewLoggerWithWriter(config, os.Stdout)
}

func NewLoggerWithWriter(config LoggerConfig, writer io.Writer) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(config.Level)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
	}))
}
