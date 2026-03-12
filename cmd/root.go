package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/marmos91/horcrux/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	verbose bool
	dryRun  bool
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
	if cfg.DataShards != nil {
		if err := setFlagDefault(cmd.Flags(), "data-shards", strconv.Itoa(*cfg.DataShards)); err != nil {
			return err
		}
	}
	if cfg.ParityShards != nil {
		if err := setFlagDefault(cmd.Flags(), "parity-shards", strconv.Itoa(*cfg.ParityShards)); err != nil {
			return err
		}
	}
	if cfg.Output != nil {
		if err := setFlagDefault(cmd.Flags(), "output", *cfg.Output); err != nil {
			return err
		}
	}
	if cfg.NoEncrypt != nil {
		if err := setFlagDefault(cmd.Flags(), "no-encrypt", strconv.FormatBool(*cfg.NoEncrypt)); err != nil {
			return err
		}
	}
	if cfg.Workers != nil {
		if err := setFlagDefault(cmd.Flags(), "workers", strconv.Itoa(*cfg.Workers)); err != nil {
			return err
		}
	}
	if cfg.FailFast != nil {
		if err := setFlagDefault(cmd.Flags(), "fail-fast", strconv.FormatBool(*cfg.FailFast)); err != nil {
			return err
		}
	}
	return nil
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
