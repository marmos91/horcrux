package qr

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/makiuchi-d/gozxing"
	gozxingqr "github.com/makiuchi-d/gozxing/qrcode"
)

// decodeHints configures the QR decoder for maximum reliability.
var decodeHints = map[gozxing.DecodeHintType]interface{}{
	gozxing.DecodeHintType_TRY_HARDER: true,
}

// DecodeShard decodes a QR code from a PNG or JPEG image file and returns
// the raw shard bytes. The QR code content is expected to be base64-encoded
// binary data (as produced by EncodeShard/EncodeBytes).
//
// Annotated images (with text below the QR code) are handled by cropping to
// the largest square from the top-left corner before decoding.
func DecodeShard(imagePath string) ([]byte, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("opening image: %w", err)
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	img = cropToSquare(img)

	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return nil, fmt.Errorf("creating bitmap: %w", err)
	}

	reader := gozxingqr.NewQRCodeReader()
	result, err := reader.Decode(bmp, decodeHints)
	if err != nil {
		return nil, fmt.Errorf("decoding QR code: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(result.GetText())
	if err != nil {
		return nil, fmt.Errorf("decoding base64 payload: %w", err)
	}

	return data, nil
}

// cropToSquare returns a square sub-image from the top-left corner. If the
// image is already square (or wider than tall), it is returned unchanged.
// This removes annotation areas added below QR codes by Annotate().
func cropToSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if h <= w {
		return img
	}
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+w, bounds.Min.Y+w))
	}
	return img
}
