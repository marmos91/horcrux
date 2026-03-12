package shard

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	MagicBytes  = "HRCX"
	HeaderSize  = 256
	TrailerSize = 32
	Version     = 1

	maxFilenameLen = 128

	offsetMagic          = 0x00
	offsetVersion        = 0x04
	offsetShardIndex     = 0x05
	offsetDataShards     = 0x06
	offsetParityShards   = 0x07
	offsetOrigFileSize   = 0x08
	offsetPayloadSize    = 0x10
	offsetEncryptFlags   = 0x18
	offsetSalt           = 0x19
	offsetIV             = 0x39
	offsetArgonTime      = 0x49
	offsetArgonMemory    = 0x4D
	offsetArgonParallel  = 0x51
	offsetFilename       = 0x52
	offsetPasswordTag    = 0xD2
	offsetReserved       = 0xDA
	offsetHeaderChecksum = 0xE0
)

var (
	ErrInvalidMagic      = errors.New("invalid magic bytes")
	ErrUnsupportedVer    = errors.New("unsupported format version")
	ErrHeaderChecksum    = errors.New("header checksum mismatch")
	ErrHeaderTooShort    = errors.New("header too short")
	ErrFilenameTooLong   = fmt.Errorf("filename exceeds %d bytes", maxFilenameLen)
)

type Header struct {
	Version          uint8
	ShardIndex       uint8
	DataShards       uint8
	ParityShards     uint8
	OriginalFileSize uint64
	PayloadSize      uint64
	EncryptionFlags  uint8
	Salt             [32]byte
	IV               [16]byte
	ArgonTime        uint32
	ArgonMemory      uint32
	ArgonParallelism uint8
	OriginalFilename string
	PasswordTag      [8]byte

	ChecksumValid bool // set by ReadHeader
}

func (h *Header) IsEncrypted() bool {
	return h.EncryptionFlags&0x01 != 0
}

func (h *Header) SetEncrypted(encrypted bool) {
	if encrypted {
		h.EncryptionFlags |= 0x01
	} else {
		h.EncryptionFlags &^= 0x01
	}
}

func (h *Header) Marshal() ([HeaderSize]byte, error) {
	var buf [HeaderSize]byte

	copy(buf[offsetMagic:], MagicBytes)
	buf[offsetVersion] = h.Version
	buf[offsetShardIndex] = h.ShardIndex
	buf[offsetDataShards] = h.DataShards
	buf[offsetParityShards] = h.ParityShards
	binary.BigEndian.PutUint64(buf[offsetOrigFileSize:], h.OriginalFileSize)
	binary.BigEndian.PutUint64(buf[offsetPayloadSize:], h.PayloadSize)
	buf[offsetEncryptFlags] = h.EncryptionFlags
	copy(buf[offsetSalt:], h.Salt[:])
	copy(buf[offsetIV:], h.IV[:])
	binary.BigEndian.PutUint32(buf[offsetArgonTime:], h.ArgonTime)
	binary.BigEndian.PutUint32(buf[offsetArgonMemory:], h.ArgonMemory)
	buf[offsetArgonParallel] = h.ArgonParallelism

	fnBytes := []byte(h.OriginalFilename)
	if len(fnBytes) >= maxFilenameLen {
		return buf, ErrFilenameTooLong
	}
	copy(buf[offsetFilename:], fnBytes)

	copy(buf[offsetPasswordTag:], h.PasswordTag[:])

	// Compute header checksum over bytes 0x00–0xDF
	checksum := sha256.Sum256(buf[:offsetHeaderChecksum])
	copy(buf[offsetHeaderChecksum:], checksum[:])

	return buf, nil
}

func ReadHeader(r io.Reader) (*Header, error) {
	var buf [HeaderSize]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrHeaderTooShort
		}
		return nil, fmt.Errorf("reading header: %w", err)
	}

	if string(buf[offsetMagic:offsetMagic+4]) != MagicBytes {
		return nil, ErrInvalidMagic
	}

	if buf[offsetVersion] != Version {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVer, buf[offsetVersion], Version)
	}

	// Verify checksum
	expectedChecksum := sha256.Sum256(buf[:offsetHeaderChecksum])
	var storedChecksum [32]byte
	copy(storedChecksum[:], buf[offsetHeaderChecksum:])

	h := &Header{
		Version:          buf[offsetVersion],
		ShardIndex:       buf[offsetShardIndex],
		DataShards:       buf[offsetDataShards],
		ParityShards:     buf[offsetParityShards],
		OriginalFileSize: binary.BigEndian.Uint64(buf[offsetOrigFileSize:]),
		PayloadSize:      binary.BigEndian.Uint64(buf[offsetPayloadSize:]),
		EncryptionFlags:  buf[offsetEncryptFlags],
		ArgonTime:        binary.BigEndian.Uint32(buf[offsetArgonTime:]),
		ArgonMemory:      binary.BigEndian.Uint32(buf[offsetArgonMemory:]),
		ArgonParallelism: buf[offsetArgonParallel],
		ChecksumValid:    expectedChecksum == storedChecksum,
	}

	copy(h.Salt[:], buf[offsetSalt:])
	copy(h.IV[:], buf[offsetIV:])
	copy(h.PasswordTag[:], buf[offsetPasswordTag:])

	// Read filename (null-terminated)
	fnBytes := buf[offsetFilename : offsetFilename+maxFilenameLen]
	if idx := bytes.IndexByte(fnBytes[:], 0); idx >= 0 {
		fnBytes = fnBytes[:idx]
	}
	h.OriginalFilename = string(fnBytes)

	if !h.ChecksumValid {
		return h, ErrHeaderChecksum
	}

	return h, nil
}
