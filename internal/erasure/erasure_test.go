package erasure

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	data := make([]byte, 10000)
	_, _ = rand.Read(data)

	dataShards := 5
	parityShards := 3
	totalShards := dataShards + parityShards

	enc, err := NewEncoder(dataShards, parityShards)
	if err != nil {
		t.Fatal(err)
	}

	// Split into data shards
	dataBuffers := make([]*bytes.Buffer, dataShards)
	dataWriters := make([]io.Writer, dataShards)
	for i := range dataShards {
		dataBuffers[i] = &bytes.Buffer{}
		dataWriters[i] = dataBuffers[i]
	}

	if err := enc.Split(bytes.NewReader(data), dataWriters, int64(len(data))); err != nil {
		t.Fatalf("Split: %v", err)
	}

	// Encode parity
	dataReaders := make([]io.Reader, dataShards)
	for i := range dataShards {
		dataReaders[i] = bytes.NewReader(dataBuffers[i].Bytes())
	}

	parityBuffers := make([]*bytes.Buffer, parityShards)
	parityWriters := make([]io.Writer, parityShards)
	for i := range parityShards {
		parityBuffers[i] = &bytes.Buffer{}
		parityWriters[i] = parityBuffers[i]
	}

	if err := enc.Encode(dataReaders, parityWriters); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Join data shards back
	joinReaders := make([]io.Reader, dataShards)
	for i := range dataShards {
		joinReaders[i] = bytes.NewReader(dataBuffers[i].Bytes())
	}

	dec, err := NewDecoder(dataShards, parityShards)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := dec.Join(&output, joinReaders, int64(len(data))); err != nil {
		t.Fatalf("Join: %v", err)
	}

	if !bytes.Equal(data, output.Bytes()) {
		t.Fatal("round-trip failed")
	}

	// Test reconstruction with missing shards
	for missingCount := 1; missingCount <= parityShards; missingCount++ {
		t.Run(fmt.Sprintf("missing_%d", missingCount), func(t *testing.T) {
			allBuffers := make([][]byte, totalShards)
			for i := range dataShards {
				allBuffers[i] = dataBuffers[i].Bytes()
			}
			for i := range parityShards {
				allBuffers[dataShards+i] = parityBuffers[i].Bytes()
			}

			// Create readers with missing shards
			rsReaders := make([]io.Reader, totalShards)
			rsWriters := make([]io.Writer, totalShards)
			for i := range totalShards {
				if i < missingCount {
					// Missing
					rsWriters[i] = &bytes.Buffer{}
				} else {
					rsReaders[i] = bytes.NewReader(allBuffers[i])
				}
			}

			dec2, _ := NewDecoder(dataShards, parityShards)
			if err := dec2.Reconstruct(rsReaders, rsWriters); err != nil {
				t.Fatalf("Reconstruct: %v", err)
			}

			// Verify reconstructed data matches
			for i := 0; i < missingCount; i++ {
				reconstructed := rsWriters[i].(*bytes.Buffer).Bytes()
				if !bytes.Equal(allBuffers[i], reconstructed) {
					t.Fatalf("shard %d reconstruction mismatch", i)
				}
			}
		})
	}
}

func TestReconstructTooManyMissing(t *testing.T) {
	data := make([]byte, 5000)
	_, _ = rand.Read(data)

	dataShards := 3
	parityShards := 2
	totalShards := dataShards + parityShards

	enc, err := NewEncoder(dataShards, parityShards)
	if err != nil {
		t.Fatal(err)
	}

	dataBuffers := make([]*bytes.Buffer, dataShards)
	dataWriters := make([]io.Writer, dataShards)
	for i := range dataShards {
		dataBuffers[i] = &bytes.Buffer{}
		dataWriters[i] = dataBuffers[i]
	}

	if err := enc.Split(bytes.NewReader(data), dataWriters, int64(len(data))); err != nil {
		t.Fatal(err)
	}

	dataReaders := make([]io.Reader, dataShards)
	for i := range dataShards {
		dataReaders[i] = bytes.NewReader(dataBuffers[i].Bytes())
	}

	parityBuffers := make([]*bytes.Buffer, parityShards)
	parityWriters := make([]io.Writer, parityShards)
	for i := range parityShards {
		parityBuffers[i] = &bytes.Buffer{}
		parityWriters[i] = parityBuffers[i]
	}

	if err := enc.Encode(dataReaders, parityWriters); err != nil {
		t.Fatal(err)
	}

	// Try to reconstruct with K+1 missing (should fail)
	rsReaders := make([]io.Reader, totalShards)
	rsWriters := make([]io.Writer, totalShards)
	for i := range totalShards {
		if i < parityShards+1 {
			rsWriters[i] = &bytes.Buffer{}
		} else {
			var allBytes []byte
			if i < dataShards {
				allBytes = dataBuffers[i].Bytes()
			} else {
				allBytes = parityBuffers[i-dataShards].Bytes()
			}
			rsReaders[i] = bytes.NewReader(allBytes)
		}
	}

	dec, _ := NewDecoder(dataShards, parityShards)
	err = dec.Reconstruct(rsReaders, rsWriters)
	if err == nil {
		t.Fatal("expected error when too many shards missing")
	}
}

func TestSmallDataSingleBlock(t *testing.T) {
	data := []byte("tiny")

	enc, err := NewEncoder(3, 2)
	if err != nil {
		t.Fatal(err)
	}

	dataBuffers := make([]*bytes.Buffer, 3)
	dataWriters := make([]io.Writer, 3)
	for i := range 3 {
		dataBuffers[i] = &bytes.Buffer{}
		dataWriters[i] = dataBuffers[i]
	}

	if err := enc.Split(bytes.NewReader(data), dataWriters, int64(len(data))); err != nil {
		t.Fatal(err)
	}

	joinReaders := make([]io.Reader, 3)
	for i := range 3 {
		joinReaders[i] = bytes.NewReader(dataBuffers[i].Bytes())
	}

	dec, _ := NewDecoder(3, 2)
	var output bytes.Buffer
	if err := dec.Join(&output, joinReaders, int64(len(data))); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, output.Bytes()) {
		t.Fatalf("got %q, want %q", output.String(), string(data))
	}
}
