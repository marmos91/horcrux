package erasure

import (
	"fmt"
	"io"

	"github.com/klauspost/reedsolomon"
)

// Decoder wraps the Reed-Solomon streaming decoder.
type Decoder struct {
	enc       reedsolomon.StreamEncoder
	dataCount int
	parCount  int
}

// NewDecoder creates a new streaming RS decoder.
func NewDecoder(dataShards, parityShards int) (*Decoder, error) {
	enc, err := reedsolomon.NewStream(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("creating RS decoder: %w", err)
	}
	return &Decoder{
		enc:       enc,
		dataCount: dataShards,
		parCount:  parityShards,
	}, nil
}

// Reconstruct rebuilds missing shards from available ones.
// Readers/writers slices must be len(data+parity). Set nil for missing shards
// in readers, and provide writers for the shards you want reconstructed.
func (d *Decoder) Reconstruct(readers []io.Reader, writers []io.Writer) error {
	total := d.dataCount + d.parCount
	if len(readers) != total {
		return fmt.Errorf("expected %d readers, got %d", total, len(readers))
	}
	if len(writers) != total {
		return fmt.Errorf("expected %d writers, got %d", total, len(writers))
	}
	return d.enc.Reconstruct(readers, writers)
}

// Join joins the data shards and writes the original data to the output writer.
// outSize is the original file size (to strip RS padding).
func (d *Decoder) Join(output io.Writer, dataReaders []io.Reader, outSize int64) error {
	if len(dataReaders) != d.dataCount {
		return fmt.Errorf("expected %d data readers, got %d", d.dataCount, len(dataReaders))
	}
	return d.enc.Join(output, dataReaders, outSize)
}
