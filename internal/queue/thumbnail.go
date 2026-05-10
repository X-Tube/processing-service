package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

type ThumbnailUploadInput struct {
	ThumbnailID string
	Bucket      string
	Key         string
}

type ThumbnailProcessor struct{}

func NewThumbnailProcessor() *ThumbnailProcessor {
	return &ThumbnailProcessor{}
}

func (p *ThumbnailProcessor) Name() string {
	return "thumbnail"
}

func (p *ThumbnailProcessor) Process(ctx context.Context, body string) error {
	var event S3EventMessage

	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return err
	}

	if len(event.Records) == 0 {
		return fmt.Errorf("s3 event records are required")
	}

	for _, record := range event.Records {
		input, err := p.extractInput(record)
		if err != nil {
			return err
		}

		if err := p.processThumbnail(ctx, input); err != nil {
			return err
		}
	}

	return nil
}

func (p *ThumbnailProcessor) extractInput(record S3EventRecord) (ThumbnailUploadInput, error) {
	bucket := record.BucketName()
	if bucket == "" {
		return ThumbnailUploadInput{}, fmt.Errorf("bucket is required")
	}

	key, err := record.ObjectKey()
	if err != nil {
		return ThumbnailUploadInput{}, err
	}

	if key == "" {
		return ThumbnailUploadInput{}, fmt.Errorf("key is required")
	}

	thumbnailID := p.extractThumbnailID(key)
	if thumbnailID == "" {
		return ThumbnailUploadInput{}, fmt.Errorf("thumbnail id could not be extracted from key")
	}

	return ThumbnailUploadInput{
		ThumbnailID: thumbnailID,
		Bucket:      bucket,
		Key:         key,
	}, nil
}

func (p *ThumbnailProcessor) extractThumbnailID(key string) string {
	parts := strings.Split(key, "/")

	if len(parts) >= 3 && parts[0] == "thumbnails" {
		return parts[1]
	}

	fileName := path.Base(key)
	extension := path.Ext(fileName)

	return strings.TrimSuffix(fileName, extension)
}

func (p *ThumbnailProcessor) processThumbnail(ctx context.Context, input ThumbnailUploadInput) error {
	fmt.Printf("processing thumbnail: thumbnailId=%s bucket=%s key=%s\n", input.ThumbnailID, input.Bucket, input.Key)

	return nil
}