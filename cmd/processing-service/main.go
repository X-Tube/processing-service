package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/X-Tube/processing-service/internal/awsclient"
	"github.com/X-Tube/processing-service/internal/config"
	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/X-Tube/processing-service/internal/storage"
	"github.com/X-Tube/processing-service/internal/thumbnail"
	"github.com/X-Tube/processing-service/internal/video"
	"github.com/X-Tube/processing-service/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	appConfig, err := config.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}

	logger := observability.NewLogger(observability.LoggerConfig{Level: appConfig.Logging.Level})
	logger.Info("processing service started", "component", "service", "log_level", appConfig.Logging.Level)
	observability.StartServer(ctx, logger, ":9090")

	awsConfig, err := awsclient.LoadConfig(ctx, appConfig.AWS.Region)
	if err != nil {
		log.Fatal(err)
	}

	sqsClient := awsclient.NewSQSClient(awsConfig, appConfig.AWS.EndpointURL)
	s3Client := awsclient.NewS3Client(awsConfig, appConfig.AWS.EndpointURL)
	objectStore := storage.NewS3ObjectStore(s3Client)

	videoProcessor := video.NewProcessor(
		video.ProcessorConfig{
			Name:         appConfig.Worker.VideoName,
			InputBucket:  appConfig.Buckets.Input,
			OutputBucket: appConfig.Buckets.Output,
			TempDir:      appConfig.Video.TempDir,
			Profiles:     videoProfiles(appConfig.Video.Profiles),
			ChunkDetails: appConfig.Logging.ChunkDetails,
		},
		objectStore,
		video.NewFFmpegTranscoder(video.TranscoderConfig{
			SegmentSeconds: appConfig.Video.SegmentSeconds,
			ChunkDetails:   appConfig.Logging.ChunkDetails,
			FFmpegProgress: appConfig.Logging.FFmpegProgress,
			LogLevel:       appConfig.Logging.Level,
		}, logger),
		logger,
	)

	thumbnailProcessor := thumbnail.NewProcessor(
		thumbnail.ProcessorConfig{
			Name:         appConfig.Worker.ThumbnailName,
			Bucket:       appConfig.Buckets.Thumbnails,
			TempDir:      appConfig.Thumbnail.TempDir,
			ResizeFactor: appConfig.Thumbnail.ResizeFactor,
		},
		objectStore,
		logger,
	)

	videoWorker := worker.New(
		sqsClient,
		appConfig.Queues.VideoProcessing,
		videoProcessor,
		worker.Config{
			MaxNumberOfMessages: appConfig.Queues.MaxMessages,
			WaitTimeSeconds:     appConfig.Queues.WaitTimeSeconds,
			VisibilityTimeout:   appConfig.Queues.VideoVisibilityTimeout,
			ErrorDelay:          appConfig.Queues.ErrorDelay,
		},
		logger,
	)

	thumbnailWorker := worker.New(
		sqsClient,
		appConfig.Queues.ThumbnailProcessing,
		thumbnailProcessor,
		worker.Config{
			MaxNumberOfMessages: appConfig.Queues.MaxMessages,
			WaitTimeSeconds:     appConfig.Queues.WaitTimeSeconds,
			VisibilityTimeout:   appConfig.Queues.ThumbVisibilityTimeout,
			ErrorDelay:          appConfig.Queues.ErrorDelay,
		},
		logger,
	)

	go videoWorker.Start(ctx)
	go thumbnailWorker.Start(ctx)

	<-ctx.Done()
}

func videoProfiles(profiles []config.VideoProfile) []video.Profile {
	result := make([]video.Profile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, video.Profile{
			Name:   profile.Name,
			Height: profile.Height,
		})
	}

	return result
}
