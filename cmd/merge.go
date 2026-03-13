package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/marmos91/horcrux/internal/backend/all"
	"github.com/marmos91/horcrux/internal/manifest"
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/marmos91/horcrux/internal/progress"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge <shard-dir>",
	Short: "Reconstruct a file from shards",
	Long: `Reconstruct a file from shards. Tolerates up to K missing or corrupt shards.

If the shard directory contains subdirectories with .hrcx files (batch layout
produced by splitting a directory), all shard sets are merged in parallel.

Use --manifest to validate shard integrity before merging and verify the
reconstructed output against the original file hash.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runMerge,
}

var (
	mergeOutput   string
	mergePassword string
	mergeWorkers  int
	mergeFailFast bool
	mergeManifest string
	collectRaw    []string
)

func init() {
	mergeCmd.Flags().StringVarP(&mergeOutput, "output", "o", "", "Output file or directory (default: original filename from header)")
	mergeCmd.Flags().StringVarP(&mergePassword, "password", "p", "", "Decryption password (omit for interactive prompt)")
	mergeCmd.Flags().IntVarP(&mergeWorkers, "workers", "w", runtime.NumCPU(), "Max parallel operations (batch mode)")
	mergeCmd.Flags().BoolVar(&mergeFailFast, "fail-fast", false, "Stop on first error (batch mode)")
	mergeCmd.Flags().StringVar(&mergeManifest, "manifest", "", "Manifest file for shard validation and output verification")
	mergeCmd.Flags().StringSliceVar(&collectRaw, "collect", nil, "Backend URIs to collect shards from (comma-separated)")

	rootCmd.AddCommand(mergeCmd)
}

func runMerge(cmd *cobra.Command, args []string) error {
	// --collect is incompatible with --dry-run (it downloads real files)
	if len(collectRaw) > 0 && dryRun {
		return fmt.Errorf("--dry-run is not supported with --collect (collection downloads real files)")
	}

	// If --collect with --manifest, use manifest-guided collection
	if len(collectRaw) > 0 && mergeManifest != "" {
		return runCollectWithManifest()
	}

	// If --collect without manifest, use backend listing
	if len(collectRaw) > 0 {
		return runCollectFromBackends()
	}

	// Determine shard directory
	var shardDir string
	switch {
	case len(args) == 1:
		shardDir = args[0]
	case mergeManifest != "":
		// Derive shard dir from manifest's directory
		shardDir = filepath.Dir(mergeManifest)
	default:
		return fmt.Errorf("requires a shard directory argument (or use --manifest or --collect)")
	}

	if dryRun {
		if pipeline.IsBatchMergeDir(shardDir) {
			return runMergeDirDryRun(shardDir)
		}
		return runMergeSingleDryRun(shardDir)
	}

	prog, cleanup := newProgressReporter()
	defer cleanup()

	if pipeline.IsBatchMergeDir(shardDir) {
		return runMergeDir(shardDir, prog)
	}

	// If --manifest is provided, validate shards before merging
	var mf *manifest.Manifest
	if mergeManifest != "" {
		var err error
		mf, err = manifest.Load(mergeManifest)
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}
		validateShardsAgainstManifest(mf, shardDir)
	}

	opts := pipeline.MergeOptions{
		ShardDir:   shardDir,
		OutputFile: mergeOutput,
		Password:   mergePassword,
		Verbose:    verbose && !quiet,
		Progress:   prog,
		PromptPassword: func() (string, error) {
			return promptPassword("Enter decryption password: ")
		},
	}

	if err := pipeline.Merge(opts); err != nil {
		return err
	}

	// If manifest was provided, verify the output file hash
	if mf != nil {
		outputPath, err := resolveOutputPath(mf)
		if err != nil {
			return err
		}
		if err := verifyOutputAgainstManifest(mf, outputPath); err != nil {
			return err
		}
	}

	return nil
}

// runCollectWithManifest downloads shards from their manifest Location fields,
// then merges from the temp directory.
func runCollectWithManifest() error {
	mf, err := manifest.Load(mergeManifest)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "hrcx-collect-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	if err := pipeline.CollectFromManifest(context.Background(), mf, tempDir, loadedBackendConfig); err != nil {
		return fmt.Errorf("collecting shards: %w", err)
	}

	if err := mergeFromShardDir(tempDir); err != nil {
		return err
	}

	outputPath, err := resolveOutputPath(mf)
	if err != nil {
		return err
	}
	return verifyOutputAgainstManifest(mf, outputPath)
}

// runCollectFromBackends lists .hrcx files on each backend, downloads them,
// then merges from the temp directory.
func runCollectFromBackends() error {
	tempDir, err := os.MkdirTemp("", "hrcx-collect-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	if err := pipeline.CollectFromBackends(context.Background(), collectRaw, tempDir, loadedBackendConfig); err != nil {
		return fmt.Errorf("collecting shards: %w", err)
	}

	return mergeFromShardDir(tempDir)
}

// mergeFromShardDir runs a merge operation on the given shard directory
// using the current command-line options.
func mergeFromShardDir(shardDir string) error {
	prog, cleanup := newProgressReporter()
	defer cleanup()

	return pipeline.Merge(pipeline.MergeOptions{
		ShardDir:   shardDir,
		OutputFile: mergeOutput,
		Password:   mergePassword,
		Verbose:    verbose && !quiet,
		Progress:   prog,
		PromptPassword: func() (string, error) {
			return promptPassword("Enter decryption password: ")
		},
	})
}

func runMergeSingleDryRun(shardDir string) error {
	r, err := pipeline.DryRunMerge(pipeline.MergeOptions{
		ShardDir:   shardDir,
		OutputFile: mergeOutput,
	})
	if err != nil {
		return err
	}
	printMergeDryRun(r)
	return nil
}

func runMergeDirDryRun(inputDir string) error {
	results, err := pipeline.DryRunMergeDir(pipeline.MergeDirOptions{
		InputDir:  inputDir,
		OutputDir: mergeOutput,
	})
	if err != nil {
		return err
	}
	printMergeDirDryRun(results)
	return nil
}

func runMergeDir(inputDir string, prog progress.Reporter) error {
	results, err := pipeline.MergeDir(pipeline.MergeDirOptions{
		InputDir:  inputDir,
		OutputDir: mergeOutput,
		Password:  mergePassword,
		Verbose:   verbose && !quiet,
		Workers:   mergeWorkers,
		FailFast:  mergeFailFast,
		Progress:  prog,
		PromptPassword: func() (string, error) {
			return promptPassword("Enter decryption password: ")
		},
	})
	if err != nil && results == nil {
		return err
	}
	return reportBatchResults(results, "merge")
}

// validateShardsAgainstManifest checks each shard file's SHA-256 against the manifest.
func validateShardsAgainstManifest(m *manifest.Manifest, shardDir string) {
	fmt.Println("Validating shards against manifest...")
	for _, entry := range m.Shards {
		shardPath, err := safeShardPath(shardDir, entry.Filename)
		if err != nil {
			fmt.Printf("  %-9s  shard %d: %s (%v)\n", "[ERROR]", entry.Index, entry.Filename, err)
			continue
		}

		hash, _, err := pipeline.HashFile(shardPath)

		var label string
		switch {
		case err != nil && os.IsNotExist(err):
			label = "[MISSING]"
		case err != nil:
			label = "[ERROR]"
		case hash != entry.SHA256:
			label = "[CORRUPT]"
		default:
			label = "[OK]"
		}

		if err != nil && !os.IsNotExist(err) {
			fmt.Printf("  %-9s  shard %d: %s (%v)\n", label, entry.Index, entry.Filename, err)
		} else {
			fmt.Printf("  %-9s  shard %d: %s\n", label, entry.Index, entry.Filename)
		}
	}
}

// validateBaseName checks that name is a plain filename with no path components.
func validateBaseName(name string) error {
	if name == "" {
		return fmt.Errorf("empty filename")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("unsafe filename %q", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("unsafe filename %q (contains path components)", name)
	}
	return nil
}

// safeShardPath validates that a shard filename is a plain base name (no path
// separators or traversal) and returns the joined path under shardDir.
func safeShardPath(shardDir, filename string) (string, error) {
	if err := validateBaseName(filename); err != nil {
		return "", err
	}
	joined := filepath.Join(shardDir, filename)
	// Double-check the result stays under shardDir using filepath.Rel,
	// which correctly handles edge cases like root directories.
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	base, err := filepath.Abs(shardDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return "", fmt.Errorf("resolved path escapes shard directory")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resolved path escapes shard directory")
	}
	return joined, nil
}

// resolveOutputPath returns the output file path, falling back to the manifest's
// original filename when no explicit output was specified.
func resolveOutputPath(mf *manifest.Manifest) (string, error) {
	if mergeOutput != "" {
		return mergeOutput, nil
	}
	if err := validateBaseName(mf.Original.Filename); err != nil {
		return "", fmt.Errorf("unsafe filename in manifest: %w", err)
	}
	return mf.Original.Filename, nil
}

// verifyOutputAgainstManifest checks the reconstructed file's SHA-256 against the manifest.
func verifyOutputAgainstManifest(m *manifest.Manifest, outputPath string) error {
	hash, _, err := pipeline.HashFile(outputPath)
	if err != nil {
		return fmt.Errorf("verifying output: %w", err)
	}

	if hash == m.Original.SHA256 {
		fmt.Println("Verification: OK (SHA-256 matches manifest)")
		return nil
	}

	return fmt.Errorf("verification failed: output SHA-256 %s does not match manifest %s", hash, m.Original.SHA256)
}
