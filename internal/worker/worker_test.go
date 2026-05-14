package worker

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func TestWorkerRenewsVisibilityWhileProcessing(t *testing.T) {
	client := &fakeSQSClient{
		receiveOutput: &sqs.ReceiveMessageOutput{
			Messages: []types.Message{
				{
					MessageId:     aws.String("message-1"),
					ReceiptHandle: aws.String("receipt-handle-123456"),
					Body:          aws.String(s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4")),
				},
			},
		},
	}

	worker := New(
		client,
		"http://localhost/queue",
		slowProcessor{delay: 80 * time.Millisecond},
		Config{
			MaxNumberOfMessages:     1,
			WaitTimeSeconds:         1,
			VisibilityTimeout:       30,
			VisibilityRenewInterval: 10 * time.Millisecond,
			ErrorDelay:              time.Millisecond,
		},
		discardLogger(),
	)

	if err := worker.poll(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := atomic.LoadInt32(&client.changeVisibilityCalls); got == 0 {
		t.Fatalf("expected visibility renewal to be called")
	}

	if got := atomic.LoadInt32(&client.deleteCalls); got != 1 {
		t.Fatalf("expected one delete call, got %d", got)
	}
}

type slowProcessor struct {
	delay time.Duration
}

func (p slowProcessor) Name() string {
	return "video"
}

func (p slowProcessor) Process(ctx context.Context, _ string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(p.delay):
		return nil
	}
}

type fakeSQSClient struct {
	receiveOutput         *sqs.ReceiveMessageOutput
	changeVisibilityCalls int32
	deleteCalls           int32
}

func (c *fakeSQSClient) ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return c.receiveOutput, nil
}

func (c *fakeSQSClient) DeleteMessage(context.Context, *sqs.DeleteMessageInput, ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	atomic.AddInt32(&c.deleteCalls, 1)
	return &sqs.DeleteMessageOutput{}, nil
}

func (c *fakeSQSClient) ChangeMessageVisibility(context.Context, *sqs.ChangeMessageVisibilityInput, ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error) {
	atomic.AddInt32(&c.changeVisibilityCalls, 1)
	return &sqs.ChangeMessageVisibilityOutput{}, nil
}

func (c *fakeSQSClient) GetQueueAttributes(context.Context, *sqs.GetQueueAttributesInput, ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return &sqs.GetQueueAttributesOutput{
		Attributes: map[string]string{"RedrivePolicy": `{"maxReceiveCount":"3"}`},
	}, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func s3EventBody(bucket, key string) string {
	return `{"Records":[{"s3":{"bucket":{"name":"` + bucket + `"},"object":{"key":"` + key + `"}}}]}`
}
