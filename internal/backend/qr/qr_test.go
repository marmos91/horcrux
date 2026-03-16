package qr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/shard"
)

// createTestShard creates a minimal valid .hrcx shard file for testing.
func createTestShard(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)

	hdr := &shard.Header{
		Version:          shard.Version,
		ShardIndex:       0,
		DataShards:       2,
		ParityShards:     1,
		OriginalFileSize: uint64(len(data)),
		PayloadSize:      uint64(len(data)),
		OriginalFilename: "test.txt",
	}

	w, err := shard.CreateWriter(path, hdr)
	if err != nil {
		t.Fatalf("creating test shard: %v", err)
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			t.Fatalf("writing test data: %v", err)
		}
	}
	if err := w.WriteTrailer(); err != nil {
		t.Fatalf("writing trailer: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing shard: %v", err)
	}
	return path
}

func TestQRBackendRoundTrip(t *testing.T) {
	// Create a small shard that fits in a QR code
	srcDir := t.TempDir()
	qrDir := t.TempDir()
	destDir := t.TempDir()

	testData := []byte("hello horcrux")
	shardPath := createTestShard(t, srcDir, "test.txt.000.hrcx", testData)

	// Create QR backend
	b, err := backend.Open("qr://"+qrDir, nil)
	if err != nil {
		t.Fatalf("opening QR backend: %v", err)
	}

	ctx := context.Background()

	// Upload (encode to QR)
	if err := b.Upload(ctx, shardPath, "test.txt.000.hrcx"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Verify image was created
	pngPath := filepath.Join(qrDir, "test.txt.000.png")
	if _, err := os.Stat(pngPath); err != nil {
		t.Fatalf("QR image not created: %v", err)
	}

	// List
	files, err := b.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Key != "test.txt.000.hrcx" {
		t.Errorf("expected key test.txt.000.hrcx, got %s", files[0].Key)
	}

	// Download (decode from QR)
	downloadPath := filepath.Join(destDir, "test.txt.000.hrcx")
	if err := b.Download(ctx, "test.txt.000.hrcx", downloadPath); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Verify the downloaded shard matches the original
	original, err := os.ReadFile(shardPath)
	if err != nil {
		t.Fatalf("reading original: %v", err)
	}
	downloaded, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("reading downloaded: %v", err)
	}
	if string(original) != string(downloaded) {
		t.Errorf("downloaded shard differs from original (orig=%d bytes, dl=%d bytes)", len(original), len(downloaded))
	}

	// Delete
	if err := b.Delete(ctx, "test.txt.000.hrcx"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(pngPath); !os.IsNotExist(err) {
		t.Error("QR image not deleted")
	}
}

func TestQRBackendSVGFormat(t *testing.T) {
	srcDir := t.TempDir()
	qrDir := t.TempDir()

	testData := []byte("svg test")
	shardPath := createTestShard(t, srcDir, "test.txt.000.hrcx", testData)

	b, err := backend.Open("qr://"+qrDir+"?format=svg", nil)
	if err != nil {
		t.Fatalf("opening QR backend: %v", err)
	}

	ctx := context.Background()
	if err := b.Upload(ctx, shardPath, "test.txt.000.hrcx"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	svgPath := filepath.Join(qrDir, "test.txt.000.svg")
	if _, err := os.Stat(svgPath); err != nil {
		t.Fatalf("SVG not created: %v", err)
	}

	// Verify it lists as .hrcx
	files, err := b.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].Key != "test.txt.000.hrcx" {
		t.Errorf("unexpected list result: %+v", files)
	}
}

func TestQRBackendOversizedShard(t *testing.T) {
	srcDir := t.TempDir()
	qrDir := t.TempDir()

	// Create a shard larger than QR capacity
	bigData := make([]byte, 3000)
	shardPath := createTestShard(t, srcDir, "big.000.hrcx", bigData)

	b, err := backend.Open("qr://"+qrDir, nil)
	if err != nil {
		t.Fatalf("opening QR backend: %v", err)
	}

	err = b.Upload(context.Background(), shardPath, "big.000.hrcx")
	if err == nil {
		t.Fatal("expected error for oversized shard, got nil")
	}
}

func TestQRBackendNotFoundDownload(t *testing.T) {
	qrDir := t.TempDir()
	destDir := t.TempDir()

	b, err := backend.Open("qr://"+qrDir, nil)
	if err != nil {
		t.Fatalf("opening QR backend: %v", err)
	}

	err = b.Download(context.Background(), "nonexistent.hrcx", filepath.Join(destDir, "out.hrcx"))
	if err == nil {
		t.Fatal("expected error for nonexistent shard, got nil")
	}
}
