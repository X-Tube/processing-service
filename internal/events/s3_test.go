package events

import (
	"fmt"
	"testing"
)

func TestParseS3EventMessageDirectEvent(t *testing.T) {
	event, ignored, err := ParseS3Message(s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if ignored {
		t.Fatalf("expected event to be processed")
	}

	if len(event.Records) != 1 {
		t.Fatalf("expected one record, got %d", len(event.Records))
	}
}

func TestParseS3EventMessageSNSWrappedEvent(t *testing.T) {
	body := fmt.Sprintf(`{"Type":"Notification","Message":%q}`, s3EventBody("xtube-videos-input", "uploads/video-123/original.mp4"))

	event, ignored, err := ParseS3Message(body)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if ignored {
		t.Fatalf("expected event to be processed")
	}

	if len(event.Records) != 1 {
		t.Fatalf("expected one record, got %d", len(event.Records))
	}
}

func TestParseS3EventMessageIgnoresS3TestEvent(t *testing.T) {
	body := `{"Service":"Amazon S3","Event":"s3:TestEvent","Bucket":"xtube-videos-input"}`

	event, ignored, err := ParseS3Message(body)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !ignored {
		t.Fatalf("expected s3 test event to be ignored")
	}

	if len(event.Records) != 0 {
		t.Fatalf("expected no records, got %d", len(event.Records))
	}
}

func TestParseS3EventMessageRejectsUnknownBodyWithoutRecords(t *testing.T) {
	_, ignored, err := ParseS3Message(`{"hello":"world"}`)
	if err == nil {
		t.Fatalf("expected error")
	}

	if ignored {
		t.Fatalf("expected unknown event not to be ignored")
	}
}

func TestSummarizeS3Message(t *testing.T) {
	body := `{"Records":[{"eventSource":"aws:s3","eventName":"ObjectCreated:Put","s3":{"bucket":{"name":"xtube-videos-input"},"object":{"key":"uploads/video-123/original.mp4"}}}]}`

	summary := SummarizeS3Message(body)

	if summary.RecordCount != 1 {
		t.Fatalf("expected one record, got %d", summary.RecordCount)
	}
	if summary.EventSource != "aws:s3" {
		t.Fatalf("expected event source aws:s3, got %q", summary.EventSource)
	}
	if summary.EventName != "ObjectCreated:Put" {
		t.Fatalf("expected event name ObjectCreated:Put, got %q", summary.EventName)
	}
	if summary.Bucket != "xtube-videos-input" {
		t.Fatalf("expected bucket xtube-videos-input, got %q", summary.Bucket)
	}
	if summary.Key != "uploads/video-123/original.mp4" {
		t.Fatalf("expected key uploads/video-123/original.mp4, got %q", summary.Key)
	}
}

func s3EventBody(bucket, key string) string {
	return fmt.Sprintf(`{"Records":[{"s3":{"bucket":{"name":%q},"object":{"key":%q}}}]}`, bucket, key)
}
