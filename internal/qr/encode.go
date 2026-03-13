package qr

import (
	"encoding/base64"
	"fmt"
	"image"
	"os"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	// MaxQRBinaryBytes is the maximum number of raw bytes that can be encoded
	// in a QR code after base64 overhead. QR v40 ECC-L holds 2953 alphanumeric
	// chars; base64 expands data by 4/3, so max raw bytes = 2953 * 3 / 4 = 2214.
	MaxQRBinaryBytes = 2214

	// DefaultQRSize is the default pixel dimension for generated QR code PNGs.
	// 2048px ensures reliable decoding even for large QR codes (v40).
	DefaultQRSize = 2048
)

// ShardTooLargeError indicates a shard file exceeds QR code capacity.
type ShardTooLargeError struct {
	Path string
	Size int64
	Max  int
}

func (e *ShardTooLargeError) Error() string {
	return fmt.Sprintf("shard %s is %d bytes, exceeds QR capacity of %d bytes", e.Path, e.Size, e.Max)
}

// CheckShardFits checks whether a shard file fits within QR code capacity
// using only a stat call (no file read).
func CheckShardFits(shardPath string) error {
	info, err := os.Stat(shardPath)
	if err != nil {
		return fmt.Errorf("cannot stat shard: %w", err)
	}
	if info.Size() > int64(MaxQRBinaryBytes) {
		return &ShardTooLargeError{Path: shardPath, Size: info.Size(), Max: MaxQRBinaryBytes}
	}
	return nil
}

// ReadShardData reads a shard file and validates it fits within QR capacity.
func ReadShardData(shardPath string) ([]byte, error) {
	data, err := os.ReadFile(shardPath)
	if err != nil {
		return nil, fmt.Errorf("reading shard: %w", err)
	}
	if len(data) > MaxQRBinaryBytes {
		return nil, &ShardTooLargeError{Path: shardPath, Size: int64(len(data)), Max: MaxQRBinaryBytes}
	}
	return data, nil
}

// newQRCode creates a QR code from raw bytes via base64 encoding.
// ECC level L maximizes data capacity; shards already contain SHA-256
// checksums for integrity verification.
func newQRCode(data []byte) (*qrcode.QRCode, error) {
	if len(data) > MaxQRBinaryBytes {
		return nil, &ShardTooLargeError{Size: int64(len(data)), Max: MaxQRBinaryBytes}
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	qrc, err := qrcode.New(encoded, qrcode.Low)
	if err != nil {
		return nil, fmt.Errorf("generating QR code: %w", err)
	}
	return qrc, nil
}

// EncodeShard reads an entire .hrcx shard file and encodes it as a QR code image.
func EncodeShard(shardPath string, size int) (image.Image, error) {
	data, err := ReadShardData(shardPath)
	if err != nil {
		return nil, err
	}
	return EncodeBytes(data, size)
}

// EncodeBytes encodes raw bytes as a QR code image.
func EncodeBytes(data []byte, size int) (image.Image, error) {
	qrc, err := newQRCode(data)
	if err != nil {
		return nil, err
	}
	return qrc.Image(size), nil
}
