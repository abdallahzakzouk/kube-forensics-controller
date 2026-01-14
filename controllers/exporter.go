package controllers

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Exporter defines the interface for uploading forensic artifacts
type Exporter interface {
	Upload(ctx context.Context, key string, data []byte) (string, error)
}

// S3Exporter implements Exporter for AWS S3
type S3Exporter struct {
	Client *s3.Client
	Bucket string
	Region string
}

// NewS3Exporter creates a new S3Exporter with default config (IRSA/Env vars)
func NewS3Exporter(ctx context.Context, bucket string, region string) (*S3Exporter, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	return &S3Exporter{
		Client: s3.NewFromConfig(cfg),
		Bucket: bucket,
		Region: region,
	}, nil
}

// Upload uploads data to S3 and returns the S3 URI
func (e *S3Exporter) Upload(ctx context.Context, key string, data []byte) (string, error) {
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

// NoOpExporter is a fallback
type NoOpExporter struct{}

func (e *NoOpExporter) Upload(ctx context.Context, key string, data []byte) (string, error) {
	return "", nil
}
