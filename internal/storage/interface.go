package storage

import (
	"context"
	"errors"
	"io"
)

var ErrNotFound = errors.New("Artifact was not found")

type StorageBackend interface {
	Get(ctx context.Context, checksum string) (io.ReadCloser, error)
	Put(ctx context.Context, checksum string, r io.Reader, size int64) error
	Exists(ctx context.Context, checksum string) (bool, error)
	Delete(ctx context.Context, checksum string) error
	Name() string
}