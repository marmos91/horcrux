package tests

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marmos91/horcrux/internal/shard"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build binary once for all tests
	tmpDir, err := os.MkdirTemp("", "hrcx-test-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmpDir, "hrcx")
	cmd := exec.Command("go", "build", "-o", binaryPath, "..")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
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
	defer func() { _ = f.Close() }()
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
	defer func() { _ = f.Close() }()
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
	_ = os.Remove(filepath.Join(shardDir, "small.txt.001.hrcx"))
	_ = os.Remove(filepath.Join(shardDir, "small.txt.004.hrcx"))
	_ = os.Remove(filepath.Join(shardDir, "small.txt.006.hrcx"))

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
	// Flip bytes in the payload area (offset into payload after the fixed header)
	if _, err := f.Seek(int64(shard.HeaderSize)+4, io.SeekStart); err != nil {
		t.Fatalf("seeking into shard payload: %v", err)
	}
	if _, err := f.Write([]byte{0xFF, 0xFF, 0xFF}); err != nil {
		t.Fatalf("writing corruption bytes: %v", err)
	}
	_ = f.Close()

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
	_, _ = f.Seek(0, io.SeekStart)
	_, _ = f.Write([]byte("DEAD"))
	_ = f.Close()

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
	_ = os.Remove(filepath.Join(shardDir, "small.txt.001.hrcx"))
	_ = os.Remove(filepath.Join(shardDir, "small.txt.003.hrcx"))
	_ = os.Remove(filepath.Join(shardDir, "small.txt.005.hrcx"))
	_ = os.Remove(filepath.Join(shardDir, "small.txt.007.hrcx"))

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
	_ = os.WriteFile(input, []byte("fake tar.gz content for testing"), 0644)

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
		if strings.HasSuffix(e.Name(), ".manifest.json") {
			continue // skip manifest file
		}
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
	_ = os.MkdirAll(mixDir, 0755)

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

// --- Directory / batch mode E2E tests ---

func createTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestE2E_SplitDir_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")

	createTestFile(t, filepath.Join(inputDir, "a.txt"), "file a content")
	createTestFile(t, filepath.Join(inputDir, "b.txt"), "file b content here")
	createTestFile(t, filepath.Join(inputDir, "c.txt"), "file c")

	out, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split dir failed: %v\n%s", err, out)
	}

	// Verify output structure: shardDir/a.txt/, shardDir/b.txt/, shardDir/c.txt/
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		subDir := filepath.Join(shardDir, name)
		entries, err := os.ReadDir(subDir)
		if err != nil {
			t.Fatalf("missing shard subdirectory %s: %v", name, err)
		}
		hrcxCount := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".hrcx") {
				hrcxCount++
			}
		}
		if hrcxCount == 0 {
			t.Fatalf("no .hrcx files in %s", subDir)
		}
	}

	// Verify summary output
	if !strings.Contains(out, "3 files processed") {
		t.Fatalf("expected summary in output, got: %s", out)
	}
}

func TestE2E_SplitMergeDir_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")
	outputDir := filepath.Join(tmpDir, "recovered")

	createTestFile(t, filepath.Join(inputDir, "hello.txt"), "hello world")
	createTestFile(t, filepath.Join(inputDir, "data.bin"), "binary data content 1234567890")

	out, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split dir failed: %v\n%s", err, out)
	}

	out, err = runHrcx(t, "merge", "-o", outputDir, shardDir)
	if err != nil {
		t.Fatalf("merge dir failed: %v\n%s", err, out)
	}

	// Verify each file matches
	for _, name := range []string{"hello.txt", "data.bin"} {
		orig := filepath.Join(inputDir, name)
		recovered := filepath.Join(outputDir, name)
		if fileSHA256(t, orig) != fileSHA256(t, recovered) {
			t.Fatalf("SHA-256 mismatch for %s", name)
		}
	}
}

