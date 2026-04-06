package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

func CheckSum(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("Computing Checksum %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func ChecksumTree(r io.Reader, dst io.Writer) (string, error){
	h := sha256.New()
	tee := io.TeeReader(r, h)
	if _, err := io.Copy(dst, tee); err != nil {
		return "", fmt.Errorf("Streaming with checksum: %w", err)
	}
	
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ChecksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}