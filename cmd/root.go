package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	dryRun  bool
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
}
