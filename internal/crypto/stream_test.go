package crypto

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	iv := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	plaintext := []byte("Hello, World! This is a test of the streaming encryption.")

	// Encrypt
	encReader, err := NewEncryptReader(bytes.NewReader(plaintext), key, iv)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := io.ReadAll(encReader)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	// Decrypt
	decReader, err := NewDecryptReader(bytes.NewReader(ciphertext), key, iv)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := io.ReadAll(decReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted doesn't match plaintext: got %q", decrypted)
	}
}

func TestEncryptDecryptLargeData(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	iv := [16]byte{}
	rand.Read(iv[:])

	// 1MB of random data
	plaintext := make([]byte, 1<<20)
	rand.Read(plaintext)

	encReader, err := NewEncryptReader(bytes.NewReader(plaintext), key, iv)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := io.ReadAll(encReader)
	if err != nil {
		t.Fatal(err)
	}

	decReader, err := NewDecryptReader(bytes.NewReader(ciphertext), key, iv)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := io.ReadAll(decReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatal("round-trip failed for large data")
	}
}

func TestEncryptDecryptEmptyInput(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	iv := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	encReader, err := NewEncryptReader(bytes.NewReader(nil), key, iv)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := io.ReadAll(encReader)
	if err != nil {
		t.Fatal(err)
	}
	if len(ciphertext) != 0 {
		t.Fatalf("expected empty ciphertext, got %d bytes", len(ciphertext))
	}
}

func TestEncryptWriterDecryptReader(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	iv := [16]byte{}
	rand.Read(iv[:])

	plaintext := []byte("Testing writer/reader combo for streaming crypto")

	// Encrypt via writer
	var cipherBuf bytes.Buffer
	encWriter, err := NewEncryptWriter(&cipherBuf, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encWriter.Write(plaintext); err != nil {
		t.Fatal(err)
	}

	// Decrypt via reader
	decReader, err := NewDecryptReader(&cipherBuf, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := io.ReadAll(decReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatal("writer-encrypt + reader-decrypt round-trip failed")
	}
}