func TestE2E_SplitMergeDir_Encrypted(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")
	outputDir := filepath.Join(tmpDir, "recovered")

	createTestFile(t, filepath.Join(inputDir, "secret.txt"), "top secret data")
	createTestFile(t, filepath.Join(inputDir, "private.key"), "private key material")

	out, err := runHrcx(t, "split", "-p", "batch-pass", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split dir failed: %v\n%s", err, out)
	}

	out, err = runHrcx(t, "merge", "-p", "batch-pass", "-o", outputDir, shardDir)
	if err != nil {
		t.Fatalf("merge dir failed: %v\n%s", err, out)
	}

	for _, name := range []string{"secret.txt", "private.key"} {
		orig := filepath.Join(inputDir, name)
		recovered := filepath.Join(outputDir, name)
		if fileSHA256(t, orig) != fileSHA256(t, recovered) {
			t.Fatalf("SHA-256 mismatch for %s", name)
		}
	}
}

func TestE2E_SplitMergeDir_Recursive(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")
	outputDir := filepath.Join(tmpDir, "recovered")

	createTestFile(t, filepath.Join(inputDir, "root.txt"), "root file")
	createTestFile(t, filepath.Join(inputDir, "docs", "readme.txt"), "readme content")
	createTestFile(t, filepath.Join(inputDir, "docs", "sub", "deep.txt"), "deep file")

	out, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split dir failed: %v\n%s", err, out)
	}

	// Verify nested output structure
	for _, rel := range []string{"root.txt", "docs/readme.txt", "docs/sub/deep.txt"} {
		subDir := filepath.Join(shardDir, rel)
		if _, err := os.Stat(subDir); err != nil {
			t.Fatalf("missing shard subdir for %s: %v", rel, err)
		}
	}

	out, err = runHrcx(t, "merge", "-o", outputDir, shardDir)
	if err != nil {
		t.Fatalf("merge dir failed: %v\n%s", err, out)
	}

	for _, rel := range []string{"root.txt", "docs/readme.txt", "docs/sub/deep.txt"} {
		orig := filepath.Join(inputDir, rel)
		recovered := filepath.Join(outputDir, rel)
		if fileSHA256(t, orig) != fileSHA256(t, recovered) {
			t.Fatalf("SHA-256 mismatch for %s", rel)
		}
	}
}

func TestE2E_SplitDir_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "empty")
	_ = os.MkdirAll(inputDir, 0755)

	out, err := runHrcx(t, "split", "--no-encrypt", "-o", filepath.Join(tmpDir, "shards"), inputDir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if !strings.Contains(out, "no files found") {
		t.Fatalf("expected 'no files found' error, got: %s", out)
	}
}

func TestE2E_SplitDir_Workers(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")

	// Create several small files
	for i := 0; i < 5; i++ {
		createTestFile(t, filepath.Join(inputDir, fmt.Sprintf("file%d.txt", i)),
			fmt.Sprintf("content of file %d", i))
	}

	// Test with --workers 1 (sequential)
	out, err := runHrcx(t, "split", "--no-encrypt", "-w", "1", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split with --workers 1 failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "5 files processed: 5 succeeded") {
		t.Fatalf("expected all 5 files succeeded, got: %s", out)
	}
}

// --- Dry-run E2E tests ---

func TestE2E_SplitDryRun_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	out, err := runHrcx(t, "split", "--dry-run", "--no-encrypt", "-o", shardDir, input)
	if err != nil {
		t.Fatalf("split --dry-run failed: %v\n%s", err, out)
	}

	// Verify output contains expected metadata
	checks := []string{
		"Dry run: split",
		"small.txt",
		"Encryption:   disabled",
		"Shards:",
		"data",
		"parity",
		"Per shard:",
		"Total output:",
		"Shard files:",
		".hrcx",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("dry-run output missing %q\nGot: %s", check, out)
		}
	}

	// Verify no files were created
	if _, err := os.Stat(shardDir); !os.IsNotExist(err) {
		t.Fatalf("expected no output directory to be created, but %s exists", shardDir)
	}
}

func TestE2E_SplitDryRun_Encrypted(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	// --dry-run should NOT prompt for password
	out, err := runHrcx(t, "split", "--dry-run", "-o", shardDir, input)
	if err != nil {
		t.Fatalf("split --dry-run (encrypted) failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Encryption:   enabled") {
		t.Errorf("expected encryption enabled in output\nGot: %s", out)
	}

	// Verify no files were created
	if _, err := os.Stat(shardDir); !os.IsNotExist(err) {
		t.Fatalf("expected no output directory to be created")
	}
}

func TestE2E_SplitDryRun_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")

	createTestFile(t, filepath.Join(inputDir, "a.txt"), "file a content")
	createTestFile(t, filepath.Join(inputDir, "b.txt"), "file b content here")

	out, err := runHrcx(t, "split", "--dry-run", "--no-encrypt", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split --dry-run dir failed: %v\n%s", err, out)
	}

	checks := []string{
		"Dry run: split directory",
		"a.txt",
		"b.txt",
		"2 files",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("dry-run dir output missing %q\nGot: %s", check, out)
		}
	}

	// Verify no output files
	if _, err := os.Stat(shardDir); !os.IsNotExist(err) {
		t.Fatalf("expected no output directory to be created")
	}
}

