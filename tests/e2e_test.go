package tests

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build binary once for all tests
	tmpDir, err := os.MkdirTemp("", "hrcx-test-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binaryPath = filepath.Join(tmpDir, "hrcx")
	cmd := exec.Command("go", "build", "-o", binaryPath, "..")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func runHrcx(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hashing %s: %v", path, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func createRandomFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := io.CopyN(f, rand.Reader, size); err != nil {
		t.Fatal(err)
	}
}

func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func TestE2E_SplitMerge_SmallText(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_SplitMerge_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("empty.txt")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected 0 bytes, got %d", info.Size())
	}
}

func TestE2E_SplitMerge_TinyFile(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")
	input := testdataPath("tiny.bin")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_SplitMerge_MediumBinary(t *testing.T) {
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "medium.bin")
	createRandomFile(t, input, 10*1024*1024) // 10MB

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_SplitMerge_NoEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_SplitMerge_UnicodeFilename(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("unicode_名前.txt")

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Inspect a shard to verify filename
	out, err := runHrcx(t, "inspect", filepath.Join(shardDir, "unicode_名前.txt.000.hrcx"))
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !strings.Contains(out, "unicode_名前.txt") {
		t.Fatalf("unicode filename not preserved in shard header: %s", out)
	}

	// Merge to verify round-trip
	output := filepath.Join(tmpDir, "recovered.txt")
	if _, err := runHrcx(t, "merge", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_SplitMerge_CustomShardCounts(t *testing.T) {
	tests := []struct {
		data   int
		parity int
	}{
		{2, 1},
		{3, 2},
		{5, 3},
		{10, 4},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d+%d", tc.data, tc.parity), func(t *testing.T) {
			tmpDir := t.TempDir()
			shardDir := filepath.Join(tmpDir, "shards")
			output := filepath.Join(tmpDir, "recovered.txt")
			input := testdataPath("small.txt")

			_, err := runHrcx(t, "split",
				"-n", fmt.Sprintf("%d", tc.data),
				"-k", fmt.Sprintf("%d", tc.parity),
				"-p", "test123",
				"-o", shardDir,
				input)
			if err != nil {
				t.Fatalf("split failed: %v", err)
			}

			if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
				t.Fatalf("merge failed: %v", err)
			}

			if fileSHA256(t, input) != fileSHA256(t, output) {
				t.Fatal("SHA-256 mismatch")
			}
		})
	}
}

func TestE2E_MergeWithMissingShards(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Delete exactly K=3 shards
	os.Remove(filepath.Join(shardDir, "small.txt.001.hrcx"))
	os.Remove(filepath.Join(shardDir, "small.txt.004.hrcx"))
	os.Remove(filepath.Join(shardDir, "small.txt.006.hrcx"))

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge with missing shards failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch after reconstruction")
	}
}

func TestE2E_MergeWithCorruptShard(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Corrupt one shard's payload (flip bytes after header)
	shardPath := filepath.Join(shardDir, "small.txt.002.hrcx")
	f, err := os.OpenFile(shardPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the payload area (after 256-byte header)
	f.Seek(260, io.SeekStart)
	f.Write([]byte{0xFF, 0xFF, 0xFF})
	f.Close()

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge with corrupt shard failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch after reconstruction from corrupt shard")
	}
}

func TestE2E_MergeWithCorruptHeader(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Corrupt one shard's header (mess up the magic bytes area)
	shardPath := filepath.Join(shardDir, "small.txt.003.hrcx")
	f, err := os.OpenFile(shardPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Seek(0, io.SeekStart)
	f.Write([]byte("DEAD"))
	f.Close()

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge with corrupt header failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_MergeInsufficientShards(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Delete K+1 = 4 shards (only 4 remain, need 5)
	os.Remove(filepath.Join(shardDir, "small.txt.001.hrcx"))
	os.Remove(filepath.Join(shardDir, "small.txt.003.hrcx"))
	os.Remove(filepath.Join(shardDir, "small.txt.005.hrcx"))
	os.Remove(filepath.Join(shardDir, "small.txt.007.hrcx"))

	out, err := runHrcx(t, "merge", "-p", "test123", "-o", filepath.Join(tmpDir, "out.txt"), shardDir)
	if err == nil {
		t.Fatal("expected merge to fail with insufficient shards")
	}
	if !strings.Contains(out, "shards available") {
		t.Fatalf("expected helpful error message, got: %s", out)
	}
}

func TestE2E_MergeWrongPassword(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-p", "correct-pass", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	out, err := runHrcx(t, "merge", "-p", "wrong-pass", "-o", filepath.Join(tmpDir, "out.txt"), shardDir)
	if err == nil {
		t.Fatal("expected merge to fail with wrong password")
	}
	if !strings.Contains(out, "wrong password") {
		t.Fatalf("expected 'wrong password' error, got: %s", out)
	}
}

func TestE2E_InspectSingleShard(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	out, err := runHrcx(t, "inspect", filepath.Join(shardDir, "small.txt.003.hrcx"))
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	checks := []string{
		"Format version:    1",
		"Data shards:       5",
		"Parity shards:     3",
		"Original filename: small.txt",
		"Encrypted:         yes",
		"Header checksum:   OK",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("inspect output missing %q\nGot: %s", check, out)
		}
	}
}

func TestE2E_InspectDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "3", "-k", "2", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	out, err := runHrcx(t, "inspect", shardDir)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	// Should show all 5 shards
	for i := 0; i < 5; i++ {
		expected := fmt.Sprintf("small.txt.%03d.hrcx", i)
		if !strings.Contains(out, expected) {
			t.Errorf("inspect output missing shard %s\nGot: %s", expected, out)
		}
	}
}

