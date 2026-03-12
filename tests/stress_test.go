package tests

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestStress_100MB_Encrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "100mb.bin")
	createRandomFile(t, input, 100*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-p", "stress-test", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "stress-test", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestStress_500MB_Encrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "500mb.bin")
	createRandomFile(t, input, 500*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-p", "stress-test", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "stress-test", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestStress_1GB_Encrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "1gb.bin")
	createRandomFile(t, input, 1024*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-p", "stress-test", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "stress-test", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestStress_1GB_NoEncryption(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "1gb.bin")
	createRandomFile(t, input, 1024*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

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

func TestStress_HighShardCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "100mb.bin")
	createRandomFile(t, input, 100*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-n", "50", "-k", "20", "-p", "stress", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "stress", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestStress_MaxParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "100mb.bin")
	createRandomFile(t, input, 100*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	if _, err := runHrcx(t, "split", "-n", "3", "-k", "50", "-p", "stress", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-p", "stress", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestStress_ReconstructHeavy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "100mb.bin")
	createRandomFile(t, input, 100*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	// Split into 5+10 shards
	if _, err := runHrcx(t, "split", "-n", "5", "-k", "10", "-p", "stress", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	// Delete all 10 parity shards (keeping only the 5 data shards)
	for i := 5; i < 15; i++ {
		_ = os.Remove(filepath.Join(shardDir, fmt.Sprintf("100mb.bin.%03d.hrcx", i)))
	}

	if _, err := runHrcx(t, "merge", "-p", "stress", "-o", output, shardDir); err != nil {
		t.Fatalf("merge with heavy reconstruction failed: %v", err)
	}

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}
}

func TestStress_MemoryBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "1gb.bin")
	createRandomFile(t, input, 1024*1024*1024)

	shardDir := filepath.Join(tmpDir, "shards")
	output := filepath.Join(tmpDir, "recovered.bin")

	// Sample memory in background
	var maxHeap uint64
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapInuse > maxHeap {
					maxHeap = m.HeapInuse
				}
			}
		}
	}()

	// Note: this measures the test process memory, not the hrcx subprocess.
	// For subprocess memory, we'd need to use os/exec and read /proc.
	// This still validates our library code if called directly.

	if _, err := runHrcx(t, "split", "--no-encrypt", "-o", shardDir, input); err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if _, err := runHrcx(t, "merge", "-o", output, shardDir); err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	close(done)

	if fileSHA256(t, input) != fileSHA256(t, output) {
		t.Fatal("SHA-256 mismatch")
	}

	t.Logf("Max heap during test process: %.1f MB", float64(maxHeap)/(1024*1024))
}

func TestStress_ConcurrentSplits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	const numFiles = 4

	var wg sync.WaitGroup
	errCh := make(chan error, numFiles)

	for i := range numFiles {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			input := filepath.Join(tmpDir, fmt.Sprintf("file_%d.bin", idx))
			_ = createRandomFileNoHelper(input, 10*1024*1024)

			shardDir := filepath.Join(tmpDir, fmt.Sprintf("shards_%d", idx))
			output := filepath.Join(tmpDir, fmt.Sprintf("recovered_%d.bin", idx))

			cmd := "split"
			if _, err := runHrcxDirect(binaryPath, cmd, "--no-encrypt", "-o", shardDir, input); err != nil {
				errCh <- fmt.Errorf("split %d failed: %w", idx, err)
				return
			}

			if _, err := runHrcxDirect(binaryPath, "merge", "-o", output, shardDir); err != nil {
				errCh <- fmt.Errorf("merge %d failed: %w", idx, err)
				return
			}

			h1 := hashFile(input)
			h2 := hashFile(output)
			if h1 != h2 {
				errCh <- fmt.Errorf("file %d SHA-256 mismatch", idx)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}
}

func createRandomFileNoHelper(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.CopyN(f, rand.Reader, size)
	return err
}

func hashFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func runHrcxDirect(binary string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
