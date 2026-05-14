package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/X-Tube/processing-service/internal/events"
	"github.com/X-Tube/processing-service/internal/messaging"
	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/X-Tube/processing-service/internal/processor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type Config struct {
	MaxNumberOfMessages     int32
	WaitTimeSeconds         int32
	VisibilityTimeout       int32
	VisibilityRenewInterval time.Duration
	ErrorDelay              time.Duration
}

type Worker struct {
	client    messaging.SQSClient
	queueURL  string
	config    Config
	processor processor.Processor
	logger    *slog.Logger
	hasDLQ    bool
}

func New(client messaging.SQSClient, queueURL string, processor processor.Processor, config Config, logger *slog.Logger) *Worker {
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

	w.hasDLQ = w.hasRedrivePolicy(ctx)

	w.logger.Info(
		"sqs worker started",
		"component", "worker",
		"worker", queueName,
		"queue", queueName,
		"visibility_timeout_seconds", w.config.VisibilityTimeout,
		"wait_time_seconds", w.config.WaitTimeSeconds,
		"max_messages", w.config.MaxNumberOfMessages,
		"redrive_policy_enabled", w.hasDLQ,
	)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("sqs worker stopped", "component", "worker", "worker", queueName, "queue", queueName)
			return

		default:
			if err := w.poll(ctx); err != nil {
				w.logger.Error("sqs polling error", "component", "worker", "worker", queueName, "queue", queueName, "error", err)
				time.Sleep(w.config.ErrorDelay)
			}
		}
	}
}

func (w *Worker) poll(ctx context.Context) error {
	queueName := w.processor.Name()
	startedAt := time.Now()

	w.logger.Debug("sqs polling started", "component", "worker", "worker", queueName, "queue", queueName, "wait_time_seconds", w.config.WaitTimeSeconds)

	output, err := w.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(w.queueURL),
		MaxNumberOfMessages: w.config.MaxNumberOfMessages,
		WaitTimeSeconds:     w.config.WaitTimeSeconds,
		VisibilityTimeout:   w.config.VisibilityTimeout,
		AttributeNames:      []types.QueueAttributeName{types.QueueAttributeNameAll},
	})

	if err != nil {
		return err
	}

	w.logger.Debug(
		"sqs polling finished",
		"component", "worker",
		"worker", queueName,
		"queue", queueName,
		"messages", len(output.Messages),
		"duration_ms", observability.DurationMillis(startedAt),
	)

	if len(output.Messages) == 0 {
		return nil
	}

	observability.SQSMessagesReceived.WithLabelValues(queueName).Add(float64(len(output.Messages)))

	for _, message := range output.Messages {
		if message.Body == nil || message.ReceiptHandle == nil {
			continue
		}

		messageStartedAt := time.Now()
		messageID := aws.ToString(message.MessageId)
		receiptHandle := aws.ToString(message.ReceiptHandle)
		summary := events.SummarizeS3Message(*message.Body)

		logAttrs := []any{
			"component", "worker",
			"worker", queueName,
			"queue", queueName,
			"message_id", messageID,
			"receipt_handle", messaging.MaskReceiptHandle(receiptHandle),
			"body_size", len(*message.Body),
			"records", summary.RecordCount,
			"event_source", summary.EventSource,
			"event_name", summary.EventName,
			"bucket", summary.Bucket,
			"key", summary.Key,
		}

		w.logger.Info("sqs message received", logAttrs...)
		w.logger.Debug("sqs message processing started", logAttrs...)

		stopRenewal := w.startVisibilityRenewal(ctx, messageID, receiptHandle, queueName)

		if err := w.processor.Process(ctx, *message.Body); err != nil {
			stopRenewal()

			observability.SQSMessagesProcessed.WithLabelValues(queueName, "error").Inc()
			observability.ObserveDuration(observability.SQSProcessingDuration, queueName, messageStartedAt)

			w.logger.Error(
				"sqs message processing error",
				withAttrs(logAttrs,
					"duration_ms", observability.DurationMillis(messageStartedAt),
					"error", err,
					"dlq_handled_by_redrive_policy", w.hasDLQ,
				)...,
			)

			continue
		}
		stopRenewal()

		observability.SQSMessagesProcessed.WithLabelValues(queueName, "success").Inc()
		observability.ObserveDuration(observability.SQSProcessingDuration, queueName, messageStartedAt)

		w.logger.Info("sqs message processing finished", withAttrs(logAttrs, "duration_ms", observability.DurationMillis(messageStartedAt))...)
		w.logger.Debug("sqs message delete started", logAttrs...)

		_, err := w.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(w.queueURL),
			ReceiptHandle: message.ReceiptHandle,
		})

		if err != nil {
			w.logger.Error("sqs message delete failed", withAttrs(logAttrs, "duration_ms", observability.DurationMillis(messageStartedAt), "error", err)...)
			continue
		}

		observability.SQSMessagesDeleted.WithLabelValues(queueName).Inc()

		w.logger.Info("sqs message deleted", withAttrs(logAttrs, "duration_ms", observability.DurationMillis(messageStartedAt))...)
	}

	return nil
}