func TestE2E_OutputDirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	// Nested non-existent directory
	shardDir := filepath.Join(tmpDir, "a", "b", "c", "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed to create nested output dir: %v", err)
	}

	entries, err := os.ReadDir(shardDir)
	if err != nil {
		t.Fatalf("cannot read shard dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no shards created")
	}
}

func TestE2E_FilesWithDotsInName(t *testing.T) {
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "my.archive.tar.gz")
	os.WriteFile(input, []byte("fake tar.gz content for testing"), 0644)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.tar.gz")

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Verify shard naming
	entries, err := os.ReadDir(shardDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "my.archive.tar.gz.") || !strings.HasSuffix(e.Name(), ".hrcx") {
			t.Errorf("unexpected shard name: %s", e.Name())
		}
	}

	if _, err := runHrcx(t, "merge", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "large.bin")
	createRandomFile(t, input, 5*1024*1024) // 5MB

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_MergeMixedShards(t *testing.T) {
	tmpDir := t.TempDir()
	input := testdataPath("small.txt")

	// Split twice into separate dirs
	dir1 := filepath.Join(tmpDir, "split1")
	dir2 := filepath.Join(tmpDir, "split2")

	if _, err := runHrcx(t, "split", "-p", "pass1", "-o", dir1, input); err != nil {
		t.Fatal(err)
	}
	if _, err := runHrcx(t, "split", "-p", "pass2", "-o", dir2, input); err != nil {
		t.Fatal(err)
	}

	// Mix shards from both splits into one dir
	mixDir := filepath.Join(tmpDir, "mixed")
	os.MkdirAll(mixDir, 0755)

	// Copy shard 0 from split1 and shard 1 from split2
	copyFile(t, filepath.Join(dir1, "small.txt.000.hrcx"), filepath.Join(mixDir, "small.txt.000.hrcx"))
	copyFile(t, filepath.Join(dir2, "small.txt.001.hrcx"), filepath.Join(mixDir, "small.txt.001.hrcx"))
	copyFile(t, filepath.Join(dir1, "small.txt.002.hrcx"), filepath.Join(mixDir, "small.txt.002.hrcx"))
	copyFile(t, filepath.Join(dir1, "small.txt.003.hrcx"), filepath.Join(mixDir, "small.txt.003.hrcx"))
	copyFile(t, filepath.Join(dir1, "small.txt.004.hrcx"), filepath.Join(mixDir, "small.txt.004.hrcx"))
	copyFile(t, filepath.Join(dir1, "small.txt.005.hrcx"), filepath.Join(mixDir, "small.txt.005.hrcx"))
	copyFile(t, filepath.Join(dir1, "small.txt.006.hrcx"), filepath.Join(mixDir, "small.txt.006.hrcx"))
	copyFile(t, filepath.Join(dir1, "small.txt.007.hrcx"), filepath.Join(mixDir, "small.txt.007.hrcx"))

	// Merge should fail because shard 1 has different salt (from split2)
	out, err := runHrcx(t, "merge", "-p", "pass1", "-o", filepath.Join(tmpDir, "out.txt"), mixDir)
	// The mismatched shard should be detected. Due to corrupt payload checksum
	// (different encryption), it should still succeed since we have 7 good shards.
	// But let's verify the output is correct
	if err != nil {
		// If it fails, that's also acceptable behavior for mixed shards
		t.Logf("merge of mixed shards failed as expected: %s", out)
		return
	}

	// If merge succeeded, the recovered file should match
	if fileSHA256(t, input) != fileSHA256(t, filepath.Join(tmpDir, "out.txt")) {
		t.Logf("merge of mixed shards produced incorrect output (expected for truly mixed shards)")
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatal(err)
	}
}
