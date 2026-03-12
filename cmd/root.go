package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/marmos91/horcrux/internal/config"
	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/marmos91/horcrux/internal/progress"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

var (
	verbose bool
	dryRun  bool
	quiet   bool
)

var rootCmd = &cobra.Command{
	Use:               "hrcx",
	Short:             "Split files into encrypted, erasure-coded shards",
	Long:              "Horcrux splits files into encrypted, erasure-coded shards and reconstructs them from a subset of those shards.",
	PersistentPreRunE: loadConfig,
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

func loadConfig(cmd *cobra.Command, args []string) error {
	// Skip config loading for "config init" to avoid chicken-and-egg
	if cmd.Name() == "init" && cmd.Parent() != nil && cmd.Parent().Name() == "config" {
		return nil
	}

	cfgPath := config.FindConfigFile()
	if cfgPath == "" {
		return nil
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config file %s: %w", cfgPath, err)
	}

	if err := applyConfigToFlags(cmd, cfg); err != nil {
		return fmt.Errorf("config file %s: %w", cfgPath, err)
	}
	return nil
}

// applyConfigToFlags sets flag defaults from config for any flag not explicitly
// set on the command line. CLI flags always take precedence over config values.
func applyConfigToFlags(cmd *cobra.Command, cfg *config.Config) error {
	flags := cmd.Flags()

	overrides := []struct {
		name  string
		value *string
	}{
		{"data-shards", ptrItoa(cfg.DataShards)},
		{"parity-shards", ptrItoa(cfg.ParityShards)},
		{"output", cfg.Output},
		{"no-encrypt", ptrFormatBool(cfg.NoEncrypt)},
		{"workers", ptrItoa(cfg.Workers)},
		{"fail-fast", ptrFormatBool(cfg.FailFast)},
		{"no-manifest", ptrFormatBool(cfg.NoManifest)},
	}

	for _, o := range overrides {
		if o.value != nil {
			if err := setFlagDefault(flags, o.name, *o.value); err != nil {
				return err
			}
		}
	}
	return nil
}

// ptrItoa converts a *int to a *string via strconv.Itoa, returning nil for nil input.
func ptrItoa(p *int) *string {
	if p == nil {
		return nil
	}
	s := strconv.Itoa(*p)
	return &s
}

// ptrFormatBool converts a *bool to a *string via strconv.FormatBool, returning nil for nil input.
func ptrFormatBool(p *bool) *string {
	if p == nil {
		return nil
	}
	s := strconv.FormatBool(*p)
	return &s
}

// setFlagDefault sets a flag's value only if the flag exists on this command
// and was not explicitly set on the command line.
func setFlagDefault(flags *pflag.FlagSet, name, value string) error {
	f := flags.Lookup(name)
	if f == nil || flags.Changed(name) {
		return nil
	}
	if err := f.Value.Set(value); err != nil {
		return fmt.Errorf("invalid value %q for --%s: %w", value, name, err)
	}
	return nil
}
