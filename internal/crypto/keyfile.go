package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	// KeyFileSize is the default size in bytes for generated key files.
	KeyFileSize = 64

	// maxKeyFileSize is the maximum key file size we accept (1 MB).
	maxKeyFileSize = 1 << 20
)

var (
	ErrEmptyKeyFile     = errors.New("key file is empty")
	ErrKeyFileIsDir     = errors.New("key file path is a directory")
	ErrKeyFileExists    = errors.New("key file already exists (refusing to overwrite)")
	ErrKeyFileTooLarge  = errors.New("key file exceeds 1 MB")
	ErrKeyFileSizeRange = errors.New("key file size must be between 1 and 1048576 bytes")
)

// ReadKeyFile reads a key file and returns its SHA-256 hash as 32 bytes of key material.
// It rejects empty files, directories, and files larger than 1 MB.
func ReadKeyFile(path string) ([32]byte, error) {
	var zero [32]byte

	info, err := os.Stat(path)
	if err != nil {
		return zero, fmt.Errorf("reading key file: %w", err)
	}
	if info.IsDir() {
		return zero, ErrKeyFileIsDir
	}
	if info.Size() == 0 {
		return zero, ErrEmptyKeyFile
	}
	if info.Size() > maxKeyFileSize {
		return zero, ErrKeyFileTooLarge
	}

	f, err := os.Open(path)
	if err != nil {
		return zero, fmt.Errorf("opening key file: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return zero, fmt.Errorf("hashing key file: %w", err)
	}

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

// GenerateKeyFile creates a new key file with cryptographically random bytes.
// It refuses to overwrite existing files.
func GenerateKeyFile(path string, size int) error {
	if size < 1 || size > maxKeyFileSize {
		return ErrKeyFileSizeRange
	}

	// O_EXCL ensures we don't overwrite existing files
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return ErrKeyFileExists
		}
		return fmt.Errorf("creating key file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.CopyN(f, rand.Reader, int64(size)); err != nil {
		// Clean up partial file on error
		_ = os.Remove(path)
		return fmt.Errorf("writing key file: %w", err)
	}

	return nil
}

// CombinePasswordAndKeyFile combines a password with key file material using
// HMAC-SHA256 for two-factor encryption mode.
func CombinePasswordAndKeyFile(password string, keyFileMaterial [32]byte) []byte {
	mac := hmac.New(sha256.New, keyFileMaterial[:])
	mac.Write([]byte(password))
	return mac.Sum(nil)
}
