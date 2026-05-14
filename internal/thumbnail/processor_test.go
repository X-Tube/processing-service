package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestProcessorResizesThumbnail(t *testing.T) {
	store := &fakeObjectStore{source: jpegImage(t, 1920, 1080)}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("xtube-thumbnails", "uploads/maxresdefault.jpg"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.downloadBucket != "xtube-thumbnails" {
		t.Fatalf("expected download bucket xtube-thumbnails, got %q", store.downloadBucket)
	}
	if store.downloadKey != "uploads/maxresdefault.jpg" {
		t.Fatalf("expected download key, got %q", store.downloadKey)
	}
	if len(store.uploads) != 2 {
		t.Fatalf("expected two uploads, got %d", len(store.uploads))
	}
	if store.uploads[0].bucket != "xtube-thumbnails" || store.uploads[0].key != "processed/maxresdefault/original.jpg" {
		t.Fatalf("expected original upload to processed/maxresdefault/original.jpg, got %#v", store.uploads[0])
	}
	if store.uploads[1].bucket != "xtube-thumbnails" || store.uploads[1].key != "processed/maxresdefault/3x.jpg" {
		t.Fatalf("expected resized upload to processed/maxresdefault/3x.jpg, got %#v", store.uploads[1])
	}
	if store.uploads[0].width != 1920 || store.uploads[0].height != 1080 {
		t.Fatalf("expected original dimensions 1920x1080, got %dx%d", store.uploads[0].width, store.uploads[0].height)
	}
	if store.uploads[1].width != 640 || store.uploads[1].height != 360 {
		t.Fatalf("expected resized dimensions 640x360, got %dx%d", store.uploads[1].width, store.uploads[1].height)
	}
}

func TestProcessorKeepsMinimumDimensions(t *testing.T) {
	store := &fakeObjectStore{source: jpegImage(t, 1, 1)}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("xtube-thumbnails", "uploads/tiny.jpg"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.uploads[1].width != 1 || store.uploads[1].height != 1 {
		t.Fatalf("expected resized dimensions 1x1, got %dx%d", store.uploads[1].width, store.uploads[1].height)
	}
}

func TestProcessorRejectsProcessedPrefix(t *testing.T) {
	store := &fakeObjectStore{source: jpegImage(t, 30, 30)}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("xtube-thumbnails", "processed/maxresdefault/original.jpg"))
	if err == nil || !strings.Contains(err.Error(), "uploads/ prefix") {
		t.Fatalf("expected uploads prefix error, got %v", err)
	}

	if store.downloadKey != "" {
		t.Fatalf("expected no download, got key %q", store.downloadKey)
	}
}

func TestProcessorRejectsInvalidImage(t *testing.T) {
	store := &fakeObjectStore{source: []byte("not an image")}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("xtube-thumbnails", "uploads/maxresdefault.jpg"))
	if err == nil || !strings.Contains(err.Error(), "resize thumbnail") {
		t.Fatalf("expected resize error, got %v", err)
	}
}

func TestProcessorPropagatesDownloadError(t *testing.T) {
	store := &fakeObjectStore{downloadErr: errors.New("download failed")}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("xtube-thumbnails", "uploads/maxresdefault.jpg"))
	if err == nil || !strings.Contains(err.Error(), "download thumbnail") {
		t.Fatalf("expected download error, got %v", err)
	}
}

func TestProcessorPropagatesUploadError(t *testing.T) {
	store := &fakeObjectStore{source: jpegImage(t, 30, 30), uploadErr: errors.New("upload failed"), uploadErrAt: 2}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("xtube-thumbnails", "uploads/maxresdefault.jpg"))
	if err == nil || !strings.Contains(err.Error(), "upload reduced thumbnail") {
		t.Fatalf("expected upload error, got %v", err)
	}
}

func TestProcessorRejectsUnexpectedBucket(t *testing.T) {
	store := &fakeObjectStore{source: jpegImage(t, 30, 30)}
	processor := newTestProcessor(store)

	err := processor.Process(context.Background(), s3EventBody("other-bucket", "uploads/maxresdefault.jpg"))
	if err == nil || !strings.Contains(err.Error(), "unexpected thumbnail bucket") {
		t.Fatalf("expected bucket error, got %v", err)
	}
}

func newTestProcessor(store *fakeObjectStore) *Processor {
	return NewProcessor(
		ProcessorConfig{
			Name:         "thumbnail",
			Bucket:       "xtube-thumbnails",
			ResizeFactor: 3,
		},
		store,
		discardLogger(),
	)
}

func s3EventBody(bucket, key string) string {
	return fmt.Sprintf(`{"Records":[{"s3":{"bucket":{"name":%q},"object":{"key":%q}}}]}`, bucket, key)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func jpegImage(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	return buf.Bytes()
}

type fakeObjectStore struct {
	source         []byte
	downloadBucket string
	downloadKey    string
	uploads        []fakeUpload
	downloadErr    error
	uploadErr      error
	uploadErrAt    int
}

type fakeUpload struct {
	bucket string
	key    string
	width  int
	height int
}

func (s *fakeObjectStore) DownloadToFile(_ context.Context, bucket, key, dstPath string) error {
	s.downloadBucket = bucket
	s.downloadKey = key

	if s.downloadErr != nil {
		return s.downloadErr
	}

	return os.WriteFile(dstPath, s.source, 0o600)
}

func (s *fakeObjectStore) UploadFile(_ context.Context, bucket, key, filePath string) error {
	if s.uploadErr != nil && (s.uploadErrAt == 0 || len(s.uploads)+1 == s.uploadErrAt) {
		return s.uploadErr
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	s.uploads = append(s.uploads, fakeUpload{
		bucket: bucket,
		key:    key,
		width:  bounds.Dx(),
		height: bounds.Dy(),
	})

	return nil
}
