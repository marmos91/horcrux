package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadKeyFile(t *testing.T) {
	dir := t.TempDir()

	// Create a valid key file
	keyPath := filepath.Join(dir, "test.key")
	if err := os.WriteFile(keyPath, []byte("test key material"), 0o600); err != nil {
		t.Fatal(err)
	}

	hash, err := ReadKeyFile(keyPath)
	if err != nil {
		t.Fatalf("ReadKeyFile: %v", err)
	}

	// Should be deterministic
	hash2, err := ReadKeyFile(keyPath)
	if err != nil {
		t.Fatalf("ReadKeyFile second call: %v", err)
	}
	if hash != hash2 {
		t.Fatal("ReadKeyFile should be deterministic")
	}
}

func TestReadKeyFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "empty.key")
	if err := os.WriteFile(keyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ReadKeyFile(keyPath)
	if err != ErrEmptyKeyFile {
		t.Fatalf("expected ErrEmptyKeyFile, got %v", err)
	}
}

func TestReadKeyFile_Directory(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadKeyFile(dir)
	if err != ErrKeyFileIsDir {
		t.Fatalf("expected ErrKeyFileIsDir, got %v", err)
	}
}

func TestReadKeyFile_NotFound(t *testing.T) {
	_, err := ReadKeyFile("/nonexistent/path/key.file")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestGenerateKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "gen.key")

	if err := GenerateKeyFile(keyPath, KeyFileSize); err != nil {
		t.Fatalf("GenerateKeyFile: %v", err)
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != KeyFileSize {
		t.Fatalf("expected size %d, got %d", KeyFileSize, info.Size())
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestGenerateKeyFile_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "existing.key")

	if err := os.WriteFile(keyPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := GenerateKeyFile(keyPath, KeyFileSize)
	if err != ErrKeyFileExists {
		t.Fatalf("expected ErrKeyFileExists, got %v", err)
	}
}

func TestGenerateKeyFile_InvalidSize(t *testing.T) {
	dir := t.TempDir()

	if err := GenerateKeyFile(filepath.Join(dir, "a.key"), 0); err != ErrKeyFileSizeRange {
		t.Fatalf("expected ErrKeyFileSizeRange for size 0, got %v", err)
	}
	if err := GenerateKeyFile(filepath.Join(dir, "b.key"), maxKeyFileSize+1); err != ErrKeyFileSizeRange {
		t.Fatalf("expected ErrKeyFileSizeRange for size > max, got %v", err)
	}
}

func TestCombinePasswordAndKeyFile(t *testing.T) {
	keyMaterial := []byte("test key material 32 bytes long!")

	result := CombinePasswordAndKeyFile("password", keyMaterial)
	if len(result) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(result))
	}

	// Deterministic
	result2 := CombinePasswordAndKeyFile("password", keyMaterial)
	if !bytes.Equal(result, result2) {
		t.Fatal("CombinePasswordAndKeyFile should be deterministic")
	}

	// Different password produces different result
	result3 := CombinePasswordAndKeyFile("other-password", keyMaterial)
	if bytes.Equal(result, result3) {
		t.Fatal("different passwords should produce different results")
	}
}
