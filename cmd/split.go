package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var splitCmd = &cobra.Command{
	Use:   "split <input-file-or-dir>",
	Short: "Split a file (or all files in a directory) into encrypted, erasure-coded shards",
	Long: `Split a file into N data + K parity encrypted shards using Reed-Solomon erasure coding.

If the input is a directory, all files are recursively split in parallel.
The output mirrors the input directory structure, with each file's shards
placed in a subdirectory named after the file.`,
	Args: cobra.ExactArgs(1),
	RunE: runSplit,
}

var (
	dataShards   int
	parityShards int
	outputDir    string
	password     string
	noEncrypt    bool
	workers      int
	failFast     bool
)

func init() {
	splitCmd.Flags().IntVarP(&dataShards, "data-shards", "n", 5, "Number of data shards")
	splitCmd.Flags().IntVarP(&parityShards, "parity-shards", "k", 3, "Number of parity shards")
	splitCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "Output directory")
	splitCmd.Flags().StringVarP(&password, "password", "p", "", "Encryption password (omit for interactive prompt)")
	splitCmd.Flags().BoolVar(&noEncrypt, "no-encrypt", false, "Skip encryption")
	splitCmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "Max parallel operations (directory mode)")
	splitCmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on first error (directory mode)")

	rootCmd.AddCommand(splitCmd)
}

func runSplit(cmd *cobra.Command, args []string) error {
	input := args[0]

	info, err := os.Stat(input)
	if os.IsNotExist(err) {
		return fmt.Errorf("input not found: %s", input)
	}
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", input, err)
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

	// Resolve password once before any work
	var pwd string
	if !noEncrypt {
		if password != "" {
			pwd = password
		} else {
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

	if info.IsDir() {
		return runSplitDir(input, pwd)
	}

	return pipeline.Split(pipeline.SplitOptions{
		InputFile:    input,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		Password:     pwd,
		NoEncrypt:    noEncrypt,
		Verbose:      verbose,
	})
}

func runSplitDir(inputDir, pwd string) error {
	results, err := pipeline.SplitDir(pipeline.SplitDirOptions{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		Password:     pwd,
		NoEncrypt:    noEncrypt,
		Verbose:      verbose,
		Workers:      workers,
		FailFast:     failFast,
	})
	if err != nil && results == nil {
		return err
	}

	batchResults := make([]display.BatchResult, len(results))
	for i, r := range results {
		batchResults[i] = display.BatchResult{File: r.File, Error: r.Error}
	}
	display.FormatBatchResults(batchResults)

	// Return an error if any file failed
	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("some files failed to split")
		}
	}
	return nil
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
