package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/backend/local"
)

func TestLocalBackendRoundTrip(t *testing.T) {
	root := t.TempDir()
	b := local.New(root)
	ctx := context.Background()

	// Create a test file
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "test.hrcx")
	content := []byte("hello horcrux shard data")
	if err := os.WriteFile(srcPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Upload
	if err := b.Upload(ctx, srcPath, "test.hrcx"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Verify file exists on backend
	uploaded := filepath.Join(root, "test.hrcx")
	if _, err := os.Stat(uploaded); err != nil {
		t.Fatalf("uploaded file not found: %v", err)
	}

	// List
	files, err := b.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("List returned %d files, want 1", len(files))
	}
	if files[0].Key != "test.hrcx" {
		t.Errorf("Key = %q, want %q", files[0].Key, "test.hrcx")
	}

	// Download
	dstDir := t.TempDir()
	dstPath := filepath.Join(dstDir, "downloaded.hrcx")
	if err := b.Download(ctx, "test.hrcx", dstPath); err != nil {
		t.Fatalf("Download: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", got, content)
	}

	// Delete
	if err := b.Delete(ctx, "test.hrcx"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	if _, err := os.Stat(uploaded); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestLocalBackendDownloadNotFound(t *testing.T) {
	root := t.TempDir()
	b := local.New(root)

	err := b.Download(context.Background(), "nonexistent.hrcx", filepath.Join(t.TempDir(), "out"))
	if !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalBackendDeleteNotFound(t *testing.T) {
	root := t.TempDir()
	b := local.New(root)

	err := b.Delete(context.Background(), "nonexistent.hrcx")
	if !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalBackendListEmpty(t *testing.T) {
	root := t.TempDir()
	b := local.New(root)

	files, err := b.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestLocalBackendListFiltersNonHrcx(t *testing.T) {
	root := t.TempDir()
	b := local.New(root)
	ctx := context.Background()

	// Create a test file (not .hrcx)
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "readme.txt")
	if err := os.WriteFile(srcPath, []byte("not a shard"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.Upload(ctx, srcPath, "readme.txt"); err != nil {
		t.Fatal(err)
	}

	// Create a .hrcx file
	shardPath := filepath.Join(srcDir, "test.hrcx")
	if err := os.WriteFile(shardPath, []byte("shard"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.Upload(ctx, shardPath, "test.hrcx"); err != nil {
		t.Fatal(err)
	}

	files, err := b.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 .hrcx file, got %d", len(files))
	}
}
