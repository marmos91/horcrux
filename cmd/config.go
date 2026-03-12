package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/marmos91/horcrux/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage horcrux configuration",
	Long:  "View and manage the horcrux configuration file (.hrcxrc or ~/.config/horcrux/config.yaml).",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a default config file",
	Long:  "Write a default config file to ~/.config/horcrux/config.yaml.",
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the active configuration",
	Long:  "Find and display the active config file, showing each setting and its source.",
	RunE:  runConfigShow,
}

var forceInit bool

func init() {
	configInitCmd.Flags().BoolVar(&forceInit, "force", false, "Overwrite existing config file")

	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}

	if !forceInit {
		if _, err := os.Stat(cfgPath); err == nil {
			return fmt.Errorf("config file already exists: %s (use --force to overwrite)", cfgPath)
		}
	}

	cfg := config.DefaultConfig()
	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Config file created: %s\n", cfgPath)
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfgPath := config.FindConfigFile()

	if cfgPath == "" {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), `No config file found.

Search locations:
  1. ./.hrcxrc
  2. ~/.config/horcrux/config.yaml
  3. ~/.hrcxrc

Run 'hrcx config init' to create a default config file.
`)
		return nil
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config file %s: %w", cfgPath, err)
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "Config file: %s\n\n", cfgPath)
	_, _ = fmt.Fprintln(w, "Settings:")

	printSetting(w, "data-shards", intPtrStr(cfg.DataShards), cfg.DataShards != nil, "5")
	printSetting(w, "parity-shards", intPtrStr(cfg.ParityShards), cfg.ParityShards != nil, "3")
	printSetting(w, "output", strPtrStr(cfg.Output), cfg.Output != nil, ".")
	printSetting(w, "no-encrypt", boolPtrStr(cfg.NoEncrypt), cfg.NoEncrypt != nil, "false")
	printSetting(w, "workers", intPtrStr(cfg.Workers), cfg.Workers != nil, strconv.Itoa(runtime.NumCPU()))
	printSetting(w, "fail-fast", boolPtrStr(cfg.FailFast), cfg.FailFast != nil, "false")

	return nil
}

func printSetting(w io.Writer, name, value string, fromConfig bool, defaultValue string) {
	source := "(default)"
	displayValue := defaultValue
	if fromConfig {
		source = "(from config)"
		displayValue = value
	}
	_, _ = fmt.Fprintf(w, "  %-16s %s %s\n", name+":", displayValue, source)
}

func intPtrStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func strPtrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func boolPtrStr(p *bool) string {
	if p == nil {
		return ""
	}
	return strconv.FormatBool(*p)
}
