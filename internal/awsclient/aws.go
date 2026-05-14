package awsclient

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func LoadConfig(ctx context.Context, region string) (aws.Config, error) {
	if region == "" {
		return awsconfig.LoadDefaultConfig(ctx)
	}

	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

func NewSQSClient(cfg aws.Config, endpointURL string) *sqs.Client {
	if endpointURL == "" {
		return sqs.NewFromConfig(cfg)
	}

	return sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})
}

func NewS3Client(cfg aws.Config, endpointURL string) *s3.Client {
	if endpointURL == "" {
		return s3.NewFromConfig(cfg)
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
		o.UsePathStyle = true
	})
}
