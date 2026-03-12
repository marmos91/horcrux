package cmd

import (
	"fmt"
	"path/filepath"
	"runtime"

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
)

func init() {
	mergeCmd.Flags().StringVarP(&mergeOutput, "output", "o", "", "Output file or directory (default: original filename from header)")
	mergeCmd.Flags().StringVarP(&mergePassword, "password", "p", "", "Decryption password (omit for interactive prompt)")
	mergeCmd.Flags().IntVarP(&mergeWorkers, "workers", "w", runtime.NumCPU(), "Max parallel operations (batch mode)")
	mergeCmd.Flags().BoolVar(&mergeFailFast, "fail-fast", false, "Stop on first error (batch mode)")
	mergeCmd.Flags().StringVar(&mergeManifest, "manifest", "", "Manifest file for shard validation and output verification")

	rootCmd.AddCommand(mergeCmd)
}

func runMerge(cmd *cobra.Command, args []string) error {
	// Determine shard directory
	var shardDir string
	switch {
	case len(args) == 1:
		shardDir = args[0]
	case mergeManifest != "":
		// Derive shard dir from manifest's directory
		shardDir = filepath.Dir(mergeManifest)
	default:
		return fmt.Errorf("requires a shard directory argument (or use --manifest)")
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
		outputPath := mergeOutput
		if outputPath == "" {
			outputPath = mf.Original.Filename
		}
		if err := verifyOutputAgainstManifest(mf, outputPath); err != nil {
			return err
		}
	}

	return nil
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
		shardPath := filepath.Join(shardDir, entry.Filename)
		hash, _, err := pipeline.HashFile(shardPath)

		var label string
		switch {
		case err != nil:
			label = "[MISSING]"
		case hash != entry.SHA256:
			label = "[CORRUPT]"
		default:
			label = "[OK]"
		}

		fmt.Printf("  %-9s  shard %d: %s\n", label, entry.Index, entry.Filename)
	}
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
