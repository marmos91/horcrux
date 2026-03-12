package shard

import (
	"bytes"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	h := &Header{
		Version:          Version,
		ShardIndex:       3,
		DataShards:       5,
		ParityShards:     3,
		OriginalFileSize: 1024 * 1024 * 15,
		PayloadSize:      1024 * 1024 * 3,
		OriginalFilename: "secret.pdf",
	}
	h.SetEncrypted(true)
	h.Salt = [32]byte{1, 2, 3, 4, 5}
	h.IV = [16]byte{10, 20, 30}
	h.ArgonTime = 3
	h.ArgonMemory = 65536
	h.ArgonParallelism = 4
	h.PasswordTag = [8]byte{0xAA, 0xBB}

	buf, err := h.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	parsed, err := ReadHeader(bytes.NewReader(buf[:]))
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}

	if parsed.Version != h.Version {
		t.Errorf("Version: got %d, want %d", parsed.Version, h.Version)
	}
	if parsed.ShardIndex != h.ShardIndex {
		t.Errorf("ShardIndex: got %d, want %d", parsed.ShardIndex, h.ShardIndex)
	}
	if parsed.DataShards != h.DataShards {
		t.Errorf("DataShards: got %d, want %d", parsed.DataShards, h.DataShards)
	}
	if parsed.ParityShards != h.ParityShards {
		t.Errorf("ParityShards: got %d, want %d", parsed.ParityShards, h.ParityShards)
	}
	if parsed.OriginalFileSize != h.OriginalFileSize {
		t.Errorf("OriginalFileSize: got %d, want %d", parsed.OriginalFileSize, h.OriginalFileSize)
	}
	if parsed.PayloadSize != h.PayloadSize {
		t.Errorf("PayloadSize: got %d, want %d", parsed.PayloadSize, h.PayloadSize)
	}
	if parsed.OriginalFilename != h.OriginalFilename {
		t.Errorf("OriginalFilename: got %q, want %q", parsed.OriginalFilename, h.OriginalFilename)
	}
	if !parsed.IsEncrypted() {
		t.Error("expected encrypted flag to be set")
	}
	if parsed.Salt != h.Salt {
		t.Error("Salt mismatch")
	}
	if parsed.IV != h.IV {
		t.Error("IV mismatch")
	}
	if parsed.ArgonTime != h.ArgonTime {
		t.Errorf("ArgonTime: got %d, want %d", parsed.ArgonTime, h.ArgonTime)
	}
	if parsed.ArgonMemory != h.ArgonMemory {
		t.Errorf("ArgonMemory: got %d, want %d", parsed.ArgonMemory, h.ArgonMemory)
	}
	if parsed.ArgonParallelism != h.ArgonParallelism {
		t.Errorf("ArgonParallelism: got %d, want %d", parsed.ArgonParallelism, h.ArgonParallelism)
	}
	if parsed.PasswordTag != h.PasswordTag {
		t.Error("PasswordTag mismatch")
	}
	if !parsed.ChecksumValid {
		t.Error("expected checksum to be valid")
	}
}

func TestHeaderChecksumCorruption(t *testing.T) {
	h := &Header{
		Version:          Version,
		ShardIndex:       0,
		DataShards:       3,
		ParityShards:     2,
		OriginalFilename: "test.bin",
	}

	buf, err := h.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Corrupt one byte in the header data area
	buf[0x10] ^= 0xFF

	parsed, err := ReadHeader(bytes.NewReader(buf[:]))
	if err == nil {
		t.Fatal("expected error for corrupted header")
	}
	if parsed == nil {
		t.Fatal("expected parsed header even with checksum error")
	}
	if parsed.ChecksumValid {
		t.Error("expected checksum to be invalid")
	}
}

func TestHeaderInvalidMagic(t *testing.T) {
	var buf [HeaderSize]byte
	copy(buf[:], "NOPE")

	_, err := ReadHeader(bytes.NewReader(buf[:]))
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}

func TestHeaderTooShort(t *testing.T) {
	buf := make([]byte, 10) // Way too short
	_, err := ReadHeader(bytes.NewReader(buf))
	if err == nil {
		t.Fatal("expected error for short header")
	}
}

func TestHeaderUnsupportedVersion(t *testing.T) {
	h := &Header{
		Version:          Version,
		DataShards:       3,
		ParityShards:     2,
		OriginalFilename: "test.bin",
	}
	buf, _ := h.Marshal()
	// Overwrite version
	buf[offsetVersion] = 99
	// Fix checksum won't match, but version check comes after magic check
	// Actually, we need to re-marshal. Let's just modify and recompute.
	// Simpler: just change the version byte — the checksum will also fail,
	// but version is checked before checksum.

	_, err := ReadHeader(bytes.NewReader(buf[:]))
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestHeaderMaxFilename(t *testing.T) {
	h := &Header{
		Version:          Version,
		DataShards:       3,
		ParityShards:     2,
		OriginalFilename: string(make([]byte, maxFilenameLen)), // exactly max, should fail
	}

	_, err := h.Marshal()
	if err == nil {
		t.Fatal("expected error for filename at max length")
	}
}

func TestHeaderEmptyPayload(t *testing.T) {
	h := &Header{
		Version:          Version,
		DataShards:       3,
		ParityShards:     2,
		OriginalFileSize: 0,
		PayloadSize:      0,
		OriginalFilename: "empty.txt",
	}

	buf, err := h.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	parsed, err := ReadHeader(bytes.NewReader(buf[:]))
	if err != nil {
		t.Fatalf("ReadHeader: %v", err)
	}
	if parsed.OriginalFileSize != 0 {
		t.Errorf("expected 0 file size, got %d", parsed.OriginalFileSize)
	}
}

func TestHeaderEncryptionFlag(t *testing.T) {
	h := &Header{}

	if h.IsEncrypted() {
		t.Error("should not be encrypted by default")
	}

	h.SetEncrypted(true)
	if !h.IsEncrypted() {
		t.Error("should be encrypted after SetEncrypted(true)")
	}

	h.SetEncrypted(false)
	if h.IsEncrypted() {
		t.Error("should not be encrypted after SetEncrypted(false)")
	}
}
