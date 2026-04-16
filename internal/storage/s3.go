package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config holds the settings needed to connect to an S3-compatible store.
// Endpoint is optional and is only needed for non-AWS providers (e.g. MinIO, LocalStack).
type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string // optional: override the AWS endpoint URL
	KeyPrefix       string // optional: prefix prepended to every object key
	AccessKeyID     string // optional: static credentials (falls back to default chain)
	SecretAccessKey string // optional
}

// S3 is a StorageBackend backed by an S3-compatible object store.
type S3 struct {
	client    *s3.Client
	bucket    string
	keyPrefix string
}

// NewS3 creates an S3 backend.  Credentials are resolved in this order:
//  1. Explicit AccessKeyID / SecretAccessKey (if both are non-empty)
//  2. The standard AWS credential chain (env vars, ~/.aws/credentials, IAM role, …)
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// Path-style addressing is required by most S3-compatible servers.
			o.UsePathStyle = true
		})
	}

	return &S3{
		client:    s3.NewFromConfig(awsCfg, s3Opts...),
		bucket:    cfg.Bucket,
		keyPrefix: cfg.KeyPrefix,
	}, nil
}

func (s *S3) Name() string { return "s3" }

func (s *S3) Get(ctx context.Context, checksum string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(checksum)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("S3 GetObject %s: %w", checksum[:8], err)
	}
	return out.Body, nil
}

func (s *S3) Put(ctx context.Context, checksum string, r io.Reader, size int64) error {
	exists, err := s.Exists(ctx, checksum)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(checksum)),
		Body:   r,
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("S3 PutObject %s: %w", checksum[:8], err)
	}
	return nil
}

func (s *S3) Exists(ctx context.Context, checksum string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(checksum)),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("S3 HeadObject %s: %w", checksum[:8], err)
	}
	return true, nil
}

func (s *S3) Delete(ctx context.Context, checksum string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(checksum)),
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("S3 DeleteObject %s: %w", checksum[:8], err)
	}
	return nil
}

// key builds the full object key for a checksum, mirroring the two-level
// directory structure used by the local backend (first two hex chars / full checksum).
func (s *S3) key(checksum string) string {
	var suffix string
	if len(checksum) >= 2 {
		suffix = checksum[:2] + "/" + checksum
	} else {
		suffix = checksum
	}

	if s.keyPrefix == "" {
		return suffix
	}
	return strings.TrimRight(s.keyPrefix, "/") + "/" + suffix
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var notFound *types.NotFound
	return errors.As(err, &notFound)
}
