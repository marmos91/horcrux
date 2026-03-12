package cmd

import (
	"fmt"
	"os"

	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var splitCmd = &cobra.Command{
	Use:   "split <input-file>",
	Short: "Split a file into encrypted, erasure-coded shards",
	Long:  "Split a file into N data + K parity encrypted shards using Reed-Solomon erasure coding.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSplit,
}

var (
	dataShards   int
	parityShards int
	outputDir    string
	password     string
	noEncrypt    bool
)

func init() {
	splitCmd.Flags().IntVarP(&dataShards, "data-shards", "n", 5, "Number of data shards")
	splitCmd.Flags().IntVarP(&parityShards, "parity-shards", "k", 3, "Number of parity shards")
	splitCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "Output directory")
	splitCmd.Flags().StringVarP(&password, "password", "p", "", "Encryption password (omit for interactive prompt)")
	splitCmd.Flags().BoolVar(&noEncrypt, "no-encrypt", false, "Skip encryption")

	rootCmd.AddCommand(splitCmd)
}

func runSplit(cmd *cobra.Command, args []string) error {
	inputFile := args[0]

	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputFile)
	}

	if dataShards < 1 {
		return fmt.Errorf("data shards must be >= 1")
	}
	if parityShards < 1 {
		return fmt.Errorf("parity shards must be >= 1")
	}
	if dataShards+parityShards > 255 {
		return fmt.Errorf("total shards (data + parity) must be <= 255")
	}

	var pwd string
	if !noEncrypt {
		if password != "" {
			pwd = password
		} else {
			var err error
			pwd, err = promptPassword("Enter encryption password: ")
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			confirm, err := promptPassword("Confirm password: ")
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			if pwd != confirm {
				return fmt.Errorf("passwords do not match")
			}
		}
	}

	opts := pipeline.SplitOptions{
		InputFile:    inputFile,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		Password:     pwd,
		NoEncrypt:    noEncrypt,
		Verbose:      verbose,
	}

	return pipeline.Split(opts)
}

func promptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(pw), err
	}
	// Non-terminal: read line from stdin
	var pw string
	_, err := fmt.Scanln(&pw)
	return pw, err
}
