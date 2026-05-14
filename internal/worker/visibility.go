package worker

import (
	"context"
	"time"

	"github.com/X-Tube/processing-service/internal/messaging"
	"github.com/X-Tube/processing-service/internal/observability"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func (w *Worker) startVisibilityRenewal(ctx context.Context, messageID, receiptHandle, queueName string) func() {
	interval := w.config.VisibilityRenewInterval
	if interval <= 0 {
		interval = time.Duration(w.config.VisibilityTimeout/2) * time.Second
	}
	if interval <= 0 || w.config.VisibilityTimeout <= 1 {
		return func() {}
	}

	renewCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				startedAt := time.Now()
				_, err := w.client.ChangeMessageVisibility(renewCtx, &sqs.ChangeMessageVisibilityInput{
					QueueUrl:          aws.String(w.queueURL),
					ReceiptHandle:     aws.String(receiptHandle),
					VisibilityTimeout: w.config.VisibilityTimeout,
				})
				if err != nil {
					w.logger.Warn(
						"sqs message visibility renewal failed",
						"component", "worker",
						"worker", queueName,
						"queue", queueName,
						"message_id", messageID,
						"receipt_handle", messaging.MaskReceiptHandle(receiptHandle),
						"visibility_timeout_seconds", w.config.VisibilityTimeout,
						"error", err,
					)
					continue
				}

				w.logger.Debug(
					"sqs message visibility renewed",
					"component", "worker",
					"worker", queueName,
					"queue", queueName,
					"message_id", messageID,
					"receipt_handle", messaging.MaskReceiptHandle(receiptHandle),
					"visibility_timeout_seconds", w.config.VisibilityTimeout,
					"duration_ms", observability.DurationMillis(startedAt),
				)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func (w *Worker) hasRedrivePolicy(ctx context.Context) bool {
	output, err := w.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(w.queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameRedrivePolicy},
	})
	if err != nil {
		queueName := w.processor.Name()
		w.logger.Warn("sqs queue attributes error", "component", "worker", "worker", queueName, "queue", queueName, "error", err)
		return false
	}

	return output.Attributes["RedrivePolicy"] != ""
}
