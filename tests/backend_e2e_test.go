package tests

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/marmos91/horcrux/internal/backend/local"
	"github.com/marmos91/horcrux/internal/manifest"
	"github.com/marmos91/horcrux/internal/pipeline"
)

// TestBackendE2E_SplitDistributeCollectMerge performs a full round-trip:
// split a file → distribute to file:// backends → collect from backends → merge.
func TestBackendE2E_SplitDistributeCollectMerge(t *testing.T) {
	// Create a test file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "secret.txt")
	content := []byte("This is secret data that should survive the full cloud backend round-trip.")
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	originalHash := sha256Hash(t, srcFile)

	// Create two "remote" directories to simulate two cloud backends
	backendA := t.TempDir()
	backendB := t.TempDir()

	// Step 1: Split locally
	splitDir := t.TempDir()
	result, err := pipeline.Split(pipeline.SplitOptions{
		InputFile:    srcFile,
		OutputDir:    splitDir,
		DataShards:   3,
		ParityShards: 2,
		Password:     "testpass",
	})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	if len(result.ShardFiles) != 5 {
		t.Fatalf("expected 5 shard files, got %d", len(result.ShardFiles))
	}

	// Step 2: Distribute round-robin to two file:// backends
	uriA := "file://" + backendA
	uriB := "file://" + backendB

	backends, err := pipeline.OpenBackends([]string{uriA, uriB}, nil)
	if err != nil {
		t.Fatalf("OpenBackends: %v", err)
	}

	distributed, err := pipeline.DistributeShards(context.Background(), result.ShardFiles, backends)
	if err != nil {
		t.Fatalf("DistributeShards: %v", err)
	}

	// Verify shards were distributed (locations populated)
	for i, sf := range distributed {
		if sf.Location == "" {
			t.Errorf("shard %d: Location is empty", i)
		}
		t.Logf("shard %d: Location=%s", i, sf.Location)
	}

	// Update result with distributed info and save manifest
	result.ShardFiles = distributed
	m := result.BuildManifest()
	manifestPath := filepath.Join(splitDir, manifest.ManifestFilename("secret.txt"))
	if err := m.Save(manifestPath); err != nil {
		t.Fatalf("Save manifest: %v", err)
	}

	// Verify shards exist on backends
	for i, sf := range distributed {
		expectedBackend := backendA
		if i%2 == 1 {
			expectedBackend = backendB
		}
		shardPath := filepath.Join(expectedBackend, sf.Filename)
		if _, err := os.Stat(shardPath); err != nil {
			t.Errorf("shard %d not found on expected backend: %v", i, err)
		}
	}

	// Step 3: Collect from manifest (simulates downloading from cloud)
	collectDir := t.TempDir()
	mf, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}

	if err := pipeline.CollectFromManifest(context.Background(), mf, collectDir); err != nil {
		t.Fatalf("CollectFromManifest: %v", err)
	}

	// Verify all shards were collected
	for _, entry := range mf.Shards {
		collectedPath := filepath.Join(collectDir, entry.Filename)
		if _, err := os.Stat(collectedPath); err != nil {
			t.Errorf("collected shard %d not found: %v", entry.Index, err)
		}
	}

	// Step 4: Merge from collected shards
	outputFile := filepath.Join(t.TempDir(), "recovered.txt")
	if err := pipeline.Merge(pipeline.MergeOptions{
		ShardDir:   collectDir,
		OutputFile: outputFile,
		Password:   "testpass",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Step 5: Verify output matches original
	recoveredHash := sha256Hash(t, outputFile)
	if originalHash != recoveredHash {
		t.Fatalf("hash mismatch: original=%s, recovered=%s", originalHash, recoveredHash)
	}

	recovered, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(recovered) != string(content) {
		t.Fatal("recovered content does not match original")
	}
}

// TestBackendE2E_CollectFromBackends tests the listing-based collection
// (without manifest) using file:// backends.
func TestBackendE2E_CollectFromBackends(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "data.bin")
	content := make([]byte, 1024*10) // 10KB
	for i := range content {
		content[i] = byte(i % 256)
	}
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	originalHash := sha256Hash(t, srcFile)

	// Split with no encryption for simplicity
	splitDir := t.TempDir()
	result, err := pipeline.Split(pipeline.SplitOptions{
		InputFile:    srcFile,
		OutputDir:    splitDir,
		DataShards:   2,
		ParityShards: 1,
		NoEncrypt:    true,
	})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	// Upload all to a single file:// backend
	remoteDir := t.TempDir()
	uri := "file://" + remoteDir

	backends, err := pipeline.OpenBackends([]string{uri}, nil)
	if err != nil {
		t.Fatalf("OpenBackends: %v", err)
	}

	if _, err := pipeline.DistributeShards(context.Background(), result.ShardFiles, backends); err != nil {
		t.Fatalf("DistributeShards: %v", err)
	}

	// Collect without manifest (listing-based)
	collectDir := t.TempDir()
	if err := pipeline.CollectFromBackends(context.Background(), []string{uri}, collectDir, nil); err != nil {
		t.Fatalf("CollectFromBackends: %v", err)
	}

	// Merge
	outputFile := filepath.Join(t.TempDir(), "recovered.bin")
	if err := pipeline.Merge(pipeline.MergeOptions{
		ShardDir:   collectDir,
		OutputFile: outputFile,
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	recoveredHash := sha256Hash(t, outputFile)
	if originalHash != recoveredHash {
		t.Fatalf("hash mismatch: original=%s, recovered=%s", originalHash, recoveredHash)
	}
}

// TestBackendE2E_CleanupLocalShards tests that local shards can be removed
// after distribution.
func TestBackendE2E_CleanupLocalShards(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "temp.txt")
	if err := os.WriteFile(srcFile, []byte("temporary"), 0o644); err != nil {
		t.Fatal(err)
	}

	splitDir := t.TempDir()
	result, err := pipeline.Split(pipeline.SplitOptions{
		InputFile:    srcFile,
		OutputDir:    splitDir,
		DataShards:   2,
		ParityShards: 1,
		NoEncrypt:    true,
	})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	remoteDir := t.TempDir()
	backends, err := pipeline.OpenBackends([]string{"file://" + remoteDir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	distributed, err := pipeline.DistributeShards(context.Background(), result.ShardFiles, backends)
	if err != nil {
		t.Fatal(err)
	}

	// All local shard files should still exist
	for _, sf := range distributed {
		if _, err := os.Stat(sf.Path); err != nil {
			t.Errorf("shard %d should still exist locally: %v", sf.Index, err)
		}
	}

	// Cleanup
	if err := pipeline.CleanupLocalShards(distributed); err != nil {
		t.Fatalf("CleanupLocalShards: %v", err)
	}

	// All local shard files should be gone
	for _, sf := range distributed {
		if _, err := os.Stat(sf.Path); !os.IsNotExist(err) {
			t.Errorf("shard %d should be deleted", sf.Index)
		}
	}
}

func sha256Hash(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
