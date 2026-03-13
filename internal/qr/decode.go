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
