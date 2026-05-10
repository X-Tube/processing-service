package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/X-Tube/processing-service/internal/config"
	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/X-Tube/processing-service/internal/queue"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := observability.NewLogger()
	observability.StartServer(ctx, logger, ":9090")

	videoQueueURL := os.Getenv("SQS_VIDEO_PROCESSING_URL")
	if videoQueueURL == "" {
		log.Fatal("SQS_VIDEO_PROCESSING_URL is required")
	}

	thumbnailQueueURL := os.Getenv("SQS_THUMBNAIL_PROCESSING_URL")
	if thumbnailQueueURL == "" {
		log.Fatal("SQS_THUMBNAIL_PROCESSING_URL is required")
	}

	cfg, err := config.LoadAWSConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	sqsClient := config.NewSQSClient(cfg)

	videoWorker := queue.NewWorker(
		sqsClient,
		videoQueueURL,
		queue.NewVideoProcessor(),
		queue.WorkerConfig{
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   300,
			ErrorDelay:          2 * time.Second,
		},
		logger,
	)

	thumbnailWorker := queue.NewWorker(
		sqsClient,
		thumbnailQueueURL,
		queue.NewThumbnailProcessor(),
		queue.WorkerConfig{
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   120,
			ErrorDelay:          2 * time.Second,
		},
		logger,
	)

	go videoWorker.Start(ctx)
	go thumbnailWorker.Start(ctx)

	<-ctx.Done()
}
