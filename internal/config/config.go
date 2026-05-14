package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AWS       AWS
	Buckets   Buckets
	Kafka     Kafka
	Logging   Logging
	Queues    Queues
	Worker    Worker
	Video     Video
	Thumbnail Thumbnail
}

type AWS struct {
	EndpointURL     string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

type Buckets struct {
	Input      string
	Output     string
	Thumbnails string
	Temp       string
}

type Kafka struct {
	Enabled            bool
	Brokers            []string
	VideoProgressTopic string
	ClientID           string
}

type Logging struct {
	Level          string
	ChunkDetails   bool
	FFmpegProgress bool
}

type Queues struct {
	VideoProcessing        string
	VideoProcessingDLQ     string
	ThumbnailProcessing    string
	WaitTimeSeconds        int32
	MaxMessages            int32
	ErrorDelay             time.Duration
	VideoVisibilityTimeout int32
	ThumbVisibilityTimeout int32
}

type Worker struct {
	VideoName     string
	ThumbnailName string
}

type Video struct {
	TempDir        string
	SegmentSeconds int
	Profiles       []VideoProfile
}

type VideoProfile struct {
	Name   string
	Height int
}

type Thumbnail struct {
	TempDir      string
	ResizeFactor int
}

func Load(_ context.Context) (Config, error) {
	cfg := Config{
		AWS: AWS{
			EndpointURL:     os.Getenv("AWS_ENDPOINT_URL"),
			Region:          envString("AWS_REGION", envString("AWS_DEFAULT_REGION", "us-east-1")),
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		},
		Buckets: Buckets{
			Input:      os.Getenv("S3_BUCKET_INPUT"),
			Output:     os.Getenv("S3_BUCKET_OUTPUT"),
			Thumbnails: os.Getenv("S3_BUCKET_THUMBNAILS"),
			Temp:       os.Getenv("S3_BUCKET_TEMP"),
		},
		Kafka: Kafka{
			Enabled:            envBool("KAFKA_ENABLED", true),
			Brokers:            envList("KAFKA_BROKERS", []string{"localhost:9092"}),
			VideoProgressTopic: envString("KAFKA_VIDEO_PROGRESS_TOPIC", "xtube.video.progress"),
			ClientID:           envString("KAFKA_CLIENT_ID", "xtube-processing-service"),
		},
		Logging: Logging{
			Level:          envString("LOG_LEVEL", "info"),
			ChunkDetails:   envBool("LOG_CHUNK_DETAILS", false),
			FFmpegProgress: envBool("LOG_FFMPEG_PROGRESS", true),
		},
		Queues: Queues{
			VideoProcessing:        os.Getenv("SQS_VIDEO_PROCESSING_URL"),
			VideoProcessingDLQ:     os.Getenv("SQS_VIDEO_PROCESSING_DLQ_URL"),
			ThumbnailProcessing:    os.Getenv("SQS_THUMBNAIL_PROCESSING_URL"),
			WaitTimeSeconds:        int32(envInt("SQS_WAIT_TIME_SECONDS", 20)),
			MaxMessages:            int32(envInt("SQS_MAX_MESSAGES", 10)),
			ErrorDelay:             time.Duration(envInt("SQS_ERROR_DELAY_SECONDS", 2)) * time.Second,
			VideoVisibilityTimeout: int32(envInt("SQS_VIDEO_VISIBILITY_TIMEOUT_SECONDS", 3600)),
			ThumbVisibilityTimeout: int32(envInt("SQS_THUMBNAIL_VISIBILITY_TIMEOUT_SECONDS", 600)),
		},
		Worker: Worker{
			VideoName:     envString("VIDEO_PROCESSOR_NAME", "video"),
			ThumbnailName: envString("THUMBNAIL_PROCESSOR_NAME", "thumbnail"),
		},
		Video: Video{
			TempDir:        envString("VIDEO_TEMP_DIR", ""),
			SegmentSeconds: envInt("VIDEO_SEGMENT_SECONDS", 10),
			Profiles: []VideoProfile{
				{Name: "360p", Height: 360},
				{Name: "480p", Height: 480},
				{Name: "720p", Height: 720},
			},
		},
		Thumbnail: Thumbnail{
			TempDir:      envString("THUMBNAIL_TEMP_DIR", ""),
			ResizeFactor: envInt("THUMBNAIL_RESIZE_FACTOR", 3),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	required := map[string]string{
		"SQS_VIDEO_PROCESSING_URL":     c.Queues.VideoProcessing,
		"SQS_THUMBNAIL_PROCESSING_URL": c.Queues.ThumbnailProcessing,
		"S3_BUCKET_INPUT":              c.Buckets.Input,
		"S3_BUCKET_OUTPUT":             c.Buckets.Output,
	}

	for name, value := range required {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}

	return nil
}

func envList(name string, fallback []string) []string {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	if len(result) == 0 {
		return fallback
	}

	return result
}

func envString(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
