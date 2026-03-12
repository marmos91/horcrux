package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func benchmarkSplit(b *testing.B, size int64, encrypt bool) {
	b.Helper()
	tmpDir := b.TempDir()
	input := filepath.Join(tmpDir, "bench.bin")
	createRandomFileNoHelper(input, size)

	b.SetBytes(size)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shardDir := filepath.Join(tmpDir, fmt.Sprintf("shards_%d", i))
		args := []string{"split", "-o", shardDir}
		if encrypt {
			args = append(args, "-p", "benchpass")
		} else {
			args = append(args, "--no-encrypt")
		}
		args = append(args, input)

		cmd := execCommand(binaryPath, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("split failed: %v\n%s", err, out)
		}
	}
}

func benchmarkMerge(b *testing.B, size int64, encrypt bool) {
	b.Helper()
	tmpDir := b.TempDir()
	input := filepath.Join(tmpDir, "bench.bin")
	createRandomFileNoHelper(input, size)

	shardDir := filepath.Join(tmpDir, "shards")
	splitArgs := []string{"split", "-o", shardDir}
	if encrypt {
		splitArgs = append(splitArgs, "-p", "benchpass")
	} else {
		splitArgs = append(splitArgs, "--no-encrypt")
	}
	splitArgs = append(splitArgs, input)

	cmd := execCommand(binaryPath, splitArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("split failed: %v\n%s", err, out)
	}

	b.SetBytes(size)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		output := filepath.Join(tmpDir, fmt.Sprintf("recovered_%d.bin", i))
		mergeArgs := []string{"merge", "-o", output}
		if encrypt {
			mergeArgs = append(mergeArgs, "-p", "benchpass")
		}
		mergeArgs = append(mergeArgs, shardDir)

		cmd := execCommand(binaryPath, mergeArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("merge failed: %v\n%s", err, out)
		}
		os.Remove(output)
	}
}

func BenchmarkSplit_1MB(b *testing.B)   { benchmarkSplit(b, 1<<20, true) }
func BenchmarkSplit_10MB(b *testing.B)  { benchmarkSplit(b, 10<<20, true) }
func BenchmarkSplit_100MB(b *testing.B) { benchmarkSplit(b, 100<<20, true) }

func BenchmarkMerge_1MB(b *testing.B)   { benchmarkMerge(b, 1<<20, true) }
func BenchmarkMerge_10MB(b *testing.B)  { benchmarkMerge(b, 10<<20, true) }
func BenchmarkMerge_100MB(b *testing.B) { benchmarkMerge(b, 100<<20, true) }

func BenchmarkSplitMerge_Encrypted_100MB(b *testing.B) {
	tmpDir := b.TempDir()
	input := filepath.Join(tmpDir, "bench.bin")
	createRandomFileNoHelper(input, 100<<20)

	b.SetBytes(100 << 20)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shardDir := filepath.Join(tmpDir, fmt.Sprintf("shards_%d", i))
		output := filepath.Join(tmpDir, fmt.Sprintf("recovered_%d.bin", i))

		cmd := execCommand(binaryPath, "split", "-p", "benchpass", "-o", shardDir, input)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("split failed: %v\n%s", err, out)
		}

		cmd = execCommand(binaryPath, "merge", "-p", "benchpass", "-o", output, shardDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("merge failed: %v\n%s", err, out)
		}

		os.Remove(output)
		os.RemoveAll(shardDir)
	}
}

func BenchmarkSplitMerge_NoEncrypt_100MB(b *testing.B) {
	tmpDir := b.TempDir()
	input := filepath.Join(tmpDir, "bench.bin")
	createRandomFileNoHelper(input, 100<<20)

	b.SetBytes(100 << 20)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shardDir := filepath.Join(tmpDir, fmt.Sprintf("shards_%d", i))
		output := filepath.Join(tmpDir, fmt.Sprintf("recovered_%d.bin", i))

		cmd := execCommand(binaryPath, "split", "--no-encrypt", "-o", shardDir, input)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("split failed: %v\n%s", err, out)
		}

		cmd = execCommand(binaryPath, "merge", "-o", output, shardDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("merge failed: %v\n%s", err, out)
		}

		os.Remove(output)
		os.RemoveAll(shardDir)
	}
}

func BenchmarkReconstruct_100MB_1Missing(b *testing.B) {
	benchmarkReconstruct(b, 100<<20, 1)
}

func BenchmarkReconstruct_100MB_KMissing(b *testing.B) {
	benchmarkReconstruct(b, 100<<20, 3)
}

func benchmarkReconstruct(b *testing.B, size int64, missingCount int) {
	b.Helper()
	tmpDir := b.TempDir()
	input := filepath.Join(tmpDir, "bench.bin")
	createRandomFileNoHelper(input, size)

	b.SetBytes(size)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shardDir := filepath.Join(tmpDir, fmt.Sprintf("shards_%d", i))
		output := filepath.Join(tmpDir, fmt.Sprintf("recovered_%d.bin", i))

		cmd := execCommand(binaryPath, "split", "--no-encrypt", "-o", shardDir, input)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("split failed: %v\n%s", err, out)
		}

		// Delete missingCount shards
		for j := 0; j < missingCount; j++ {
			os.Remove(filepath.Join(shardDir, fmt.Sprintf("bench.bin.%03d.hrcx", j)))
		}

		cmd = execCommand(binaryPath, "merge", "-o", output, shardDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("merge failed: %v\n%s", err, out)
		}

		os.Remove(output)
		os.RemoveAll(shardDir)
	}
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
