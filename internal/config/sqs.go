package config

import (
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func NewSQSClient(cfg aws.Config) *sqs.Client {
	endpointURL := os.Getenv("AWS_ENDPOINT_URL")

	if endpointURL == "" {
		return sqs.NewFromConfig(cfg)
	}

	return sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})
}