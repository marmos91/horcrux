package shard

import (
	"crypto/sha256"
	"hash"
	"io"
	"os"
)

// Writer writes a shard file: header + payload + trailer (payload SHA-256).
type Writer struct {
	file        *os.File
	header      *Header
	payloadHash hash.Hash
	written     int64
}

// CreateWriter creates a shard file and writes the header.
func CreateWriter(path string, header *Header) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	headerBytes, err := header.Marshal()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if _, err := f.Write(headerBytes[:]); err != nil {
		_ = f.Close()
		return nil, err
	}

	return &Writer{
		file:        f,
		header:      header,
		payloadHash: sha256.New(),
	}, nil
}

// Write writes payload data.
func (w *Writer) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if n > 0 {
		w.payloadHash.Write(p[:n])
		w.written += int64(n)
	}
	return n, err
}

// File returns the underlying os.File for seeking.
func (w *Writer) File() *os.File {
	return w.file
}

// Close writes the trailer (payload checksum) and closes the file.
func (w *Writer) Close() error {
	checksum := w.payloadHash.Sum(nil)
	if _, err := w.file.Write(checksum); err != nil {
		_ = w.file.Close()
		return err
	}
	return w.file.Close()
}

// WriteTrailer writes the payload checksum trailer. Used when the payload
// was written by seeking the file directly (e.g., parity computation).
func (w *Writer) WriteTrailer() error {
	// Seek to end of payload and read all payload to compute hash
	payloadSize := w.header.PayloadSize
	if _, err := w.file.Seek(HeaderSize, io.SeekStart); err != nil {
		return err
	}

	h := sha256.New()
	if _, err := io.CopyN(h, w.file, int64(payloadSize)); err != nil {
		return err
	}

	checksum := h.Sum(nil)
	// file cursor is now at end of payload, write trailer
	if _, err := w.file.Write(checksum); err != nil {
		return err
	}
	return nil
}
