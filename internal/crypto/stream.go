package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
)

func newCTRStream(key []byte, iv [16]byte) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	return cipher.NewCTR(block, iv[:]), nil
}

// NewEncryptReader wraps a reader with AES-256-CTR encryption.
func NewEncryptReader(r io.Reader, key []byte, iv [16]byte) (io.Reader, error) {
	stream, err := newCTRStream(key, iv)
	if err != nil {
		return nil, err
	}
	return &cipher.StreamReader{S: stream, R: r}, nil
}

// NewDecryptReader wraps a reader with AES-256-CTR decryption.
// CTR mode decryption is identical to encryption.
func NewDecryptReader(r io.Reader, key []byte, iv [16]byte) (io.Reader, error) {
	return NewEncryptReader(r, key, iv)
}

// NewEncryptWriter wraps a writer with AES-256-CTR encryption.
func NewEncryptWriter(w io.Writer, key []byte, iv [16]byte) (io.Writer, error) {
	stream, err := newCTRStream(key, iv)
	if err != nil {
		return nil, err
	}
	return &cipher.StreamWriter{S: stream, W: w}, nil
}

// NewDecryptWriter wraps a writer with AES-256-CTR decryption.
func NewDecryptWriter(w io.Writer, key []byte, iv [16]byte) (io.Writer, error) {
	return NewEncryptWriter(w, key, iv)
}
