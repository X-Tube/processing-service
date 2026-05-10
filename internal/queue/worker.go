package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type WorkerConfig struct {
	MaxNumberOfMessages int32
	WaitTimeSeconds    int32
	VisibilityTimeout  int32
	ErrorDelay          time.Duration
}

type Worker struct {
	client    *sqs.Client
	queueURL  string
	config    WorkerConfig
	processor MessageProcessor
	logger    *slog.Logger
}

func NewWorker(client *sqs.Client, queueURL string, processor MessageProcessor, config WorkerConfig, logger *slog.Logger) *Worker {
	return &Worker{
		client:    client,
		queueURL:  queueURL,
		config:    config,
		processor: processor,
		logger:    logger,
	}
}

func (w *Worker) Start(ctx context.Context) {
	queueName := w.processor.Name()

	w.logger.Info("sqs worker started", "queue", queueName)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("sqs worker stopped", "queue", queueName)
			return

		default:
			if err := w.poll(ctx); err != nil {
				w.logger.Error("sqs polling error", "queue", queueName, "error", err)
				time.Sleep(w.config.ErrorDelay)
			}
		}
	}
}

func (w *Worker) poll(ctx context.Context) error {
	queueName := w.processor.Name()

	output, err := w.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(w.queueURL),
		MaxNumberOfMessages: w.config.MaxNumberOfMessages,
		WaitTimeSeconds:     w.config.WaitTimeSeconds,
		VisibilityTimeout:   w.config.VisibilityTimeout,
	})

	if err != nil {
		return err
	}

	if len(output.Messages) == 0 {
		return nil
	}

	observability.SQSMessagesReceived.WithLabelValues(queueName).Add(float64(len(output.Messages)))

	for _, message := range output.Messages {
		if message.Body == nil || message.ReceiptHandle == nil {
			continue
		}

		startedAt := time.Now()

		if err := w.processor.Process(ctx, *message.Body); err != nil {
			observability.SQSMessagesProcessed.WithLabelValues(queueName, "error").Inc()
			observability.ObserveDuration(observability.SQSProcessingDuration, queueName, startedAt)

			w.logger.Error("sqs message processing error", "queue", queueName, "error", err)

			continue
		}

		observability.SQSMessagesProcessed.WithLabelValues(queueName, "success").Inc()
		observability.ObserveDuration(observability.SQSProcessingDuration, queueName, startedAt)

		_, err := w.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(w.queueURL),
			ReceiptHandle: message.ReceiptHandle,
		})

		if err != nil {
			w.logger.Error("sqs delete message error", "queue", queueName, "error", err)
			continue
		}

		observability.SQSMessagesDeleted.WithLabelValues(queueName).Inc()

		w.logger.Info("sqs message processed and deleted", "queue", queueName)
	}

	return nil
}