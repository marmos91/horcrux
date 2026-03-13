package pipeline

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/marmos91/horcrux/internal/shard"
)

// splitTestFile creates a test file, splits it, and returns the shard directory.
func splitTestFile(t *testing.T, content string, dataShards, parityShards int, withManifest bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(inputPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	shardDir := filepath.Join(tmpDir, "shards")
	result, err := Split(SplitOptions{
		InputFile:    inputPath,
		OutputDir:    shardDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		NoEncrypt:    true,
		NoManifest:   !withManifest,
	})
	if err != nil {
		t.Fatal(err)
	}

	if withManifest {
		if err := SaveManifest(result, shardDir); err != nil {
			t.Fatal(err)
		}
	}
	return shardDir
}

// corruptShardPayload overwrites bytes in the payload section of a shard file.
func corruptShardPayload(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(int64(shard.HeaderSize)+2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0xFF, 0xFF, 0xFF}); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_AllValid(t *testing.T) {
	shardDir := splitTestFile(t, "hello world verify test", 3, 2, false)

	r, err := Verify(shardDir)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Recoverable {
		t.Error("expected recoverable")
	}
	if r.ShardsValid != 5 {
		t.Errorf("expected 5 valid shards, got %d", r.ShardsValid)
	}
	if r.ShardsCorrupt != 0 {
		t.Errorf("expected 0 corrupt, got %d", r.ShardsCorrupt)
	}
	if r.ShardsMissing != 0 {
		t.Errorf("expected 0 missing, got %d", r.ShardsMissing)
	}
	if r.OriginalName != "testfile.txt" {
		t.Errorf("expected original name testfile.txt, got %s", r.OriginalName)
	}
	if r.DataShards != 3 || r.ParityShards != 2 {
		t.Errorf("unexpected shard counts: %d+%d", r.DataShards, r.ParityShards)
	}
}

func TestVerify_CorruptPayload(t *testing.T) {
	shardDir := splitTestFile(t, "corrupt payload test data here", 3, 2, false)

	corruptShardPayload(t, filepath.Join(shardDir, "testfile.txt.001.hrcx"))

	r, err := Verify(shardDir)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Recoverable {
		t.Error("expected recoverable with 1 corrupt shard")
	}
	if r.ShardsCorrupt != 1 {
		t.Errorf("expected 1 corrupt, got %d", r.ShardsCorrupt)
	}
	if r.ShardsValid != 4 {
		t.Errorf("expected 4 valid, got %d", r.ShardsValid)
	}
	if len(r.CorruptIndices) != 1 || r.CorruptIndices[0] != 1 {
		t.Errorf("expected corrupt index [1], got %v", r.CorruptIndices)
	}
}

func TestVerify_MissingShard(t *testing.T) {
	shardDir := splitTestFile(t, "missing shard test data", 3, 2, false)

	if err := os.Remove(filepath.Join(shardDir, "testfile.txt.004.hrcx")); err != nil {
		t.Fatal(err)
	}

	r, err := Verify(shardDir)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Recoverable {
		t.Error("expected recoverable with 1 missing shard")
	}
	if r.ShardsMissing != 1 {
		t.Errorf("expected 1 missing, got %d", r.ShardsMissing)
	}
	if r.ShardsFound != 4 {
		t.Errorf("expected 4 found, got %d", r.ShardsFound)
	}
	if len(r.MissingIndices) != 1 || r.MissingIndices[0] != 4 {
		t.Errorf("expected missing index [4], got %v", r.MissingIndices)
	}
}

func TestVerify_TooManyMissing(t *testing.T) {
	shardDir := splitTestFile(t, "too many missing test", 5, 3, false)

	for _, idx := range []string{"001", "003", "005", "007"} {
		if err := os.Remove(filepath.Join(shardDir, "testfile.txt."+idx+".hrcx")); err != nil {
			t.Fatal(err)
		}
	}

	r, err := Verify(shardDir)
	if err != nil {
		t.Fatal(err)
	}

	if r.Recoverable {
		t.Error("expected NOT recoverable")
	}
	if r.ShardsValid != 4 {
		t.Errorf("expected 4 valid, got %d", r.ShardsValid)
	}
	if r.ShardsMissing != 4 {
		t.Errorf("expected 4 missing, got %d", r.ShardsMissing)
	}
}

func TestVerify_WithManifest(t *testing.T) {
	shardDir := splitTestFile(t, "manifest verify test data", 3, 2, true)

	r, err := Verify(shardDir)
	if err != nil {
		t.Fatal(err)
	}

	if !r.ManifestFound {
		t.Error("expected manifest to be found")
	}
	if !r.Recoverable {
		t.Error("expected recoverable")
	}

	// All present shards should have manifest hash OK
	for _, st := range r.ShardStatuses {
		if st.Path == "" {
			continue
		}
		if st.ManifestHashOK == nil {
			t.Errorf("shard %d: expected manifest hash check, got nil", st.Index)
		} else if !*st.ManifestHashOK {
			t.Errorf("shard %d: expected manifest hash OK", st.Index)
		}
	}
}

func TestVerify_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := Verify(tmpDir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestVerifyBatch(t *testing.T) {
	tmpDir := t.TempDir()
	batchRoot := filepath.Join(tmpDir, "batch")

	// Create two shard sets under batch root
	for _, name := range []string{"file1.txt", "file2.txt"} {
		inputPath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(inputPath, []byte("content of "+name), 0644); err != nil {
			t.Fatal(err)
		}
		shardDir := filepath.Join(batchRoot, name)
		_, err := Split(SplitOptions{
			InputFile:    inputPath,
			OutputDir:    shardDir,
			DataShards:   3,
			ParityShards: 2,
			NoEncrypt:    true,
			NoManifest:   true,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	results, err := VerifyBatch(batchRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if !r.Recoverable {
			t.Errorf("%s: expected recoverable", r.RelPath)
		}
		if r.ShardsValid != 5 {
			t.Errorf("%s: expected 5 valid, got %d", r.RelPath, r.ShardsValid)
		}
	}
}
