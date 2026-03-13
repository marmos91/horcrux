package cmd

import (
	"fmt"

	"github.com/marmos91/horcrux/internal/crypto"
	"github.com/spf13/cobra"
)

var keygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "Generate a random key file for encryption",
	Long: `Generate a cryptographically random key file for use with --key-file.

The key file can be used alone (key-file-only encryption) or combined
with a password for two-factor encryption.`,
	Args: cobra.NoArgs,
	RunE: runKeygen,
}

var (
	keygenOutput string
	keygenSize   int
)

func init() {
	keygenCmd.Flags().StringVarP(&keygenOutput, "output", "o", "horcrux.key", "Output key file path")
	keygenCmd.Flags().IntVarP(&keygenSize, "size", "s", crypto.KeyFileSize, "Key file size in bytes (1 to 1048576)")

	rootCmd.AddCommand(keygenCmd)
}

func runKeygen(cmd *cobra.Command, args []string) error {
	if keygenSize < 1 || keygenSize > 1<<20 {
		return fmt.Errorf("size must be between 1 and 1048576 bytes")
	}

	if err := crypto.GenerateKeyFile(keygenOutput, keygenSize); err != nil {
		return err
	}

	fmt.Printf("Key file generated: %s (%d bytes)\n", keygenOutput, keygenSize)
	return nil
}
