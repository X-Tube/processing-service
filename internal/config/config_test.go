package config

import (
	"context"
	"testing"
)

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("SQS_VIDEO_PROCESSING_URL", "video-url")
	t.Setenv("SQS_THUMBNAIL_PROCESSING_URL", "thumbnail-url")
	t.Setenv("S3_BUCKET_INPUT", "input-bucket")
	t.Setenv("S3_BUCKET_OUTPUT", "output-bucket")
	t.Setenv("SQS_MAX_MESSAGES", "3")
	t.Setenv("SQS_VIDEO_VISIBILITY_TIMEOUT_SECONDS", "1800")
	t.Setenv("THUMBNAIL_RESIZE_FACTOR", "4")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_CHUNK_DETAILS", "true")
	t.Setenv("LOG_FFMPEG_PROGRESS", "false")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Queues.MaxMessages != 3 {
		t.Fatalf("expected max messages 3, got %d", cfg.Queues.MaxMessages)
	}
	if cfg.Queues.VideoVisibilityTimeout != 1800 {
		t.Fatalf("expected video visibility 1800, got %d", cfg.Queues.VideoVisibilityTimeout)
	}
	if cfg.Queues.WaitTimeSeconds != 20 {
		t.Fatalf("expected default wait time 20, got %d", cfg.Queues.WaitTimeSeconds)
	}
	if len(cfg.Video.Profiles) != 3 {
		t.Fatalf("expected three default profiles, got %d", len(cfg.Video.Profiles))
	}
	if cfg.Thumbnail.ResizeFactor != 4 {
		t.Fatalf("expected thumbnail resize factor 4, got %d", cfg.Thumbnail.ResizeFactor)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("expected log level debug, got %q", cfg.Logging.Level)
	}
	if !cfg.Logging.ChunkDetails {
		t.Fatalf("expected chunk details enabled")
	}
	if cfg.Logging.FFmpegProgress {
		t.Fatalf("expected ffmpeg progress disabled")
	}
}

func TestLoadLoggingDefaults(t *testing.T) {
	t.Setenv("SQS_VIDEO_PROCESSING_URL", "video-url")
	t.Setenv("SQS_THUMBNAIL_PROCESSING_URL", "thumbnail-url")
	t.Setenv("S3_BUCKET_INPUT", "input-bucket")
	t.Setenv("S3_BUCKET_OUTPUT", "output-bucket")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Logging.Level != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.ChunkDetails {
		t.Fatalf("expected chunk details disabled by default")
	}
	if !cfg.Logging.FFmpegProgress {
		t.Fatalf("expected ffmpeg progress enabled by default")
	}
}

func TestLoadRequiresCriticalValues(t *testing.T) {
	_, err := Load(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
}
