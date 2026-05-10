package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type VideoUploadMessage struct {
	VideoID string `json:"videoId"`
	Bucket  string `json:"bucket"`
	Key     string `json:"key"`
}

type Worker struct {
	client   *sqs.Client
	queueURL string
}

func NewWorker(client *sqs.Client, queueURL string) *Worker {
	return &Worker{
		client:   client,
		queueURL: queueURL,
	}
}

func (w *Worker) Start(ctx context.Context) {
	log.Println("SQS worker started")

	for {

		select {
		case <-ctx.Done():
			log.Println("SQS worker stopped")
			return

		default:
			err := w.poll(ctx)
			if err != nil {
				log.Println("SQS polling error:", err)
				time.Sleep(2 * time.Second)
			}
		}

	}
}

func (w *Worker) poll(ctx context.Context) error {
	output, err := w.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(w.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20,
		VisibilityTimeout:   60,
	})

	if err != nil {
		return err
	}

	if len(output.Messages) == 0 {
		log.Println("no messages")
		return nil
	}

	for _, message := range output.Messages {
		if message.Body == nil || message.ReceiptHandle == nil {
			continue
		}

		err := w.process(*message.Body)
		if err != nil {
			log.Println("message processing error:", err)
			continue
		}

		_, err = w.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(w.queueURL),
			ReceiptHandle: message.ReceiptHandle,
		})

		if err != nil {
			log.Println("delete message error:", err)
			continue
		}

		log.Println("message processed and deleted")
	}

	return nil
}

func (w *Worker) process(body string) error {
	var message VideoUploadMessage

	err := json.Unmarshal([]byte(body), &message)
	if err != nil {
		return err
	}

	fmt.Println("videoId:", message.VideoID)
	fmt.Println("bucket:", message.Bucket)
	fmt.Println("key:", message.Key)

	return nil
}
