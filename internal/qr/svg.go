package qr

import (
	"fmt"
	"html"
	"os"
	"strings"
)

// WriteSVG generates a QR code from a shard file and writes it as SVG.
// The SVG includes the QR code and text label elements.
func WriteSVG(shardPath, outputPath string, label ShardLabel) error {
	data, err := ReadShardData(shardPath)
	if err != nil {
		return err
	}

	qrc, err := newQRCode(data)
	if err != nil {
		return err
	}

	bitmap := qrc.Bitmap()
	modules := len(bitmap)
	moduleSize := 8
	margin := 4 * moduleSize // standard QR quiet zone
	qrSize := modules*moduleSize + 2*margin
	textHeight := 50
	totalHeight := qrSize + textHeight

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n",
		qrSize, totalHeight, qrSize, totalHeight)

	// White background
	fmt.Fprintf(&sb, `  <rect width="%d" height="%d" fill="white"/>`+"\n", qrSize, totalHeight)

	// QR modules
	for y, row := range bitmap {
		for x, black := range row {
			if black {
				px := margin + x*moduleSize
				py := margin + y*moduleSize
				fmt.Fprintf(&sb, `  <rect x="%d" y="%d" width="%d" height="%d" fill="black"/>`+"\n",
					px, py, moduleSize, moduleSize)
			}
		}
	}

	// Text labels
	line1 := html.EscapeString(label.Filename)
	line2 := html.EscapeString(label.Summary())

	fmt.Fprintf(&sb, `  <text x="%d" y="%d" font-family="monospace" font-size="12" fill="black">%s</text>`+"\n",
		8, qrSize+18, line1)
	fmt.Fprintf(&sb, `  <text x="%d" y="%d" font-family="monospace" font-size="12" fill="black">%s</text>`+"\n",
		8, qrSize+36, line2)

	sb.WriteString("</svg>\n")

	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing SVG: %w", err)
	}

	return nil
}
