package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

type VideoUploadInput struct {
	VideoID string
	Bucket  string
	Key     string
}

type VideoProcessor struct{}

func NewVideoProcessor() *VideoProcessor {
	return &VideoProcessor{}
}

func (p *VideoProcessor) Name() string {
	return "video"
}

func (p *VideoProcessor) Process(ctx context.Context, body string) error {
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

		if err := p.processVideo(ctx, input); err != nil {
			return err
		}
	}

	return nil
}

func (p *VideoProcessor) extractInput(record S3EventRecord) (VideoUploadInput, error) {
	bucket := record.BucketName()
	if bucket == "" {
		return VideoUploadInput{}, fmt.Errorf("bucket is required")
	}

	key, err := record.ObjectKey()
	if err != nil {
		return VideoUploadInput{}, err
	}

	if key == "" {
		return VideoUploadInput{}, fmt.Errorf("key is required")
	}

	videoID := p.extractVideoID(key)
	if videoID == "" {
		return VideoUploadInput{}, fmt.Errorf("video id could not be extracted from key")
	}

	return VideoUploadInput{
		VideoID: videoID,
		Bucket:  bucket,
		Key:     key,
	}, nil
}

func (p *VideoProcessor) extractVideoID(key string) string {
	parts := strings.Split(key, "/")

	if len(parts) >= 3 && parts[0] == "uploads" {
		return parts[1]
	}

	fileName := path.Base(key)
	extension := path.Ext(fileName)

	return strings.TrimSuffix(fileName, extension)
}

func (p *VideoProcessor) processVideo(ctx context.Context, input VideoUploadInput) error {
	fmt.Printf("processing video: videoId=%s bucket=%s key=%s\n", input.VideoID, input.Bucket, input.Key)

	return nil
}
