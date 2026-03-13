package qr

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeShard_SmallFile(t *testing.T) {
	tmpDir := t.TempDir()
	shardPath := filepath.Join(tmpDir, "test.hrcx")

	// Create a small test shard file (500 bytes)
	data := make([]byte, 500)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shardPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	img, err := EncodeShard(shardPath, DefaultQRSize)
	if err != nil {
		t.Fatalf("EncodeShard failed: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != DefaultQRSize || bounds.Dy() != DefaultQRSize {
		t.Errorf("expected %dx%d image, got %dx%d", DefaultQRSize, DefaultQRSize, bounds.Dx(), bounds.Dy())
	}
}

func TestEncodeShard_MaxSize(t *testing.T) {
	tmpDir := t.TempDir()
	shardPath := filepath.Join(tmpDir, "test.hrcx")

	// Create a file at max QR capacity
	data := make([]byte, MaxQRBinaryBytes)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shardPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := EncodeShard(shardPath, DefaultQRSize)
	if err != nil {
		t.Fatalf("EncodeShard should succeed at max size: %v", err)
	}
}

func TestEncodeShard_TooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	shardPath := filepath.Join(tmpDir, "test.hrcx")

	// Create an oversized file
	data := make([]byte, MaxQRBinaryBytes+1)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shardPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := EncodeShard(shardPath, DefaultQRSize)
	if err == nil {
		t.Fatal("expected error for oversized shard")
	}

	stErr, ok := err.(*ShardTooLargeError)
	if !ok {
		t.Fatalf("expected ShardTooLargeError, got %T: %v", err, err)
	}
	if stErr.Size != int64(MaxQRBinaryBytes+1) {
		t.Errorf("expected size %d, got %d", MaxQRBinaryBytes+1, stErr.Size)
	}
}

func TestCheckShardFits(t *testing.T) {
	tmpDir := t.TempDir()

	// File that fits
	smallPath := filepath.Join(tmpDir, "small.hrcx")
	if err := os.WriteFile(smallPath, make([]byte, 1000), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CheckShardFits(smallPath); err != nil {
		t.Errorf("expected small shard to fit: %v", err)
	}

	// File that doesn't fit
	largePath := filepath.Join(tmpDir, "large.hrcx")
	if err := os.WriteFile(largePath, make([]byte, MaxQRBinaryBytes+1), 0644); err != nil {
		t.Fatal(err)
	}
	err := CheckShardFits(largePath)
	if err == nil {
		t.Fatal("expected error for oversized shard")
	}
	if _, ok := err.(*ShardTooLargeError); !ok {
		t.Fatalf("expected ShardTooLargeError, got %T", err)
	}
}
