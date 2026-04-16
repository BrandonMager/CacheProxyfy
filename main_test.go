package main

import (
	"testing"

	"github.com/BrandonMager/CacheProxyfy/internal/config"
	"github.com/BrandonMager/CacheProxyfy/internal/storage"
)

func TestBuildStorage_S3_returnsS3Backend(t *testing.T) {
	cfg := &config.Config{
		Cache: config.CacheConfig{
			Backend: "s3",
		},
		S3: config.S3Config{
			Bucket:          "my-bucket",
			Region:          "us-east-1",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
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
