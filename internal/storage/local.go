package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Local struct {
	BaseDir string
}

func NewLocal(baseDir string) (*Local, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating local storage dir %q: %w", baseDir, err)
	}

	return &Local{BaseDir: baseDir}, nil
}

func (l *Local) Name() string { return "local" }

func (l *Local) Get(_ context.Context, checksum string) (io.ReadCloser, error) {
	path := l.path(checksum)
	f, err := os.Open(path)

	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("Opening artifact %s: %w", checksum[:8], err)
	}

	return f, nil
}

func (l *Local) Put(_ context.Context, checksum string, r io.Reader, _ int64) error {
	path := l.path(checksum)
	
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating artifact dir: %w", err)
	}

	

	tmp := path + ".tmp"
	f, err := os.Create(tmp)

	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	// Good practice to use .tmp showing artifact currently being created/modified
	// io.Copy(dst, src) notes

	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("writing artifact: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("closing artifact: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("finalising artifact: %w", err)
	}

	return nil
}

func (l *Local) Delete(_ context.Context, checksum string) error {
	err := os.Remove(l.path(checksum))
	if os.IsNotExist(err){
		return nil 
	}

	return err
}

func (l *Local) path(checksum string) string {
	if len(checksum) < 2 {
		return filepath.Join(l.BaseDir, checksum)
	}

	return filepath.Join(l.BaseDir, checksum[:2], checksum)
}