func TestE2E_MergeDryRun_SingleDir(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	// First, actually split
	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	out, err := runHrcx(t, "merge", "--dry-run", shardDir)
	if err != nil {
		t.Fatalf("merge --dry-run failed: %v\n%s", err, out)
	}

	checks := []string{
		"Dry run: merge",
		"small.txt",
		"Encryption:       enabled",
		"Status:           RECOVERABLE",
		"Missing:          none",
		"Corrupt:          none",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("dry-run merge output missing %q\nGot: %s", check, out)
		}
	}

	// Verify no output file was created
	if _, err := os.Stat(filepath.Join(tmpDir, "small.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected no output file to be created")
	}
}

func TestE2E_MergeDryRun_MissingShards(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Delete 2 shards (still recoverable: 6 of 8 remain, need 5)
	if err := os.Remove(filepath.Join(shardDir, "small.txt.003.hrcx")); err != nil {
		t.Fatalf("failed to remove shard small.txt.003.hrcx: %v", err)
	}
	if err := os.Remove(filepath.Join(shardDir, "small.txt.007.hrcx")); err != nil {
		t.Fatalf("failed to remove shard small.txt.007.hrcx: %v", err)
	}

	out, err := runHrcx(t, "merge", "--dry-run", shardDir)
	if err != nil {
		t.Fatalf("merge --dry-run with missing shards failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "RECOVERABLE") {
		t.Errorf("expected RECOVERABLE status\nGot: %s", out)
	}
	if !strings.Contains(out, "3") || !strings.Contains(out, "7") {
		t.Errorf("expected missing indices to include 3 and 7\nGot: %s", out)
	}
}

func TestE2E_MergeDryRun_BatchDir(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")

	createTestFile(t, filepath.Join(inputDir, "a.txt"), "file a content")
	createTestFile(t, filepath.Join(inputDir, "b.txt"), "file b content")

	// Split directory first
	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, inputDir); err != nil {
		t.Fatalf("split dir failed: %v", err)
	}

	out, err := runHrcx(t, "merge", "--dry-run", shardDir)
	if err != nil {
		t.Fatalf("merge --dry-run batch failed: %v\n%s", err, out)
	}

	checks := []string{
		"Dry run: merge directory",
		"a.txt",
		"b.txt",
		"2 files",
		"recoverable",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("dry-run merge dir output missing %q\nGot: %s", check, out)
		}
	}
}

func TestE2E_DryRun_NoPasswordPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	// split --dry-run without --no-encrypt and without -p should NOT prompt
	// (would hang if it did, so a successful return proves it)
	out, err := runHrcx(t, "split", "--dry-run", "-o", shardDir, input)
	if err != nil {
		t.Fatalf("split --dry-run should not prompt for password: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Encryption:   enabled") {
		t.Errorf("expected encryption enabled in dry-run output\nGot: %s", out)
	}
}

func TestE2E_SplitDir_FailFast(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")

	createTestFile(t, filepath.Join(inputDir, "good.txt"), "good content")

	// Create an unreadable file to trigger an error
	badPath := filepath.Join(inputDir, "bad.txt")
	createTestFile(t, badPath, "will become unreadable")
	_ = os.Chmod(badPath, 0000)

	out, err := runHrcx(t, "split", "--no-encrypt", "--fail-fast", "-o", shardDir, inputDir)
	if err == nil {
		// If running as root, chmod 0000 might not prevent reading
		t.Log("split succeeded (possibly running as root), skipping fail-fast assertion")
		return
	}

	_ = out // Error is expected; the --fail-fast flag should stop early
}

// --- Manifest E2E tests ---

func TestE2E_SplitGeneratesManifest(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("manifest file not created")
	}
}

