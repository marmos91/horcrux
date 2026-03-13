package cmd

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/marmos91/horcrux/internal/qr"
	"github.com/marmos91/horcrux/internal/shard"
	"github.com/spf13/cobra"
)

var (
	exportQROutput string
	exportQRFormat string
	exportQRForce  bool
)

var exportQRCmd = &cobra.Command{
	Use:   "export-qr <shard-dir>",
	Short: "Export shards as QR codes for paper backup",
	Long: `Export shards as QR codes that can be printed and stored in physically
separate locations. QR binary mode supports up to 2953 bytes per code, so
this works best with high data-shard counts to keep individual shards small.

Supported output formats: png (default), svg.`,
	Args: cobra.ExactArgs(1),
	RunE: runExportQR,
}

func init() {
	exportQRCmd.Flags().StringVarP(&exportQROutput, "output", "o", "", "Output directory (default: <shard-dir>/qrcodes)")
	exportQRCmd.Flags().StringVarP(&exportQRFormat, "format", "f", "png", "Output format: png or svg")
	exportQRCmd.Flags().BoolVar(&exportQRForce, "force", false, "Overwrite existing output files")
	rootCmd.AddCommand(exportQRCmd)
}

func runExportQR(cmd *cobra.Command, args []string) error {
	shardDir := args[0]

	info, err := os.Stat(shardDir)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", shardDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", shardDir)
	}

	format := strings.ToLower(exportQRFormat)
	if format != "png" && format != "svg" {
		return fmt.Errorf("unsupported format %q: use png or svg", format)
	}

	// Discover .hrcx files
	entries, err := os.ReadDir(shardDir)
	if err != nil {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	var shardPaths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hrcx") {
			shardPaths = append(shardPaths, filepath.Join(shardDir, e.Name()))
		}
	}

	if len(shardPaths) == 0 {
		return fmt.Errorf("no .hrcx shard files found in %s", shardDir)
	}

	// Pre-check ALL shards fit in QR capacity
	var oversized []string
	for _, p := range shardPaths {
		if err := qr.CheckShardFits(p); err != nil {
			if stErr, ok := err.(*qr.ShardTooLargeError); ok {
				oversized = append(oversized, stErr.Error())
			} else {
				return err
			}
		}
	}
	if len(oversized) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d shard(s) exceed QR code capacity (%d bytes):\n", len(oversized), qr.MaxQRBinaryBytes)
		for _, o := range oversized {
			fmt.Fprintf(&sb, "  %s\n", o)
		}
		sb.WriteString("\nHint: use more data shards (-n) to reduce individual shard size")
		return fmt.Errorf("%s", sb.String())
	}

	// Determine output directory
	outDir := exportQROutput
	if outDir == "" {
		outDir = filepath.Join(shardDir, "qrcodes")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Generate QR codes
	for _, shardPath := range shardPaths {
		// Read shard header for metadata
		f, err := os.Open(shardPath)
		if err != nil {
			return fmt.Errorf("opening shard %s: %w", filepath.Base(shardPath), err)
		}
		header, err := shard.ReadHeader(f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("reading header from %s: %w", filepath.Base(shardPath), err)
		}

		total := int(header.DataShards) + int(header.ParityShards)
		shardType := "data"
		if header.ShardIndex >= header.DataShards {
			shardType = "parity"
		}

		label := qr.ShardLabel{
			Filename:   filepath.Base(shardPath),
			ShardIndex: int(header.ShardIndex),
			ShardTotal: total,
			ShardType:  shardType,
		}

		// Output filename: strip .hrcx, add format extension
		baseName := strings.TrimSuffix(filepath.Base(shardPath), ".hrcx")
		outPath := filepath.Join(outDir, baseName+"."+format)

		if !exportQRForce {
			if _, err := os.Stat(outPath); err == nil {
				return fmt.Errorf("output file %s already exists (use --force to overwrite)", outPath)
			}
		}

		if format == "svg" {
			if err := qr.WriteSVG(shardPath, outPath, label); err != nil {
				return fmt.Errorf("generating SVG for %s: %w", filepath.Base(shardPath), err)
			}
		} else {
			if err := exportShardAsPNG(shardPath, outPath, label); err != nil {
				return err
			}
		}

		fmt.Printf("  %s → %s\n", filepath.Base(shardPath), filepath.Base(outPath))
	}

	fmt.Printf("\nExported %d QR codes to %s\n", len(shardPaths), outDir)
	return nil
}

func exportShardAsPNG(shardPath, outPath string, label qr.ShardLabel) error {
	img, err := qr.EncodeShard(shardPath, qr.DefaultQRSize)
	if err != nil {
		return fmt.Errorf("encoding QR for %s: %w", filepath.Base(shardPath), err)
	}

	annotated := qr.Annotate(img, label)

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	if err := png.Encode(outFile, annotated); err != nil {
		return fmt.Errorf("writing PNG: %w", err)
	}
	return nil
}
