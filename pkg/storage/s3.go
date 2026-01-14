package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Provider defines the interface for uploading forensic artifacts
type Provider interface {
	Upload(ctx context.Context, key string, data []byte) (string, error)
	UploadFile(ctx context.Context, key string, filePath string) (string, error)
}

// S3Provider implements Provider for AWS S3
type S3Provider struct {
	Client *s3.Client
	Bucket string
	Region string
}

// NewS3Provider creates a new S3Provider
func NewS3Provider(ctx context.Context, bucket string, region string) (*S3Provider, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	return &S3Provider{
		Client: s3.NewFromConfig(cfg),
		Bucket: bucket,
		Region: region,
	}, nil
}

// Upload uploads byte data to S3
func (e *S3Provider) Upload(ctx context.Context, key string, data []byte) (string, error) {
	_, err := e.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(e.Bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", e.Bucket, key), nil
}

// UploadFile uploads a file from disk
func (e *S3Provider) UploadFile(ctx context.Context, key string, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = e.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(e.Bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", e.Bucket, key), nil
}

// NoOpProvider is a fallback
type NoOpProvider struct{}

func (e *NoOpProvider) Upload(ctx context.Context, key string, data []byte) (string, error) {
	return "", nil
}

func (e *NoOpProvider) UploadFile(ctx context.Context, key string, filePath string) (string, error) {
	return "", nil
}