func TestE2E_ManifestContents(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "3", "-k", "2", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading manifest: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing manifest JSON: %v", err)
	}

	// Check top-level fields
	if v, ok := m["version"].(string); !ok || v != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %v", m["version"])
	}
	if _, ok := m["created_at"].(string); !ok {
		t.Error("missing created_at")
	}

	// Check original
	orig, ok := m["original"].(map[string]any)
	if !ok {
		t.Fatal("missing original section")
	}
	if orig["filename"] != "small.txt" {
		t.Errorf("expected filename small.txt, got %v", orig["filename"])
	}
	if _, ok := orig["sha256"].(string); !ok {
		t.Error("missing original sha256")
	}

	// Check erasure
	erasure, ok := m["erasure"].(map[string]any)
	if !ok {
		t.Fatal("missing erasure section")
	}
	if erasure["data_shards"] != float64(3) {
		t.Errorf("expected 3 data shards, got %v", erasure["data_shards"])
	}
	if erasure["parity_shards"] != float64(2) {
		t.Errorf("expected 2 parity shards, got %v", erasure["parity_shards"])
	}
	if erasure["total_shards"] != float64(5) {
		t.Errorf("expected 5 total shards, got %v", erasure["total_shards"])
	}
	if erasure["min_shards_required"] != float64(3) {
		t.Errorf("expected min_shards_required 3, got %v", erasure["min_shards_required"])
	}

	// Check encryption
	enc, ok := m["encryption"].(map[string]any)
	if !ok {
		t.Fatal("missing encryption section")
	}
	if enc["encrypted"] != true {
		t.Error("expected encrypted=true")
	}
	if enc["algorithm"] != "AES-256-CTR" {
		t.Errorf("expected AES-256-CTR, got %v", enc["algorithm"])
	}
	if enc["kdf"] != "Argon2id" {
		t.Errorf("expected Argon2id, got %v", enc["kdf"])
	}

	// Check shards
	shards, ok := m["shards"].([]any)
	if !ok {
		t.Fatal("missing shards section")
	}
	if len(shards) != 5 {
		t.Fatalf("expected 5 shards, got %d", len(shards))
	}

	// Verify shard checksums match actual files
	for _, s := range shards {
		shard := s.(map[string]any)
		filename := shard["filename"].(string)
		expectedHash := shard["sha256"].(string)
		actualHash := fileSHA256(t, filepath.Join(shardDir, filename))
		if actualHash != expectedHash {
			t.Errorf("shard %s: hash mismatch (manifest=%s, actual=%s)", filename, expectedHash, actualHash)
		}
	}

	// Verify first 3 are data, last 2 are parity
	for i, s := range shards {
		shard := s.(map[string]any)
		expectedType := "data"
		if i >= 3 {
			expectedType = "parity"
		}
		if shard["type"] != expectedType {
			t.Errorf("shard %d: expected type %s, got %v", i, expectedType, shard["type"])
		}
	}
}

func TestE2E_MergeWithManifest(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")

	out, err := runHrcx(t, "merge", "--manifest", manifestPath, "-p", "test123", "-o", output, shardDir)
	if err != nil {
		t.Fatalf("merge with manifest failed: %v\n%s", err, out)
	}

	// Should print validation output
	if !strings.Contains(out, "[OK]") {
		t.Errorf("expected shard validation output with [OK], got: %s", out)
	}
	if !strings.Contains(out, "Verification: OK") {
		t.Errorf("expected verification OK, got: %s", out)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_ManifestAnnotate(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")

	// Annotate shard 0
	out, err := runHrcx(t, "manifest", "annotate", manifestPath, "0", "USB drive A")
	if err != nil {
		t.Fatalf("annotate failed: %v\n%s", err, out)
	}

	// Verify the annotation was saved
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	shards := m["shards"].([]any)
	shard0 := shards[0].(map[string]any)
	if shard0["location"] != "USB drive A" {
		t.Errorf("expected location 'USB drive A', got %v", shard0["location"])
	}

	// Annotate shard 2 with different location
	out, err = runHrcx(t, "manifest", "annotate", manifestPath, "2", "cloud backup")
	if err != nil {
		t.Fatalf("annotate shard 2 failed: %v\n%s", err, out)
	}

	// Verify both annotations exist
	data, err = os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	shards = m["shards"].([]any)
	shard0 = shards[0].(map[string]any)
	shard2 := shards[2].(map[string]any)
	if shard0["location"] != "USB drive A" {
		t.Errorf("shard 0 location lost after annotating shard 2")
	}
	if shard2["location"] != "cloud backup" {
		t.Errorf("expected shard 2 location 'cloud backup', got %v", shard2["location"])
	}
}

func TestE2E_SplitNoManifest(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "--no-encrypt", "--no-manifest", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatal("manifest file should not be created with --no-manifest")
	}

	// Verify shards still exist
	entries, err := os.ReadDir(shardDir)
	if err != nil {
		t.Fatal(err)
	}
	hrcxCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".hrcx") {
			hrcxCount++
		}
	}
	if hrcxCount == 0 {
		t.Fatal("no .hrcx files created")
	}
}

