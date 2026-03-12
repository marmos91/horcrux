package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/pipeline"
)

func printSplitDryRun(r *pipeline.SplitDryRunResult) {
	fmt.Println("Dry run: split")
	fmt.Printf("  Input:        %s (%s)\n", r.OriginalName, display.FormatSize(r.OriginalSize))
	fmt.Printf("  Encryption:   %s\n", enabledLabel(r.Encrypted))
	fmt.Printf("  Shards:       %d data + %d parity = %d total\n", r.DataShards, r.ParityShards, r.TotalShards)
	fmt.Printf("  Per shard:    %s payload (%s on disk)\n", display.FormatSize(r.PerShardPayload), display.FormatSize(r.PerShardFileSize))
	fmt.Printf("  Total output: %s\n", display.FormatSize(r.TotalOutputSize))
	fmt.Printf("  Manifest:     %s\n", enabledLabel(!noManifest))
	fmt.Printf("  Output dir:   %s\n", r.OutputDir)
	fmt.Println("  Shard files:")
	for _, p := range r.ShardPaths {
		fmt.Printf("    %s\n", filepath.Base(p))
	}
}

func printMergeDryRun(r *pipeline.MergeDryRunResult) {
	status := "RECOVERABLE"
	if !r.Recoverable {
		status = "NOT RECOVERABLE"
	}

	reconstruction := "not needed"
	if r.NeedsReconstruction {
		reconstruction = "required (missing data shards)"
	}

	fmt.Println("Dry run: merge")
	fmt.Printf("  Original file:    %s (%s)\n", r.OriginalName, display.FormatSize(r.OriginalSize))
	fmt.Printf("  Encryption:       %s\n", enabledLabel(r.Encrypted))
	fmt.Printf("  Shards:           %d of %d found (%d required)\n", r.ShardsFound, r.TotalShards, r.DataShards)
	fmt.Printf("  Valid shards:     %d of %d (checksums OK)\n", r.ShardsValid, r.ShardsFound)
	fmt.Printf("  Missing:          %s\n", indicesLabel(r.MissingIndices))
	fmt.Printf("  Corrupt:          %s\n", indicesLabel(r.CorruptIndices))
	fmt.Printf("  Status:           %s\n", status)
	fmt.Printf("  Reconstruction:   %s\n", reconstruction)
	fmt.Printf("  Output file:      %s\n", r.OutputFile)
}

func printSplitDirDryRun(results []pipeline.SplitDryRunResult) {
	fmt.Println("Dry run: split directory")
	fmt.Println()

	var totalInput, totalOutput uint64
	for _, r := range results {
		fmt.Printf("  %-30s %8s -> %d shards (%s total)\n",
			displayName(r.RelPath, r.OriginalName), display.FormatSize(r.OriginalSize), r.TotalShards, display.FormatSize(r.TotalOutputSize))
		totalInput += r.OriginalSize
		totalOutput += r.TotalOutputSize
	}

	fmt.Println()
	fmt.Printf("  %d files, %s input -> %s output\n", len(results), display.FormatSize(totalInput), display.FormatSize(totalOutput))
}

func printMergeDirDryRun(results []pipeline.MergeDryRunResult) {
	fmt.Println("Dry run: merge directory")
	fmt.Println()

	recoverable := 0
	for _, r := range results {
		status := "FAIL"
		if r.Recoverable {
			status = "OK"
			recoverable++
		}

		fmt.Printf("  %-4s  %-30s  %d/%d shards valid  %s\n",
			status, displayName(r.RelPath, r.OriginalName), r.ShardsValid, r.TotalShards, display.FormatSize(r.OriginalSize))
	}

	fmt.Println()
	fmt.Printf("  %d files: %d recoverable, %d not recoverable\n", len(results), recoverable, len(results)-recoverable)
}

// displayName returns the relative path if set, otherwise the original name.
func displayName(relPath, originalName string) string {
	if relPath != "" {
		return relPath
	}
	return originalName
}

// enabledLabel returns "enabled" or "disabled" for a boolean flag.
func enabledLabel(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

// indicesLabel formats an int slice as a string, returning "none" for empty slices.
func indicesLabel(indices []int) string {
	if len(indices) == 0 {
		return "none"
	}
	return fmt.Sprintf("%v", indices)
}
