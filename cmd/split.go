package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"

	_ "github.com/marmos91/horcrux/internal/backend/all"
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/marmos91/horcrux/internal/progress"
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
	dataShards    int
	parityShards  int
	outputDir     string
	password      string
	noEncrypt     bool
	workers       int
	failFast      bool
	noManifest    bool
	distributeRaw []string
	keepLocal     bool
)

func init() {
	splitCmd.Flags().IntVarP(&dataShards, "data-shards", "n", 5, "Number of data shards")
	splitCmd.Flags().IntVarP(&parityShards, "parity-shards", "k", 3, "Number of parity shards")
	splitCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "Output directory")
	splitCmd.Flags().StringVarP(&password, "password", "p", "", "Encryption password (omit for interactive prompt)")
	splitCmd.Flags().BoolVar(&noEncrypt, "no-encrypt", false, "Skip encryption")
	splitCmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "Max parallel operations (directory mode)")
	splitCmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on first error (directory mode)")
	splitCmd.Flags().BoolVar(&noManifest, "no-manifest", false, "Don't generate a manifest file")
	splitCmd.Flags().StringSliceVar(&distributeRaw, "distribute", nil, "Backend URIs for shard distribution (comma-separated)")
	splitCmd.Flags().BoolVar(&keepLocal, "keep-local", true, "Keep local copies after distribution")

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

	if dryRun {
		if info.IsDir() {
			return runSplitDirDryRun(input)
		}
		return runSplitDryRun(input)
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

	prog, cleanup := newProgressReporter()
	defer cleanup()

	if info.IsDir() {
		return runSplitDir(input, pwd, prog)
	}

	result, err := pipeline.Split(pipeline.SplitOptions{
		InputFile:    input,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		Password:     pwd,
		NoEncrypt:    noEncrypt,
		NoManifest:   noManifest,
		Verbose:      verbose && !quiet,
		Progress:     prog,
	})
	if err != nil {
		return err
	}

	// Distribute shards to backends if requested
	if len(distributeRaw) > 0 && result.ShardFiles != nil {
		backends, err := pipeline.OpenBackends(distributeRaw, loadedBackendConfig)
		if err != nil {
			return err
		}

		result.ShardFiles, err = pipeline.DistributeShards(context.Background(), result.ShardFiles, backends)
		if err != nil {
			return err
		}

		if !keepLocal {
			if err := pipeline.CleanupLocalShards(result.ShardFiles); err != nil {
				return err
			}
		}
	}

	if noManifest {
		return nil
	}
	return pipeline.SaveManifest(result, outputDir)
}

func runSplitDir(inputDir, pwd string, prog progress.Reporter) error {
	results, err := pipeline.SplitDir(pipeline.SplitDirOptions{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		Password:     pwd,
		NoEncrypt:    noEncrypt,
		Verbose:      verbose && !quiet,
		Workers:      workers,
		FailFast:     failFast,
		Progress:     prog,
		NoManifest:   noManifest,
	})
	if err != nil && results == nil {
		return err
	}
	return reportBatchResults(results, "split")
}

func runSplitDryRun(input string) error {
	r, err := pipeline.DryRunSplit(pipeline.SplitOptions{
		InputFile:    input,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		NoEncrypt:    noEncrypt,
	})
	if err != nil {
		return err
	}
	printSplitDryRun(r)
	return nil
}

func runSplitDirDryRun(inputDir string) error {
	results, err := pipeline.DryRunSplitDir(pipeline.SplitDirOptions{
		InputDir:     inputDir,
		OutputDir:    outputDir,
		DataShards:   dataShards,
		ParityShards: parityShards,
		NoEncrypt:    noEncrypt,
	})
	if err != nil {
		return err
	}
	printSplitDirDryRun(results)
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
