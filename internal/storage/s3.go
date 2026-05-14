package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Client interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

type ObjectStore interface {
	DownloadToFile(ctx context.Context, bucket, key, dstPath string) error
	UploadFile(ctx context.Context, bucket, key, filePath string) error
}

type S3ObjectStore struct {
	client S3Client
}

func NewS3ObjectStore(client S3Client) *S3ObjectStore {
	return &S3ObjectStore{client: client}
}

func (s *S3ObjectStore) DownloadToFile(ctx context.Context, bucket, key, dstPath string) error {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("download s3 object %s/%s: %w", bucket, key, err)
	}
	defer output.Body.Close()

	file, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, output.Body); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}

	return nil
}

func (s *S3ObjectStore) UploadFile(ctx context.Context, bucket, key, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for upload: %w", err)
	}
	defer file.Close()

	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("upload file to s3 %s/%s: %w", bucket, key, err)
	}

	return nil
}
