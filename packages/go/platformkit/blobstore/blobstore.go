// Package blobstore is a minimal, write-oriented S3-compatible client -
// shared infrastructure any service might need, the same rationale as
// rtbus/searchidx/ratelimit already living in platformkit. It exists
// for Phase 23's analytics pipeline, which runs inside `worker`
// (a separate Go module from core-api, so it cannot import
// core-api/internal/files/s3 - the same constraint documented since
// Phase 14's search indexer).
//
// This is deliberately narrower than files/s3.Store: no presigned
// URLs, no HeadObject/DeleteObject, just PutObject - everything an
// append-only batch pipeline landing files in a data lake actually
// needs. If a second consumer ever needs the fuller surface, promoting
// files/s3's implementation here (so core-api's own files module
// depends on this shared package too, rather than the reverse) would
// be the natural next step - not done now since it would touch
// Phase 13's already-validated code for a need that doesn't exist yet.
package blobstore

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
}

type Store struct {
	client *s3.Client
	bucket string
}

// New connects to the given S3-compatible endpoint and ensures the
// configured bucket exists, creating it if not - the same self-healing
// bootstrap precedent as files/s3.New, OpenFGA's store/model, and
// Keycloak's realm import: MinIO's local dev storage is exactly as
// ephemeral as those.
func New(ctx context.Context, cfg Config) (*Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})

	store := &Store{client: client, bucket: cfg.Bucket}
	if err := store.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	if _, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}); createErr != nil {
		return fmt.Errorf("ensure bucket %q exists: %w", s.bucket, createErr)
	}
	return nil
}

func (s *Store) PutObject(ctx context.Context, key string, body []byte, contentType string) error {
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(body), ContentType: aws.String(contentType),
	}); err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}
