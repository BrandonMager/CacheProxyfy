package storage

import (
	"bytes"
	"context"
	"io"
	"net"
	"os/exec"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

func TestS3_isNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "NoSuchKey returns true",
			err:  &types.NoSuchKey{},
			want: true,
		},
		{
			name: "NotFound returns true",
			err:  &types.NotFound{},
			want: true,
		},
		{
			name: "nil returns false",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error returns false",
			err:  ErrNotFound,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFound(tt.err)
			if got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestS3Integration_CRUD(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx := context.Background()

	ctr, err := localstack.Run(ctx, "localstack/localstack:3")
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start localstack: %v", err)
	}

	// Resolve the LocalStack endpoint so our S3 backend points at the container.
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

	store, err := NewS3(ctx, S3Config{
		Bucket:          bucket,
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	// Create the bucket via the underlying client (bucket provisioning is outside
	// the scope of StorageBackend).
	if _, err := store.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	const checksum = "abcdef1234567890abcdef1234567890"
	content := []byte("hello from localstack")

	// Exists returns false before any Put.
	exists, err := store.Exists(ctx, checksum)
	if err != nil {
		t.Fatalf("Exists (before put): %v", err)
	}
	if exists {
		t.Fatal("Exists (before put): want false, got true")
	}

	// Get on a missing key returns ErrNotFound.
	_, err = store.Get(ctx, checksum)
	if err != ErrNotFound {
		t.Fatalf("Get (missing): want ErrNotFound, got %v", err)
	}

	// Put stores the object.
	if err := store.Put(ctx, checksum, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Exists returns true after Put.
	exists, err = store.Exists(ctx, checksum)
	if err != nil {
		t.Fatalf("Exists (after put): %v", err)
	}
	if !exists {
		t.Fatal("Exists (after put): want true, got false")
	}

	// Get returns the correct bytes.
	rc, err := store.Get(ctx, checksum)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Get body = %q, want %q", got, content)
	}

	// Second Put with the same checksum is a no-op (idempotent).
	if err := store.Put(ctx, checksum, bytes.NewReader([]byte("different content")), 17); err != nil {
		t.Fatalf("Put (second): %v", err)
	}
	rc2, err := store.Get(ctx, checksum)
	if err != nil {
		t.Fatalf("Get (after second put): %v", err)
	}
	defer rc2.Close()
	got2, _ := io.ReadAll(rc2)
	if string(got2) != string(content) {
		t.Errorf("idempotent Put overwrote object: got %q, want %q", got2, content)
	}

	// Delete removes the object.
	if err := store.Delete(ctx, checksum); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Exists returns false after Delete.
	exists, err = store.Exists(ctx, checksum)
	if err != nil {
		t.Fatalf("Exists (after delete): %v", err)
	}
	if exists {
		t.Fatal("Exists (after delete): want false, got true")
	}
}

func TestS3_key(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		checksum string
		want     string
	}{
		{
			name:     "no prefix, normal checksum",
			prefix:   "",
			checksum: "abcdef1234567890",
			want:     "ab/abcdef1234567890",
		},
		{
			name:     "with prefix, normal checksum",
			prefix:   "artifacts",
			checksum: "abcdef1234567890",
			want:     "artifacts/ab/abcdef1234567890",
		},
		{
			name:     "prefix with trailing slash is normalised",
			prefix:   "artifacts/",
			checksum: "abcdef1234567890",
			want:     "artifacts/ab/abcdef1234567890",
		},
		{
			name:     "nested prefix",
			prefix:   "cache/npm",
			checksum: "abcdef1234567890",
			want:     "cache/npm/ab/abcdef1234567890",
		},
		{
			name:     "short checksum (1 char) does not panic",
			prefix:   "",
			checksum: "a",
			want:     "a",
		},
		{
			name:     "empty checksum does not panic",
			prefix:   "",
			checksum: "",
			want:     "",
		},
		{
			name:     "short checksum with prefix does not panic",
			prefix:   "artifacts",
			checksum: "a",
			want:     "artifacts/a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &S3{keyPrefix: tt.prefix}
			got := s.key(tt.checksum)
			if got != tt.want {
				t.Errorf("key(%q) with prefix %q = %q, want %q", tt.checksum, tt.prefix, got, tt.want)
			}
		})
	}
}
