package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marmos91/horcrux/internal/qr"
	"github.com/marmos91/horcrux/internal/shard"
	"github.com/spf13/cobra"
)

var (
	importQROutput string
	importQRForce  bool
)

var importQRCmd = &cobra.Command{
	Use:   "import-qr <image-or-dir>",
	Short: "Import shards from QR code images",
	Long: `Import shards from QR code images previously exported with export-qr.
Accepts a single image file or a directory of images.

Supported input formats: PNG, JPEG. SVG files cannot be decoded and will be skipped.
Output filenames are derived from the shard header metadata, not the image filename.`,
	Args: cobra.ExactArgs(1),
	RunE: runImportQR,
}

func init() {
	importQRCmd.Flags().StringVarP(&importQROutput, "output", "o", ".", "Output directory for recovered .hrcx files")
	importQRCmd.Flags().BoolVar(&importQRForce, "force", false, "Overwrite existing output files")
	rootCmd.AddCommand(importQRCmd)
}

func runImportQR(cmd *cobra.Command, args []string) error {
	target := args[0]

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", target, err)
	}

	if err := os.MkdirAll(importQROutput, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if info.IsDir() {
		return importQRDir(target)
	}
	return importQRFile(target)
}

func importQRDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	var imported, skipped, failed int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(e.Name()))

		if ext == ".svg" {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s (SVG decoding not supported)\n", e.Name())
			skipped++
			continue
		}

		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}

		imgPath := filepath.Join(dir, e.Name())
		if err := importQRFile(imgPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", e.Name(), err)
			failed++
			continue
		}
		imported++
	}

	if imported == 0 && failed == 0 {
		return fmt.Errorf("no QR code images found in %s", dir)
	}

	fmt.Printf("\nImported %d shard(s)", imported)
	if skipped > 0 {
		fmt.Printf(", skipped %d", skipped)
	}
	if failed > 0 {
		fmt.Printf(", failed %d", failed)
	}
	fmt.Println()

	if imported == 0 {
		return fmt.Errorf("all images failed to decode")
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d image(s) failed to decode (erasure coding may still allow reconstruction)\n", failed)
	}
	return nil
}

func importQRFile(imagePath string) error {
	data, err := qr.DecodeShard(imagePath)
	if err != nil {
		return fmt.Errorf("decoding QR from %s: %w", filepath.Base(imagePath), err)
	}

	// Validate that decoded bytes are a valid shard
	header, err := shard.ReadHeader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid shard data in %s: %w", filepath.Base(imagePath), err)
	}

	// Derive output filename from shard header
	outName := fmt.Sprintf("%s.%03d.hrcx", header.OriginalFilename, header.ShardIndex)
	outPath := filepath.Join(importQROutput, outName)

	if !importQRForce {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("output file %s already exists (use --force to overwrite)", outPath)
		}
	}

	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("writing shard file: %w", err)
	}

	fmt.Printf("  %s → %s\n", filepath.Base(imagePath), outName)
	return nil
}
