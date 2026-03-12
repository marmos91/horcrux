package cmd

import (
	"fmt"
	"os"

	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/marmos91/horcrux/internal/progress"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	verbose bool
	dryRun  bool
	quiet   bool
)

var rootCmd = &cobra.Command{
	Use:   "hrcx",
	Short: "Split files into encrypted, erasure-coded shards",
	Long:  "Horcrux splits files into encrypted, erasure-coded shards and reconstructs them from a subset of those shards.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output with progress")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview what would happen without writing any files")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all progress output")
}

// shouldShowProgress returns true if progress bars should be displayed.
// Progress is suppressed when quiet mode is on, during dry-run, or when
// stderr is not a terminal.
func shouldShowProgress() bool {
	if quiet || dryRun {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// newProgressReporter creates a progress reporter when progress display is
// enabled, or returns nil otherwise. Pipeline functions handle nil via
// progress.OrNop. The caller must defer the returned cleanup function.
func newProgressReporter() (progress.Reporter, func()) {
	if shouldShowProgress() {
		r := progress.NewBarReporter(os.Stderr)
		return r, func() { r.Close() }
	}
	return nil, func() {}
}

// reportBatchResults prints a summary of batch file results and returns an
// error if any individual operation failed.
func reportBatchResults(results []pipeline.FileResult, operation string) error {
	batchResults := make([]display.BatchResult, len(results))
	var hasFailure bool
	for i, r := range results {
		batchResults[i] = display.BatchResult{File: r.File, Error: r.Error}
		if r.Error != nil {
			hasFailure = true
		}
	}
	display.FormatBatchResults(batchResults)

	if hasFailure {
		return fmt.Errorf("some files failed to %s", operation)
	}
	return nil
}
