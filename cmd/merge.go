package cmd

import (
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge <shard-dir>",
	Short: "Reconstruct a file from shards",
	Long:  "Reconstruct a file from shards. Tolerates up to K missing or corrupt shards.",
	Args:  cobra.ExactArgs(1),
	RunE:  runMerge,
}

var (
	mergeOutput   string
	mergePassword string
)

func init() {
	mergeCmd.Flags().StringVarP(&mergeOutput, "output", "o", "", "Output file path (default: original filename from header)")
	mergeCmd.Flags().StringVarP(&mergePassword, "password", "p", "", "Decryption password (omit for interactive prompt)")

	rootCmd.AddCommand(mergeCmd)
}

func runMerge(cmd *cobra.Command, args []string) error {
	opts := pipeline.MergeOptions{
		ShardDir:   args[0],
		OutputFile: mergeOutput,
		Password:   mergePassword,
		Verbose:    verbose,
		PromptPassword: func() (string, error) {
			return promptPassword("Enter decryption password: ")
		},
	}

	return pipeline.Merge(opts)
}
