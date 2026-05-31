package module

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// DigestBytes returns the lowercase SHA-256 digest used to pin module bytes.
func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// DigestFile returns the lowercase SHA-256 digest for a local WASM module file.
func DigestFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read module file: %w", err)
	}
	return DigestBytes(data), nil
}

// FileRef creates a module reference for a local WASM file.
func FileRef(name string, path string, entrypoint string) (Ref, error) {
	digest, err := DigestFile(path)
	if err != nil {
		return Ref{}, err
	}
	return Ref{
		Name:       name,
		Digest:     digest,
		Entrypoint: entrypoint,
		Source:     path,
	}, nil
}
