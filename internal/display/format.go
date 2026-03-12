package display

import "fmt"

// FormatSize returns a human-readable representation of a byte count.
func FormatSize(bytes uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// BatchResult represents the outcome of a single file in a batch operation.
type BatchResult struct {
	File  string
	Error error
}

// FormatBatchResults prints a summary table of batch operation results.
func FormatBatchResults(results []BatchResult) {
	succeeded := 0
	failed := 0

	for _, r := range results {
		if r.Error == nil {
			fmt.Printf("  OK    %s\n", r.File)
			succeeded++
		} else {
			fmt.Printf("  FAIL  %s  (%s)\n", r.File, r.Error)
			failed++
		}
	}

	total := succeeded + failed
	fmt.Printf("\n%d files processed: %d succeeded, %d failed\n", total, succeeded, failed)
}
