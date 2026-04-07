package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalName(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if l.Name() != "local" {
		t.Errorf("expected name=local, got %s", l.Name())
	}
}

func TestLocalPutAndGet(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	checksum := "abcd1234efgh5678"
	content := "hello artifact"

	if err := l.Put(ctx, checksum, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	rc, err := l.Get(ctx, checksum)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}

	if string(got) != content {
		t.Errorf("expected %q, got %q", content, string(got))
	}
}

func TestLocalGetNotFound(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = l.Get(context.Background(), "doesnotexist1234")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func (t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	checksum := "abcd1234efgh5678"
	content := "hello artifact"

	if err := l.Put(ctx, checksum, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("first Put() error: %v", err)
	}
	if err := l.Put(ctx, checksum, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("second Put() error: %v", err)
	}
}

func TestLocalDelete(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	checksum := "abcd1234efgh5678"
	content := "hello artifact"

	if err := l.Put(ctx, checksum, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if err := l.Delete(ctx, checksum); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = l.Get(ctx, checksum)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestLocalDeleteNotFound(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = l.Delete(context.Background(), "doesnotexist1234")
	if err != nil {
		t.Errorf("expected no error deleting missing artifact, got %v", err)
	}
}

func TestLocalNoTmpFileOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}

	checksum := "abcd1234efgh5678"
	_ = l.Put(context.Background(), checksum, &errorReader{}, 0)

	tmpPath := filepath.Join(dir, checksum[:2], checksum+".tmp")
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf("expected .tmp file to be cleaned up after write failure")
	}
}

type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read error")
}
