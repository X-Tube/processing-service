package queue

import (
	"net/url"
)

type S3EventMessage struct {
	Records []S3EventRecord `json:"Records"`
}

type S3EventRecord struct {
	EventVersion      string   `json:"eventVersion"`
	EventSource      string   `json:"eventSource"`
	AWSRegion        string   `json:"awsRegion"`
	EventTime        string   `json:"eventTime"`
	EventName        string   `json:"eventName"`
	S3               S3Entity `json:"s3"`
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

func (r S3EventRecord) BucketName() string {
	return r.S3.Bucket.Name
}

func (r S3EventRecord) ObjectKey() (string, error) {
	return url.QueryUnescape(r.S3.Object.Key)
}