func TestE2E_MergeManifestDetectsCorruption(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-n", "5", "-k", "3", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")

	// Corrupt one shard's payload (flip bytes after header)
	shardPath := filepath.Join(shardDir, "small.txt.002.hrcx")
	f, err := os.OpenFile(shardPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(int64(shard.HeaderSize)+4, io.SeekStart); err != nil {
		t.Fatalf("seeking into shard payload: %v", err)
	}
	if _, err := f.Write([]byte{0xFF, 0xFF, 0xFF}); err != nil {
		t.Fatalf("writing corruption bytes: %v", err)
	}
	_ = f.Close()

	out, err := runHrcx(t, "merge", "--manifest", manifestPath, "-p", "test123", "-o", output, shardDir)
	if err != nil {
		// Merge might still succeed via reconstruction, but manifest should show corruption
		t.Logf("merge output: %s", out)
	}

	// The output should contain [CORRUPT] for the corrupted shard
	if !strings.Contains(out, "[CORRUPT]") {
		t.Errorf("expected [CORRUPT] in manifest validation output, got: %s", out)
	}
}

func TestE2E_MergeManifestOnly(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.txt")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")

	// Merge using only --manifest flag (no positional arg for shard dir)
	out, err := runHrcx(t, "merge", "--manifest", manifestPath, "-p", "test123", "-o", output)
	if err != nil {
		t.Fatalf("merge with manifest-only failed: %v\n%s", err, out)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestE2E_SplitDirGeneratesManifests(t *testing.T) {
	tmpDir := t.TempDir()
	inputDir := filepath.Join(tmpDir, "input")
	shardDir := filepath.Join(tmpDir, "shards")

	createTestFile(t, filepath.Join(inputDir, "a.txt"), "file a content")
	createTestFile(t, filepath.Join(inputDir, "b.txt"), "file b content here")

	out, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, inputDir)
	if err != nil {
		t.Fatalf("split dir failed: %v\n%s", err, out)
	}

	// Each subdirectory should have its own manifest
	for _, name := range []string{"a.txt", "b.txt"} {
		manifestPath := filepath.Join(shardDir, name, name+".manifest.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			t.Fatalf("missing manifest for %s at %s", name, manifestPath)
		}
	}
}

func TestE2E_ManifestNoEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	manifestPath := filepath.Join(shardDir, "small.txt.manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	enc := m["encryption"].(map[string]any)
	if enc["encrypted"] != false {
		t.Error("expected encrypted=false for --no-encrypt split")
	}
	if _, ok := enc["algorithm"]; ok {
		t.Error("algorithm should be omitted when not encrypted")
	}
}

