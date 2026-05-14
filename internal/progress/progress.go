package progress

import "context"

type VideoProgressEvent struct {
	VideoID         string `json:"video_id"`
	ProgressPercent int    `json:"progress_percent"`
}

type Publisher interface {
	PublishVideoProgress(ctx context.Context, event VideoProgressEvent) error
}

type NoopPublisher struct{}

func (NoopPublisher) PublishVideoProgress(context.Context, VideoProgressEvent) error {
	return nil
}
