package queue

import (
	"context"
	"log"
	"time"

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
}

func NewWorker(client *sqs.Client, queueURL string, processor MessageProcessor, config WorkerConfig) *Worker {
	return &Worker{
		client:    client,
		queueURL:  queueURL,
		config:    config,
		processor: processor,
	}
}

func (w *Worker) Start(ctx context.Context) {
	log.Printf("SQS %s worker started", w.processor.Name())

	for {
		select {
		case <-ctx.Done():
			log.Printf("SQS %s worker stopped", w.processor.Name())
			return

		default:
			if err := w.poll(ctx); err != nil {
				log.Printf("SQS %s polling error: %v", w.processor.Name(), err)
				time.Sleep(w.config.ErrorDelay)
			}
		}
	}
}

func (w *Worker) poll(ctx context.Context) error {
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

	for _, message := range output.Messages {
		if message.Body == nil || message.ReceiptHandle == nil {
			continue
		}

		if err := w.processor.Process(ctx, *message.Body); err != nil {
			log.Printf("SQS %s message processing error: %v", w.processor.Name(), err)
			continue
		}

		_, err = w.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(w.queueURL),
			ReceiptHandle: message.ReceiptHandle,
		})

		if err != nil {
			log.Printf("SQS %s delete message error: %v", w.processor.Name(), err)
			continue
		}

		log.Printf("SQS %s message processed and deleted", w.processor.Name())
	}

	return nil
}