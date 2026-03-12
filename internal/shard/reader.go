package shard

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

var ErrPayloadChecksum = errors.New("payload checksum mismatch")

// Reader reads a shard file and provides access to header and payload.
type Reader struct {
	file   *os.File
	Header *Header
}

// OpenReader opens a shard file, reads and validates the header.
func OpenReader(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	header, err := ReadHeader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("reading shard %s: %w", path, err)
	}

	return &Reader{
		file:   f,
		Header: header,
	}, nil
}

// PayloadReader returns a reader limited to the payload bytes.
func (r *Reader) PayloadReader() io.Reader {
	return io.LimitReader(r.file, int64(r.Header.PayloadSize))
}

// SeekToPayload seeks to the start of the payload.
func (r *Reader) SeekToPayload() error {
	_, err := r.file.Seek(HeaderSize, io.SeekStart)
	return err
}

// VerifyPayload reads the entire payload and verifies the checksum in the trailer.
func (r *Reader) VerifyPayload() error {
	if err := r.SeekToPayload(); err != nil {
		return err
	}

	h := sha256.New()
	if _, err := io.CopyN(h, r.file, int64(r.Header.PayloadSize)); err != nil {
		return fmt.Errorf("reading payload for verification: %w", err)
	}

	// Read trailer
	var trailer [TrailerSize]byte
	if _, err := io.ReadFull(r.file, trailer[:]); err != nil {
		return fmt.Errorf("reading trailer: %w", err)
	}

	if !bytes.Equal(h.Sum(nil), trailer[:]) {
		return ErrPayloadChecksum
	}

	// Seek back to payload start for subsequent reads
	return r.SeekToPayload()
}

// Close closes the underlying file.
func (r *Reader) Close() error {
	return r.file.Close()
}
