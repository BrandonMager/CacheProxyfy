package main

import (
	"context"
	"net"
	"os/exec"
	"testing"

	"github.com/BrandonMager/CacheProxyfy/internal/config"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

func TestBuildStorage_Local_returnsLocalBackend(t *testing.T) {
	cfg := &config.Config{
		Cache: config.CacheConfig{
			Backend:  "local",
			LocalDir: t.TempDir(),
		},
	}

	got, err := buildStorage(cfg)
	if err != nil {
		t.Fatalf("buildStorage: unexpected error: %v", err)
	}

	if _, ok := got.(*storage.Local); !ok {
		t.Errorf("buildStorage returned %T, want *storage.Local", got)
	}
}

func TestBuildStorage_S3_returnsS3Backend(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	ctr, err := localstack.Run(ctx, "localstack/localstack:3")
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start localstack: %v", err)
	}

	mappedPort, err := ctr.MappedPort(ctx, "4566/tcp")
	if err != nil {
		t.Fatalf("localstack port: %v", err)
	}
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Fatalf("docker provider: %v", err)
	}
	defer provider.Close()
	host, err := provider.DaemonHost(ctx)
	if err != nil {
		t.Fatalf("daemon host: %v", err)
	}
	endpoint := "http://" + net.JoinHostPort(host, mappedPort.Port())

	const bucket = "test-bucket"

	// Pre-create the bucket so buildStorage can validate it.
	store, err := storage.NewS3(ctx, storage.S3Config{
		Bucket:          bucket,
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatalf("pre-create S3 backend: %v", err)
	}
	if _, err := store.Client().CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	cfg := &config.Config{
		Cache: config.CacheConfig{Backend: "s3"},
		S3: config.S3Config{
			Bucket:          bucket,
			Region:          "us-east-1",
			Endpoint:        endpoint,
			AccessKeyID:     "test",
			SecretAccessKey: "test",
		},
	}

	got, err := buildStorage(cfg)
	if err != nil {
		t.Fatalf("buildStorage: unexpected error: %v", err)
	}

	if _, ok := got.(*storage.S3); !ok {
		t.Errorf("buildStorage returned %T, want *storage.S3", got)
	}
}

func TestBuildStorage_unknownBackend_returnsError(t *testing.T) {
	cfg := &config.Config{
		Cache: config.CacheConfig{Backend: "gcs"},
	}

	_, err := buildStorage(cfg)
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
}
