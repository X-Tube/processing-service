package events

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type S3EventMessage struct {
	Records []S3EventRecord `json:"Records"`
}

type S3MessageSummary struct {
	RecordCount int
	EventSource string
	EventName   string
	Bucket      string
	Key         string
}

type S3EventRecord struct {
	EventVersion string   `json:"eventVersion"`
	EventSource  string   `json:"eventSource"`
	AWSRegion    string   `json:"awsRegion"`
	EventTime    string   `json:"eventTime"`
	EventName    string   `json:"eventName"`
	S3           S3Entity `json:"s3"`
}

type S3Entity struct {
	Bucket S3Bucket `json:"bucket"`
	Object S3Object `json:"object"`
}

type S3Bucket struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

type S3Object struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	ETag string `json:"eTag"`
}

type s3TestEventMessage struct {
	Event string `json:"Event"`
}

type snsNotificationMessage struct {
	Message string `json:"Message"`
}

func (r S3EventRecord) BucketName() string {
	return r.S3.Bucket.Name
}

func (r S3EventRecord) ObjectKey() (string, error) {
	return url.QueryUnescape(r.S3.Object.Key)
}

func RecordBucketKey(record S3EventRecord) (string, string, error) {
	key, err := record.ObjectKey()
	if err != nil {
		return "", "", err
	}

	return record.BucketName(), key, nil
}

func ParseS3Message(body string) (S3EventMessage, bool, error) {
	event, err := unmarshalS3EventMessage(body)
	if err != nil {
		return S3EventMessage{}, false, err
	}

	if len(event.Records) > 0 {
		return event, false, nil
	}

	if isS3TestEvent(body) {
		return S3EventMessage{}, true, nil
	}

	var notification snsNotificationMessage
	if err := json.Unmarshal([]byte(body), &notification); err == nil && notification.Message != "" {
		event, err := unmarshalS3EventMessage(notification.Message)
		if err != nil {
			return S3EventMessage{}, false, fmt.Errorf("parse sns s3 event message: %w", err)
		}

		if len(event.Records) > 0 {
			return event, false, nil
		}

		if isS3TestEvent(notification.Message) {
			return S3EventMessage{}, true, nil
		}
	}

	return S3EventMessage{}, false, fmt.Errorf("s3 event records are required")
}

func SummarizeS3Message(body string) S3MessageSummary {
	event, ignored, err := ParseS3Message(body)
	if err != nil || ignored {
		return S3MessageSummary{}
	}

	summary := S3MessageSummary{
		RecordCount: len(event.Records),
	}

	if len(event.Records) == 0 {
		return summary
	}

	record := event.Records[0]
	key, _ := record.ObjectKey()

	summary.EventSource = record.EventSource
	summary.EventName = record.EventName
	summary.Bucket = record.BucketName()
	summary.Key = key

	return summary
}

func unmarshalS3EventMessage(body string) (S3EventMessage, error) {
	var event S3EventMessage
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return S3EventMessage{}, err
	}

	return event, nil
}

func isS3TestEvent(body string) bool {
	var event s3TestEventMessage
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return false
	}

	return event.Event == "s3:TestEvent"
}
