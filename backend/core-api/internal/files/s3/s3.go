// Package s3 implements files.ObjectStore against any S3-compatible
// endpoint - MinIO locally, real AWS S3 in production, the same adapter
// either way per the roadmap's own framing. Presigned URLs mean this
// service never receives file bytes: PresignPut/PresignGet hand the
// client a URL to talk to storage directly.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

// New connects to the given S3-compatible endpoint and ensures the
// configured bucket exists, creating it if not - the same self-healing
// bootstrap precedent as OpenFGA's store/model and Keycloak's realm
// import, since MinIO's local dev storage is exactly as ephemeral as
// those (a fresh `docker compose up` starts with no buckets at all).
func New(ctx context.Context, cfg Config) (*Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"), // required by the SDK; MinIO ignores it
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true // required for MinIO and most non-AWS S3-compatible stores
	})

	store := &Store{client: client, presign: s3.NewPresignClient(client), bucket: cfg.Bucket}
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

func (s *Store) PresignPut(ctx context.Context, objectKey, contentType string, expires time.Duration) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(objectKey), ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return req.URL, nil
}

func (s *Store) PresignGet(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(objectKey),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return req.URL, nil
}

var ErrObjectNotFound = errors.New("object not found in storage")

// HeadObject returns the object's real size and checksum as storage
// reports them - never the caller's claim. ETag is quoted by S3's wire
// format and, for a simple (non-multipart) PUT, is the object's MD5 hex,
// which is exactly the checksum RequestUpload's optional declared
// Checksum is compared against at confirm time.
func (s *Store) HeadObject(ctx context.Context, objectKey string) (int64, string, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			return 0, "", ErrObjectNotFound
		}
		return 0, "", fmt.Errorf("head object: %w", err)
	}
	size := aws.ToInt64(out.ContentLength)
	etag := strings.Trim(aws.ToString(out.ETag), `"`)
	return size, etag, nil
}

// PutObject writes bytes directly, server-side - unlike every other
// write in this package (PresignPut hands the client a URL to upload
// to), this one exists for Phase 20's privacy export bundle, which this
// service assembles itself in-process and has no client to hand a
// presigned URL to.
func (s *Store) PutObject(ctx context.Context, objectKey string, body []byte, contentType string) error {
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: bytes.NewReader(body), ContentType: aws.String(contentType),
	}); err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *Store) DeleteObject(ctx context.Context, objectKey string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}
