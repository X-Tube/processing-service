package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	appconfig "github.com/X-Tube/processing-service/internal/config"
	"github.com/X-Tube/processing-service/internal/queue"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		log.Fatal("SQS_QUEUE_URL is required")
	}

	cfg, err := appconfig.LoadAWSConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	sqsClient := appconfig.NewSQSClient(cfg)

	worker := queue.NewWorker(sqsClient, queueURL)
	worker.Start(ctx)
}