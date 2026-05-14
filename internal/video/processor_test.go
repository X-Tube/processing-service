package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/X-Tube/processing-service/internal/progress"
)

func TestProcessorExtractVideoID(t *testing.T) {
	processor := NewProcessor(ProcessorConfig{}, nil, nil, nil, discardLogger())

	tests := map[string]string{
		"uploads/video-123/original.mp4": "video-123",
		"plain-video.mp4":                "plain-video",
		"nested/plain-video.mov":         "plain-video",
	}

	for key, expected := range tests {
		t.Run(key, func(t *testing.T) {
			if got := processor.extractVideoID(key); got != expected {
				t.Fatalf("expected %q, got %q", expected, got)
			}
		})
	}
}

func TestProcessorRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing bucket", body: s3EventBody("", "uploads/video-123/original.mp4"), want: "bucket is required"},
		{name: "missing key", body: s3EventBody("xtube-videos-input", ""), want: "key is required"},
		{name: "missing video id", body: s3EventBody("xtube-videos-input", "/"), want: "video id could not be extracted"},
		{name: "unexpected bucket", body: s3EventBody("other-bucket", "uploads/video-123/original.mp4"), want: "unexpected input bucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := newTestProcessor(t, nil, nil)

			err := processor.Process(context.Background(), tt.body)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestProcessorProcessesAndUploadsSegments(t *testing.T) {
	store := &fakeObjectStore{}
	publisher := &fakeProgressPublisher{}
	segmenter := &fakeSegmenter{
		segments: []Segment{
			{Profile: "360p", Index: 1, FileName: "video-1.mp4"},
			{Profile: "360p", Index: 2, FileName: "video-2.mp4"},
			{Profile: "480p", Index: 1, FileName: "video-1.mp4"},
			{Profile: "720p", Index: 1, FileName: "video-1.mp4"},
		},
	}
	processor := newTestProcessorWithPublisher(t, store, segmenter, publisher)

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.downloadBucket != "xtube-videos-input" {
		t.Fatalf("expected downloader bucket xtube-videos-input, got %q", store.downloadBucket)
	}

	if store.downloadKey != "uploads/video-123/original.mp4" {
		t.Fatalf("expected downloader key, got %q", store.downloadKey)
	}

	if _, err := os.Stat(store.downloadPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temporary input to be removed, stat error was %v", err)
	}

	wantKeys := []string{
		"video-123/360p/video-1.mp4",
		"video-123/360p/video-2.mp4",
		"video-123/480p/video-1.mp4",
		"video-123/720p/video-1.mp4",
	}
	if !reflect.DeepEqual(store.uploadKeys, wantKeys) {
		t.Fatalf("expected upload keys %v, got %v", wantKeys, store.uploadKeys)
	}

	wantEvents := []progress.VideoProgressEvent{
		{VideoID: "video-123", ProgressPercent: 25},
		{VideoID: "video-123", ProgressPercent: 50},
		{VideoID: "video-123", ProgressPercent: 75},
		{VideoID: "video-123", ProgressPercent: 100},
	}
	if !reflect.DeepEqual(publisher.events, wantEvents) {
		t.Fatalf("expected progress events %v, got %v", wantEvents, publisher.events)
	}
}

func TestProcessorPropagatesDownloadError(t *testing.T) {
	processor := newTestProcessor(t, &fakeObjectStore{downloadErr: errors.New("download failed")}, nil)

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err == nil || !strings.Contains(err.Error(), "download input video") {
		t.Fatalf("expected download error, got %v", err)
	}
}

func TestProcessorPropagatesSegmentError(t *testing.T) {
	processor := newTestProcessor(t, &fakeObjectStore{}, &fakeSegmenter{err: errors.New("ffmpeg failed")})

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err == nil || !strings.Contains(err.Error(), "generate video segments") {
		t.Fatalf("expected segment error, got %v", err)
	}
}

func TestProcessorPropagatesUploadError(t *testing.T) {
	segmenter := &fakeSegmenter{segments: []Segment{{Profile: "360p", Index: 1, FileName: "video-1.mp4"}}}
	publisher := &fakeProgressPublisher{}
	processor := newTestProcessorWithPublisher(t, &fakeObjectStore{uploadErr: errors.New("upload failed")}, segmenter, publisher)

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err == nil || !strings.Contains(err.Error(), "upload video segment video-123/360p/video-1.mp4") {
		t.Fatalf("expected upload error, got %v", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no progress events after upload failure, got %v", publisher.events)
	}
}

func TestProcessorPropagatesProgressPublishError(t *testing.T) {
	segmenter := &fakeSegmenter{segments: []Segment{{Profile: "360p", Index: 1, FileName: "video-1.mp4"}}}
	processor := newTestProcessorWithPublisher(t, &fakeObjectStore{}, segmenter, &fakeProgressPublisher{err: errors.New("kafka failed")})

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err == nil || !strings.Contains(err.Error(), "publish video progress for video-123") {
		t.Fatalf("expected progress publish error, got %v", err)
	}
}

func TestProcessorProgressUsesFloorAndLastChunkIsOneHundred(t *testing.T) {
	publisher := &fakeProgressPublisher{}
	segmenter := &fakeSegmenter{
		segments: []Segment{
			{Profile: "360p", Index: 1, FileName: "video-1.mp4"},
			{Profile: "480p", Index: 1, FileName: "video-1.mp4"},
			{Profile: "720p", Index: 1, FileName: "video-1.mp4"},
		},
	}
	processor := newTestProcessorWithPublisher(t, &fakeObjectStore{}, segmenter, publisher)

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	wantEvents := []progress.VideoProgressEvent{
		{VideoID: "video-123", ProgressPercent: 33},
		{VideoID: "video-123", ProgressPercent: 66},
		{VideoID: "video-123", ProgressPercent: 100},
	}
	if !reflect.DeepEqual(publisher.events, wantEvents) {
		t.Fatalf("expected progress events %v, got %v", wantEvents, publisher.events)
	}
}

func TestProcessorRejectsEmptySegmentList(t *testing.T) {
	processor := newTestProcessor(t, &fakeObjectStore{}, &fakeSegmenter{})

	err := processor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err == nil || !strings.Contains(err.Error(), "no video segments generated") {
		t.Fatalf("expected empty segment error, got %v", err)
	}
}

func TestProcessorIgnoresS3TestEvent(t *testing.T) {
	store := &fakeObjectStore{}
	processor := newTestProcessor(t, store, nil)

	err := processor.Process(context.Background(), `{"Service":"Amazon S3","Event":"s3:TestEvent","Bucket":"xtube-videos-input"}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.downloadKey != "" {
		t.Fatalf("expected downloader not to be called, got key %q", store.downloadKey)
	}
}

func TestProcessorChunkLogsRequireDebugFlag(t *testing.T) {
	segmenter := &fakeSegmenter{segments: []Segment{{Profile: "360p", Index: 1, FileName: "video-1.mp4"}}}

	var quietBuffer bytes.Buffer
	quietLogger := slog.New(slog.NewJSONHandler(&quietBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	quietProcessor := newTestProcessorWithConfig(t, &fakeObjectStore{}, segmenter, ProcessorConfig{ChunkDetails: false}, quietLogger)

	if err := quietProcessor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4")); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if strings.Contains(quietBuffer.String(), "video chunk upload finished") {
		t.Fatalf("expected chunk logs to be omitted by default, got %s", quietBuffer.String())
	}

	var debugBuffer bytes.Buffer
	debugLogger := slog.New(slog.NewJSONHandler(&debugBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	debugProcessor := newTestProcessorWithConfig(t, &fakeObjectStore{}, segmenter, ProcessorConfig{ChunkDetails: true}, debugLogger)

	if err := debugProcessor.Process(context.Background(), s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4")); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(debugBuffer.String(), "video chunk upload finished") {
		t.Fatalf("expected chunk log when chunk details are enabled, got %s", debugBuffer.String())
	}
}

func newTestProcessor(t *testing.T, store *fakeObjectStore, segmenter *fakeSegmenter) *Processor {
	t.Helper()

	return newTestProcessorWithConfig(t, store, segmenter, ProcessorConfig{}, discardLogger())
}

func newTestProcessorWithPublisher(t *testing.T, store *fakeObjectStore, segmenter *fakeSegmenter, publisher progress.Publisher) *Processor {
	t.Helper()

	return newTestProcessorWithOptions(t, store, segmenter, publisher, ProcessorConfig{}, discardLogger())
}

func newTestProcessorWithConfig(t *testing.T, store *fakeObjectStore, segmenter *fakeSegmenter, overrides ProcessorConfig, logger *slog.Logger) *Processor {
	t.Helper()

	return newTestProcessorWithOptions(t, store, segmenter, nil, overrides, logger)
}

func newTestProcessorWithOptions(t *testing.T, store *fakeObjectStore, segmenter *fakeSegmenter, publisher progress.Publisher, overrides ProcessorConfig, logger *slog.Logger) *Processor {
	t.Helper()

	if store == nil {
		store = &fakeObjectStore{}
	}
	if segmenter == nil {
		segmenter = &fakeSegmenter{}
	}

	return NewProcessor(
		ProcessorConfig{
			Name:         "video",
			InputBucket:  "xtube-videos-input",
			OutputBucket: "xtube-videos-output",
			Profiles: []Profile{
				{Name: "360p", Height: 360},
				{Name: "480p", Height: 480},
				{Name: "720p", Height: 720},
			},
			ChunkDetails: overrides.ChunkDetails,
		},
		store,
		segmenter,
		publisher,
		logger,
	)
}

func s3EventBody(bucket, key string) string {
	return fmt.Sprintf(`{"Records":[{"s3":{"bucket":{"name":%q},"object":{"key":%q}}}]}`, bucket, key)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeObjectStore struct {
	downloadBucket string
	downloadKey    string
	downloadPath   string
	uploadBuckets  []string
	uploadKeys     []string
	downloadErr    error
	uploadErr      error
}

func (s *fakeObjectStore) DownloadToFile(_ context.Context, bucket, key, dstPath string) error {
	s.downloadBucket = bucket
	s.downloadKey = key
	s.downloadPath = dstPath

	if s.downloadErr != nil {
		return s.downloadErr
	}

	return os.WriteFile(dstPath, []byte("video"), 0o600)
}

func (s *fakeObjectStore) UploadFile(_ context.Context, bucket, key, filePath string) error {
	if _, err := os.Stat(filePath); err != nil {
		return err
	}

	s.uploadBuckets = append(s.uploadBuckets, bucket)
	s.uploadKeys = append(s.uploadKeys, key)

	return s.uploadErr
}

type fakeSegmenter struct {
	segments []Segment
	err      error
}

type fakeProgressPublisher struct {
	events []progress.VideoProgressEvent
	err    error
}

func (p *fakeProgressPublisher) PublishVideoProgress(_ context.Context, event progress.VideoProgressEvent) error {
	if p.err != nil {
		return p.err
	}

	p.events = append(p.events, event)

	return nil
}

func (s *fakeSegmenter) Segment(_ context.Context, _ string, outputDir string, _ []Profile) ([]Segment, error) {
	if s.err != nil {
		return nil, s.err
	}

	segments := make([]Segment, 0, len(s.segments))
	for _, segment := range s.segments {
		profileDir := filepath.Join(outputDir, segment.Profile)
		if err := os.MkdirAll(profileDir, 0o755); err != nil {
			return nil, err
		}

		segment.FilePath = filepath.Join(profileDir, segment.FileName)
		content := []byte("segment")
		if err := os.WriteFile(segment.FilePath, content, 0o600); err != nil {
			return nil, err
		}
		segment.Size = int64(len(content))

		segments = append(segments, segment)
	}

	return segments, nil
}
