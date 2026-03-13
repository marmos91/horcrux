package cmd

import (
	"fmt"
	"strconv"

	"github.com/marmos91/horcrux/internal/manifest"
	"github.com/spf13/cobra"
)

var manifestCmd = &cobra.Command{
	Use:   "manifest",
	Short: "Manage manifest files",
	Long:  "Commands for inspecting and updating shard manifest files.",
}

var manifestAnnotateCmd = &cobra.Command{
	Use:   "annotate <manifest.json> <shard-index> <location>",
	Short: "Set the location field for a shard in the manifest",
	Long: `Annotate a shard entry in the manifest with a free-text location description.

This helps track where shards are physically stored (e.g. "USB drive A",
"S3 bucket backup-shards", "office safe").`,
	Args: cobra.ExactArgs(3),
	RunE: runManifestAnnotate,
}

func init() {
	manifestCmd.AddCommand(manifestAnnotateCmd)
	rootCmd.AddCommand(manifestCmd)
}

func runManifestAnnotate(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	indexStr := args[1]
	location := args[2]

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return fmt.Errorf("invalid shard index %q: %w", indexStr, err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	shard := m.FindShardByIndex(index)
	if shard == nil {
		return fmt.Errorf("shard index %d not found (manifest has %d shards)", index, len(m.Shards))
	}

	shard.Location = location

	if err := m.Save(manifestPath); err != nil {
		return err
	}

	fmt.Printf("Updated shard %d location to %q\n", index, location)
	return nil
}