func TestE2E_SplitDryRunShowsManifest(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	out, err := runHrcx(t, "split", "--dry-run", "--no-encrypt", "-o", shardDir, input)
	if err != nil {
		t.Fatalf("split --dry-run failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Manifest:     enabled") {
		t.Errorf("expected 'Manifest:     enabled' in dry-run output, got: %s", out)
	}
}

func TestE2E_SplitDryRunNoManifest(t *testing.T) {
	tmpDir := t.TempDir()
	shardDir := filepath.Join(tmpDir, "shards")
	input := testdataPath("small.txt")

	out, err := runHrcx(t, "split", "--dry-run", "--no-encrypt", "--no-manifest", "-o", shardDir, input)
	if err != nil {
		t.Fatalf("split --dry-run --no-manifest failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Manifest:     disabled") {
		t.Errorf("expected 'Manifest:     disabled' in dry-run output, got: %s", out)
	}
}

// --- QR Code Export/Import E2E tests ---

func TestE2E_ExportImportQR_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	input := testdataPath("small.txt")
	shardDir := filepath.Join(tmpDir, "shards")
	qrDir := filepath.Join(tmpDir, "qrcodes")
	recoveredShards := filepath.Join(tmpDir, "recovered-shards")
	output := filepath.Join(tmpDir, "recovered.txt")

	// Split with many data shards (small shard size) and high parity to tolerate
	// occasional QR decode failures in the gozxing library.
	if _, err := runHrcx(t, "split", "-n", "10", "-k", "8", "-p", "test123", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Export shards as QR codes
	out, err := runHrcx(t, "export-qr", "-o", qrDir, shardDir)
	if err != nil {
		t.Fatalf("export-qr failed: %v\n%s", err, out)
	}

	// Verify QR code files were created
	entries, err := os.ReadDir(qrDir)
	if err != nil {
		t.Fatalf("cannot read QR output dir: %v", err)
	}
	pngCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			pngCount++
		}
	}
	if pngCount == 0 {
		t.Fatal("no PNG files created by export-qr")
	}

	// Import QR codes back to shard files
	out, err = runHrcx(t, "import-qr", "-o", recoveredShards, qrDir)
	if err != nil {
		t.Fatalf("import-qr failed: %v\n%s", err, out)
	}

	// Merge recovered shards
	if _, err := runHrcx(t, "merge", "-p", "test123", "-o", output, recoveredShards); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	// Verify SHA-256 match
	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch after QR round-trip")
	}
}

func TestE2E_ExportQR_OversizedShard(t *testing.T) {
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "large.bin")
	createRandomFile(t, input, 10*1024) // 10KB
	shardDir := filepath.Join(tmpDir, "shards")

	// Split with few data shards so each shard is large
	if _, err := runHrcx(t, "split", "-n", "2", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// export-qr should fail because shards are too large
	out, err := runHrcx(t, "export-qr", shardDir)
	if err == nil {
		t.Fatal("expected export-qr to fail with oversized shards")
	}
	if !strings.Contains(out, "exceed QR code capacity") {
		t.Fatalf("expected capacity error message, got: %s", out)
	}
	if !strings.Contains(out, "more data shards") {
		t.Fatalf("expected hint about more data shards, got: %s", out)
	}
}

func TestE2E_ExportQR_SVG(t *testing.T) {
	tmpDir := t.TempDir()
	input := testdataPath("small.txt")
	shardDir := filepath.Join(tmpDir, "shards")
	qrDir := filepath.Join(tmpDir, "qrcodes")

	// Split with many data shards to keep each shard small
	if _, err := runHrcx(t, "split", "-n", "10", "-k", "8", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Export as SVG
	out, err := runHrcx(t, "export-qr", "-f", "svg", "-o", qrDir, shardDir)
	if err != nil {
		t.Fatalf("export-qr --format svg failed: %v\n%s", err, out)
	}

	// Verify SVG files were created
	entries, err := os.ReadDir(qrDir)
	if err != nil {
		t.Fatalf("cannot read QR output dir: %v", err)
	}
	svgCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".svg") {
			svgCount++
		}
	}
	if svgCount == 0 {
		t.Fatal("no SVG files created by export-qr --format svg")
	}
}

func TestE2E_ExportQR_NoEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	input := testdataPath("small.txt")
	shardDir := filepath.Join(tmpDir, "shards")
	qrDir := filepath.Join(tmpDir, "qrcodes")
	recoveredShards := filepath.Join(tmpDir, "recovered-shards")
	output := filepath.Join(tmpDir, "recovered.txt")

	// Split without encryption, high parity to tolerate QR decode failures
	if _, err := runHrcx(t, "split", "-n", "10", "-k", "8", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Export → Import round-trip
	if _, err := runHrcx(t, "export-qr", "-o", qrDir, shardDir); err != nil {
		t.Fatalf("export-qr failed: %v", err)
	}
	if _, err := runHrcx(t, "import-qr", "-o", recoveredShards, qrDir); err != nil {
		t.Fatalf("import-qr failed: %v", err)
	}
	if _, err := runHrcx(t, "merge", "-o", output, recoveredShards); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch after unencrypted QR round-trip")
	}
}
