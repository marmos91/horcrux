package erasure

import (
	"fmt"
	"io"

	"github.com/klauspost/reedsolomon"
)

// Encoder wraps the Reed-Solomon streaming encoder.
type Encoder struct {
	enc       reedsolomon.StreamEncoder
	dataCount int
	parCount  int
}

// NewEncoder creates a new streaming RS encoder.
func NewEncoder(dataShards, parityShards int) (*Encoder, error) {
	enc, err := reedsolomon.NewStream(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("creating RS encoder: %w", err)
	}
	return &Encoder{
		enc:       enc,
		dataCount: dataShards,
		parCount:  parityShards,
	}, nil
}

// Split distributes data from the input reader across N data shard writers.
// The input size must be known in advance so the encoder can pad correctly.
func (e *Encoder) Split(input io.Reader, dataWriters []io.Writer, inputSize int64) error {
	if len(dataWriters) != e.dataCount {
		return fmt.Errorf("expected %d data writers, got %d", e.dataCount, len(dataWriters))
	}
	return e.enc.Split(input, dataWriters, inputSize)
}

// Encode reads from data shard readers and writes parity to parity writers.
func (e *Encoder) Encode(dataReaders []io.Reader, parityWriters []io.Writer) error {
	if len(dataReaders) != e.dataCount {
		return fmt.Errorf("expected %d data readers, got %d", e.dataCount, len(dataReaders))
	}
	if len(parityWriters) != e.parCount {
		return fmt.Errorf("expected %d parity writers, got %d", e.parCount, len(parityWriters))
	}
	return e.enc.Encode(dataReaders, parityWriters)
}
