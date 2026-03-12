package cmd

import (
	"runtime"

	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/marmos91/horcrux/internal/progress"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge <shard-dir>",
	Short: "Reconstruct a file from shards",
	Long: `Reconstruct a file from shards. Tolerates up to K missing or corrupt shards.

If the shard directory contains subdirectories with .hrcx files (batch layout
produced by splitting a directory), all shard sets are merged in parallel.`,
	Args: cobra.ExactArgs(1),
	RunE: runMerge,
}

var (
	mergeOutput   string
	mergePassword string
	mergeWorkers  int
	mergeFailFast bool
)

func init() {
	mergeCmd.Flags().StringVarP(&mergeOutput, "output", "o", "", "Output file or directory (default: original filename from header)")
	mergeCmd.Flags().StringVarP(&mergePassword, "password", "p", "", "Decryption password (omit for interactive prompt)")
	mergeCmd.Flags().IntVarP(&mergeWorkers, "workers", "w", runtime.NumCPU(), "Max parallel operations (batch mode)")
	mergeCmd.Flags().BoolVar(&mergeFailFast, "fail-fast", false, "Stop on first error (batch mode)")

	rootCmd.AddCommand(mergeCmd)
}

func runMerge(cmd *cobra.Command, args []string) error {
	shardDir := args[0]

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

	return pipeline.Merge(opts)
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
