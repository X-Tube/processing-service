package video

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/X-Tube/processing-service/internal/events"
	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/X-Tube/processing-service/internal/storage"
)

type Processor struct {
	config    ProcessorConfig
	store     storage.ObjectStore
	segmenter Segmenter
	logger    *slog.Logger
}

func NewProcessor(config ProcessorConfig, store storage.ObjectStore, segmenter Segmenter, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}

	return &Processor{
		config:    config,
		store:     store,
		segmenter: segmenter,
		logger:    logger,
	}
}

func (p *Processor) Name() string {
	if p.config.Name == "" {
		return "video"
	}

	return p.config.Name
}

func (p *Processor) Process(ctx context.Context, body string) error {
	event, ignored, err := events.ParseS3Message(body)
	if err != nil {
		return err
	}

	if ignored {
		return nil
	}

	for _, record := range event.Records {
		input, err := p.extractInput(record)
		if err != nil {
			return err
		}

		p.logger.Debug(
			"video s3 event extracted",
			"component", "processor",
			"worker", p.Name(),
			"event_source", record.EventSource,
			"event_name", record.EventName,
			"bucket", input.Bucket,
			"key", input.Key,
			"video_id", input.VideoID,
		)

		if err := p.processVideo(ctx, input); err != nil {
			return err
		}
	}

	return nil
}

func (p *Processor) extractInput(record events.S3EventRecord) (UploadInput, error) {
	bucket, key, err := events.RecordBucketKey(record)
	if err != nil {
		return UploadInput{}, err
	}

	if bucket == "" {
		return UploadInput{}, fmt.Errorf("bucket is required")
	}

	if key == "" {
		return UploadInput{}, fmt.Errorf("key is required")
	}

	videoID := p.extractVideoID(key)
	if videoID == "" {
		return UploadInput{}, fmt.Errorf("video id could not be extracted from key")
	}

	return UploadInput{
		VideoID: videoID,
		Bucket:  bucket,
		Key:     key,
	}, nil
}

func (p *Processor) extractVideoID(key string) string {
	parts := strings.Split(key, "/")

	if len(parts) >= 3 && parts[0] == "uploads" {
		return parts[1]
	}

	fileName := path.Base(key)
	if fileName == "." || fileName == "/" {
		return ""
	}

	extension := path.Ext(fileName)

	return strings.TrimSuffix(fileName, extension)
}

func (p *Processor) processVideo(ctx context.Context, input UploadInput) error {
	processStartedAt := time.Now()

	if strings.TrimSpace(p.config.InputBucket) == "" {
		return fmt.Errorf("input bucket config is required")
	}

	if strings.TrimSpace(p.config.OutputBucket) == "" {
		return fmt.Errorf("output bucket config is required")
	}

	if input.Bucket != p.config.InputBucket {
		return fmt.Errorf("unexpected input bucket %q, expected %q", input.Bucket, p.config.InputBucket)
	}

	if strings.TrimSpace(input.Key) == "" {
		return fmt.Errorf("key is required")
	}

	if strings.TrimSpace(input.VideoID) == "" {
		return fmt.Errorf("video id is required")
	}

	tempDir, err := os.MkdirTemp(p.config.TempDir, "xtube-video-*")
	if err != nil {
		return fmt.Errorf("create temporary processing directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input"+path.Ext(input.Key))
	downloadStartedAt := time.Now()
	p.logger.Info("video download started", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key)
	if err := p.store.DownloadToFile(ctx, input.Bucket, input.Key, inputPath); err != nil {
		p.logger.Error("video download failed", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key, "duration_ms", observability.DurationMillis(downloadStartedAt), "error", err)
		return fmt.Errorf("download input video: %w", err)
	}
	p.logger.Info("video download finished", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key, "duration_ms", observability.DurationMillis(downloadStartedAt))

	outputDir := filepath.Join(tempDir, "segments")
	segmentStartedAt := time.Now()
	p.logger.Info("video segmentation started", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key)
	segments, err := p.segmenter.Segment(ctx, inputPath, outputDir, p.config.Profiles)
	if err != nil {
		p.logger.Error("video segmentation failed", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key, "duration_ms", observability.DurationMillis(segmentStartedAt), "error", err)
		return fmt.Errorf("generate video segments: %w", err)
	}
	p.logger.Info("video segmentation finished", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key, "chunk_count", len(segments), "size_bytes", totalSegmentSize(segments), "duration_ms", observability.DurationMillis(segmentStartedAt))

	if len(segments) == 0 {
		return fmt.Errorf("no video segments generated")
	}

	uploadStartedAt := time.Now()
	var uploadedSize int64
	for _, segment := range segments {
		outputKey := p.outputKey(input.VideoID, segment)
		chunkStartedAt := time.Now()
		if p.config.ChunkDetails {
			p.logger.Debug("video chunk upload started", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "resolution", segment.Profile, "chunk_index", segment.Index, "bucket", p.config.OutputBucket, "output_key", outputKey, "size_bytes", segment.Size)
		}
		if err := p.store.UploadFile(ctx, p.config.OutputBucket, outputKey, segment.FilePath); err != nil {
			p.logger.Error("video chunk upload failed", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "resolution", segment.Profile, "chunk_index", segment.Index, "bucket", p.config.OutputBucket, "output_key", outputKey, "duration_ms", observability.DurationMillis(chunkStartedAt), "error", err)
			return fmt.Errorf("upload video segment %s: %w", outputKey, err)
		}
		uploadedSize += segment.Size
		if p.config.ChunkDetails {
			p.logger.Debug("video chunk upload finished", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "resolution", segment.Profile, "chunk_index", segment.Index, "bucket", p.config.OutputBucket, "output_key", outputKey, "size_bytes", segment.Size, "duration_ms", observability.DurationMillis(chunkStartedAt))
		}
	}

	p.logger.Info("video upload finished", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", p.config.OutputBucket, "chunk_count", len(segments), "size_bytes", uploadedSize, "duration_ms", observability.DurationMillis(uploadStartedAt))
	p.logger.Info("video processing finished", "component", "processor", "worker", p.Name(), "video_id", input.VideoID, "bucket", input.Bucket, "key", input.Key, "chunk_count", len(segments), "size_bytes", uploadedSize, "duration_ms", observability.DurationMillis(processStartedAt))

	return nil
}

func (p *Processor) outputKey(videoID string, segment Segment) string {
	return path.Join(videoID, segment.Profile, segment.FileName)
}

func totalSegmentSize(segments []Segment) int64 {
	var total int64
	for _, segment := range segments {
		total += segment.Size
	}

	return total
}
