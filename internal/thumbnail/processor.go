package thumbnail

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/X-Tube/processing-service/internal/events"
	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/X-Tube/processing-service/internal/storage"
)

type UploadInput struct {
	ThumbnailID string
	Bucket      string
	Key         string
	BaseName    string
	Extension   string
}

type ProcessorConfig struct {
	Name         string
	Bucket       string
	TempDir      string
	ResizeFactor int
}

type Processor struct {
	config ProcessorConfig
	store  storage.ObjectStore
	logger *slog.Logger
}

func NewProcessor(config ProcessorConfig, store storage.ObjectStore, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}

	return &Processor{
		config: config,
		store:  store,
		logger: logger,
	}
}

func (p *Processor) Name() string {
	if p.config.Name == "" {
		return "thumbnail"
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
			"thumbnail s3 event extracted",
			"component", "processor",
			"worker", p.Name(),
			"event_source", record.EventSource,
			"event_name", record.EventName,
			"bucket", input.Bucket,
			"key", input.Key,
			"thumbnail_id", input.ThumbnailID,
		)

		if err := p.processThumbnail(ctx, input); err != nil {
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

	baseName, extension, err := extractUploadName(key)
	if err != nil {
		return UploadInput{}, err
	}

	if baseName == "" {
		return UploadInput{}, fmt.Errorf("thumbnail id could not be extracted from key")
	}

	return UploadInput{
		ThumbnailID: baseName,
		Bucket:      bucket,
		Key:         key,
		BaseName:    baseName,
		Extension:   extension,
	}, nil
}

func extractUploadName(key string) (string, string, error) {
	if !strings.HasPrefix(key, "uploads/") {
		return "", "", fmt.Errorf("thumbnail key must be in uploads/ prefix: %s", key)
	}

	fileName := path.Base(key)
	if fileName == "." || fileName == "/" || fileName == "uploads" {
		return "", "", fmt.Errorf("thumbnail filename is required")
	}

	extension := path.Ext(fileName)
	if extension == "" {
		return "", "", fmt.Errorf("thumbnail extension is required")
	}

	return strings.TrimSuffix(fileName, extension), strings.ToLower(extension), nil
}

func (p *Processor) processThumbnail(ctx context.Context, input UploadInput) error {
	startedAt := time.Now()

	if strings.TrimSpace(input.Bucket) == "" {
		return fmt.Errorf("bucket is required")
	}
	if strings.TrimSpace(input.Key) == "" {
		return fmt.Errorf("key is required")
	}
	if strings.TrimSpace(input.ThumbnailID) == "" {
		return fmt.Errorf("thumbnail id is required")
	}
	if p.config.Bucket != "" && input.Bucket != p.config.Bucket {
		return fmt.Errorf("unexpected thumbnail bucket %q, expected %q", input.Bucket, p.config.Bucket)
	}
	if p.store == nil {
		return fmt.Errorf("thumbnail object store is required")
	}

	resizeFactor := p.config.ResizeFactor
	if resizeFactor <= 0 {
		resizeFactor = 3
	}

	tempDir, err := os.MkdirTemp(p.config.TempDir, "xtube-thumbnail-*")
	if err != nil {
		return fmt.Errorf("create temporary thumbnail directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "original"+input.Extension)
	outputPath := filepath.Join(tempDir, "3x"+input.Extension)
	originalKey := p.originalOutputKey(input)
	resizedKey := p.resizedOutputKey(input)

	p.logger.Info(
		"thumbnail processing started",
		"component", "processor",
		"worker", p.Name(),
		"thumbnail_id", input.ThumbnailID,
		"basename", input.BaseName,
		"bucket", input.Bucket,
		"key", input.Key,
		"original_key", originalKey,
		"resized_key", resizedKey,
		"resize_factor", resizeFactor,
	)
	p.logger.Debug("thumbnail temp paths prepared", "component", "processor", "worker", p.Name(), "thumbnail_id", input.ThumbnailID, "temp_dir", tempDir, "input_path", inputPath, "output_path", outputPath)

	if err := p.store.DownloadToFile(ctx, input.Bucket, input.Key, inputPath); err != nil {
		p.logger.Error("thumbnail download failed", "component", "processor", "worker", p.Name(), "thumbnail_id", input.ThumbnailID, "bucket", input.Bucket, "key", input.Key, "error", err)
		return fmt.Errorf("download thumbnail: %w", err)
	}

	resizeStartedAt := time.Now()
	p.logger.Info("thumbnail resize started", "component", "processor", "worker", p.Name(), "thumbnail_id", input.ThumbnailID, "basename", input.BaseName)
	original, resized, err := resizeImage(inputPath, outputPath, resizeFactor, input.Extension)
	if err != nil {
		p.logger.Error("thumbnail resize failed", "component", "processor", "worker", p.Name(), "thumbnail_id", input.ThumbnailID, "basename", input.BaseName, "duration_ms", observability.DurationMillis(resizeStartedAt), "error", err)
		return fmt.Errorf("resize thumbnail: %w", err)
	}
	p.logger.Info(
		"thumbnail resize finished",
		"component", "processor",
		"worker", p.Name(),
		"thumbnail_id", input.ThumbnailID,
		"basename", input.BaseName,
		"original_width", original.Width,
		"original_height", original.Height,
		"resized_width", resized.Width,
		"resized_height", resized.Height,
		"duration_ms", observability.DurationMillis(resizeStartedAt),
	)

	originalUploadStartedAt := time.Now()
	if err := p.store.UploadFile(ctx, input.Bucket, originalKey, inputPath); err != nil {
		p.logger.Error("thumbnail original upload failed", "component", "processor", "worker", p.Name(), "thumbnail_id", input.ThumbnailID, "basename", input.BaseName, "bucket", input.Bucket, "output_key", originalKey, "duration_ms", observability.DurationMillis(originalUploadStartedAt), "error", err)
		return fmt.Errorf("upload original thumbnail %s: %w", originalKey, err)
	}

	uploadStartedAt := time.Now()
	if err := p.store.UploadFile(ctx, input.Bucket, resizedKey, outputPath); err != nil {
		p.logger.Error("thumbnail resized upload failed", "component", "processor", "worker", p.Name(), "thumbnail_id", input.ThumbnailID, "basename", input.BaseName, "bucket", input.Bucket, "output_key", resizedKey, "duration_ms", observability.DurationMillis(uploadStartedAt), "error", err)
		return fmt.Errorf("upload reduced thumbnail %s: %w", resizedKey, err)
	}

	p.logger.Info(
		"thumbnail processing finished",
		"component", "processor",
		"worker", p.Name(),
		"thumbnail_id", input.ThumbnailID,
		"basename", input.BaseName,
		"bucket", input.Bucket,
		"key", input.Key,
		"original_key", originalKey,
		"resized_key", resizedKey,
		"original_width", original.Width,
		"original_height", original.Height,
		"resized_width", resized.Width,
		"resized_height", resized.Height,
		"duration_ms", observability.DurationMillis(startedAt),
	)

	return nil
}

func (p *Processor) originalOutputKey(input UploadInput) string {
	return path.Join("processed", input.BaseName, "original"+input.Extension)
}

func (p *Processor) resizedOutputKey(input UploadInput) string {
	return path.Join("processed", input.BaseName, fmt.Sprintf("%dx%s", p.resizeFactor(), input.Extension))
}

func (p *Processor) resizeFactor() int {
	if p.config.ResizeFactor <= 0 {
		return 3
	}

	return p.config.ResizeFactor
}

type dimensions struct {
	Width  int
	Height int
}

func resizeImage(inputPath, outputPath string, resizeFactor int, extension string) (dimensions, dimensions, error) {
	input, err := os.Open(inputPath)
	if err != nil {
		return dimensions{}, dimensions{}, fmt.Errorf("open original thumbnail: %w", err)
	}
	defer input.Close()

	source, _, err := image.Decode(input)
	if err != nil {
		return dimensions{}, dimensions{}, fmt.Errorf("decode original thumbnail: %w", err)
	}

	bounds := source.Bounds()
	original := dimensions{
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
	}
	if original.Width <= 0 || original.Height <= 0 {
		return dimensions{}, dimensions{}, fmt.Errorf("invalid original thumbnail dimensions %dx%d", original.Width, original.Height)
	}

	resized := dimensions{
		Width:  maxInt(1, int(math.Round(float64(original.Width)/float64(resizeFactor)))),
		Height: maxInt(1, int(math.Round(float64(original.Height)/float64(resizeFactor)))),
	}

	destination := image.NewRGBA(image.Rect(0, 0, resized.Width, resized.Height))
	for y := 0; y < resized.Height; y++ {
		srcY := bounds.Min.Y + minInt(original.Height-1, int(float64(y)*float64(original.Height)/float64(resized.Height)))
		for x := 0; x < resized.Width; x++ {
			srcX := bounds.Min.X + minInt(original.Width-1, int(float64(x)*float64(original.Width)/float64(resized.Width)))
			destination.Set(x, y, color.RGBAModel.Convert(source.At(srcX, srcY)))
		}
	}

	output, err := os.Create(outputPath)
	if err != nil {
		return dimensions{}, dimensions{}, fmt.Errorf("create reduced thumbnail: %w", err)
	}
	defer output.Close()

	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(output, destination, &jpeg.Options{Quality: 90}); err != nil {
			return dimensions{}, dimensions{}, fmt.Errorf("encode reduced jpeg thumbnail: %w", err)
		}
	case ".png":
		if err := png.Encode(output, destination); err != nil {
			return dimensions{}, dimensions{}, fmt.Errorf("encode reduced png thumbnail: %w", err)
		}
	case ".gif":
		if err := gif.Encode(output, destination, nil); err != nil {
			return dimensions{}, dimensions{}, fmt.Errorf("encode reduced gif thumbnail: %w", err)
		}
	default:
		return dimensions{}, dimensions{}, fmt.Errorf("unsupported thumbnail extension %q", extension)
	}

	return original, resized, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
