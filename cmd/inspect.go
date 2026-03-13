package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/shard"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <shard-file-or-dir>",
	Short: "Display shard metadata without decryption",
	Long:  "Display shard metadata without decryption. Can inspect a single shard file or all shards in a directory.",
	Args:  cobra.ExactArgs(1),
	RunE:  runInspect,
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	target := args[0]

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", target, err)
	}

	if info.IsDir() {
		return inspectDir(target)
	}
	return inspectFile(target)
}

func inspectDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	found := false
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hrcx") {
			if found {
				fmt.Println()
			}
			if err := inspectFile(filepath.Join(dir, e.Name())); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", e.Name(), err)
			}
			found = true
		}
	}

	if !found {
		return fmt.Errorf("no .hrcx shard files found in %s", dir)
	}
	return nil
}

func inspectFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header, err := shard.ReadHeader(f)
	if err != nil {
		return fmt.Errorf("invalid shard: %w", err)
	}

	total := header.DataShards + header.ParityShards

	shardType := "data shard"
	if header.ShardIndex >= header.DataShards {
		shardType = "parity shard"
	}

	encrypted := "no"
	if header.IsEncrypted() {
		usesKey := header.UsesKeyFile()
		usesPass := header.UsesPassword()
		switch {
		case usesKey && usesPass:
			encrypted = "yes (password + key file)"
		case usesKey:
			encrypted = "yes (key file)"
		default:
			encrypted = "yes (password)"
		}
	}

	checksumStatus := "OK"
	if !header.ChecksumValid {
		checksumStatus = "CORRUPT"
	}

	fmt.Printf("Shard: %s\n", filepath.Base(path))
	fmt.Printf("├── Format version:    %d\n", header.Version)
	fmt.Printf("├── Shard index:       %d / %d (%s)\n", header.ShardIndex, total, shardType)
	fmt.Printf("├── Data shards:       %d\n", header.DataShards)
	fmt.Printf("├── Parity shards:     %d\n", header.ParityShards)
	fmt.Printf("├── Original filename: %s\n", header.OriginalFilename)
	fmt.Printf("├── Original filesize: %s\n", display.FormatSize(header.OriginalFileSize))
	fmt.Printf("├── Encrypted:         %s\n", encrypted)
	fmt.Printf("└── Header checksum:   %s\n", checksumStatus)

	return nil
}
