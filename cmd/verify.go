package cmd

import (
	"fmt"
	"os"

	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/pipeline"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <shard-dir>",
	Short: "Verify shard integrity and recoverability",
	Long:  "Check shard checksums and report whether the original file can be recovered. Does not require a password.",
	Args:  cobra.ExactArgs(1),
	RunE:  runVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, args []string) error {
	dir := args[0]

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	if pipeline.IsBatchMergeDir(dir) {
		return runVerifyBatch(dir)
	}
	return runVerifySingle(dir)
}

func runVerifySingle(dir string) error {
	r, err := pipeline.Verify(dir)
	if err != nil {
		if quiet {
			os.Exit(1)
		}
		return err
	}

	if !quiet {
		printVerifyResult(r)
	}

	if !r.Recoverable {
		if quiet {
			os.Exit(1)
		}
		return fmt.Errorf("file is not recoverable")
	}
	return nil
}

func runVerifyBatch(dir string) error {
	results, err := pipeline.VerifyBatch(dir)
	if err != nil {
		if quiet {
			os.Exit(1)
		}
		return err
	}

	if !quiet {
		printVerifyBatchResults(results)
	}

	for _, r := range results {
		if !r.Recoverable {
			if quiet {
				os.Exit(1)
			}
			return fmt.Errorf("one or more files are not recoverable")
		}
	}
	return nil
}

func printVerifyResult(r *pipeline.VerifyResult) {
	fmt.Printf("Verify: %s (%s)\n", r.OriginalName, display.FormatSize(r.OriginalSize))

	if verbose {
		for _, st := range r.ShardStatuses {
			printShardStatusLine(st, r.ManifestFound)
		}
		fmt.Println()
	}

	fmt.Printf("  Shards:     %d of %d found (%d required)\n", r.ShardsFound, r.TotalShards, r.DataShards)
	fmt.Printf("  Valid:      %d\n", r.ShardsValid)
	fmt.Printf("  Corrupt:    %s\n", indicesLabel(r.CorruptIndices))
	fmt.Printf("  Missing:    %s\n", indicesLabel(r.MissingIndices))
	fmt.Printf("  Manifest:   %s\n", manifestLabel(r))
	fmt.Printf("  Status:     %s\n", recoverabilityLabel(r.Recoverable))
}

func printShardStatusLine(st pipeline.ShardStatus, hasManifest bool) {
	if st.Path == "" {
		fmt.Printf("  Shard %-3d (missing)\n", st.Index)
		return
	}

	fmt.Printf("  Shard %-3d %-30s %-7s header:%-7s payload:%-7s consistency:%-7s",
		st.Index, st.Filename, st.Type,
		validityLabel(st.HeaderValid), validityLabel(st.PayloadValid),
		validityLabel(st.ConsistencyOK))

	if hasManifest {
		fmt.Printf(" manifest:%s", manifestHashLabel(st.ManifestHashOK))
	}
	fmt.Println()
}

func printVerifyBatchResults(results []pipeline.VerifyResult) {
	recoverable := 0
	for _, r := range results {
		if r.Recoverable {
			recoverable++
		}
		fmt.Printf("  %-4s  %-30s  %d/%d shards valid  %s\n",
			batchStatusLabel(r.Recoverable), displayName(r.RelPath, r.OriginalName),
			r.ShardsValid, r.TotalShards, display.FormatSize(r.OriginalSize))
	}

	fmt.Println()
	fmt.Printf("%d files: %d recoverable, %d not recoverable\n",
		len(results), recoverable, len(results)-recoverable)
}

func validityLabel(ok bool) string {
	if ok {
		return "OK"
	}
	return "CORRUPT"
}

func manifestHashLabel(ok *bool) string {
	if ok == nil {
		return "n/a"
	}
	if *ok {
		return "OK"
	}
	return "MISMATCH"
}

func recoverabilityLabel(recoverable bool) string {
	if recoverable {
		return "RECOVERABLE"
	}
	return "NOT RECOVERABLE"
}

func batchStatusLabel(recoverable bool) string {
	if recoverable {
		return "OK"
	}
	return "FAIL"
}

// manifestLabel summarizes the manifest status for a verify result.
func manifestLabel(r *pipeline.VerifyResult) string {
	if !r.ManifestFound {
		return "not found"
	}

	checked, mismatched := 0, 0
	for _, st := range r.ShardStatuses {
		if st.ManifestHashOK == nil {
			continue
		}
		checked++
		if !*st.ManifestHashOK {
			mismatched++
		}
	}

	if checked == 0 {
		return "found (no shards checked)"
	}
	if mismatched > 0 {
		return "MISMATCH"
	}
	return "OK"
}
