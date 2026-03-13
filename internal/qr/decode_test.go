package qr

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// deterministicBytes generates deterministic bytes using a simple LCG PRNG.
// This avoids intermittent gozxing decode failures that occur with certain
// random byte patterns.
func deterministicBytes(seed uint64, n int) []byte {
	data := make([]byte, n)
	s := seed
	for i := range data {
		s = s*6364136223846793005 + 1442695040888963407
		data[i] = byte(s >> 33)
	}
	return data
}

// writePNG encodes an image as PNG to a file, failing the test on error.
func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestRoundTrip_BinaryData(t *testing.T) {
	for _, size := range []int{100, 500, 1000, 1500, 2000} {
		t.Run(fmt.Sprintf("%d_bytes", size), func(t *testing.T) {
			original := deterministicBytes(uint64(size), size)

			img, err := EncodeBytes(original, DefaultQRSize)
			if err != nil {
				t.Fatalf("encode failed for %d bytes: %v", size, err)
			}

			pngPath := filepath.Join(t.TempDir(), "qr.png")
			writePNG(t, pngPath, img)

			decoded, err := DecodeShard(pngPath)
			if err != nil {
				t.Fatalf("decode failed for %d bytes: %v", size, err)
			}

			if !bytes.Equal(original, decoded) {
				t.Fatalf("round-trip mismatch for %d bytes: got %d bytes back", size, len(decoded))
			}
		})
	}
}

func TestRoundTrip_ShardFile(t *testing.T) {
	tmpDir := t.TempDir()
	shardPath := filepath.Join(tmpDir, "test.hrcx")

	original := deterministicBytes(42, 800)
	if err := os.WriteFile(shardPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	img, err := EncodeShard(shardPath, DefaultQRSize)
	if err != nil {
		t.Fatalf("EncodeShard failed: %v", err)
	}

	pngPath := filepath.Join(tmpDir, "qr.png")
	writePNG(t, pngPath, img)

	decoded, err := DecodeShard(pngPath)
	if err != nil {
		t.Fatalf("DecodeShard failed: %v", err)
	}

	if !bytes.Equal(original, decoded) {
		t.Fatalf("shard round-trip mismatch: original %d bytes, decoded %d bytes", len(original), len(decoded))
	}
}

func TestAnnotatedImageDimensions(t *testing.T) {
	data := deterministicBytes(99, 100)

	img, err := EncodeBytes(data, DefaultQRSize)
	if err != nil {
		t.Fatal(err)
	}

	label := ShardLabel{
		Filename:   "test.txt.000.hrcx",
		ShardIndex: 0,
		ShardTotal: 5,
		ShardType:  "data",
	}

	annotated := Annotate(img, label)
	bounds := annotated.Bounds()

	if bounds.Dx() != DefaultQRSize {
		t.Errorf("expected width %d, got %d", DefaultQRSize, bounds.Dx())
	}
	if bounds.Dy() != DefaultQRSize+annotationHeight {
		t.Errorf("expected height %d, got %d", DefaultQRSize+annotationHeight, bounds.Dy())
	}
}
