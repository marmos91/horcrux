package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleManifest() *Manifest {
	return &Manifest{
		Version:        SchemaVersion,
		HorcruxVersion: "dev",
		CreatedAt:      time.Now().UTC(),
		Original: OriginalFile{
			Filename: "secret.pdf",
			Size:     15728640,
			SHA256:   "a1b2c3d4e5f6",
		},
		Erasure: ErasureConfig{
			DataShards:        5,
			ParityShards:      3,
			TotalShards:       8,
			MinShardsRequired: 5,
		},
		Encryption: EncryptionInfo{
			Encrypted: true,
			Algorithm: "AES-256-CTR",
			KDF:       "Argon2id",
			KDFParams: &KDFParams{
				Time:        3,
				MemoryKB:    65536,
				Parallelism: 4,
			},
		},
		Shards: []ShardEntry{
			{Index: 0, Type: "data", Filename: "secret.pdf.000.hrcx", Size: 3146080, SHA256: "aaa"},
			{Index: 1, Type: "data", Filename: "secret.pdf.001.hrcx", Size: 3146080, SHA256: "bbb"},
			{Index: 2, Type: "data", Filename: "secret.pdf.002.hrcx", Size: 3146080, SHA256: "ccc"},
			{Index: 3, Type: "data", Filename: "secret.pdf.003.hrcx", Size: 3146080, SHA256: "ddd"},
			{Index: 4, Type: "data", Filename: "secret.pdf.004.hrcx", Size: 3146080, SHA256: "eee"},
			{Index: 5, Type: "parity", Filename: "secret.pdf.005.hrcx", Size: 3146080, SHA256: "fff"},
			{Index: 6, Type: "parity", Filename: "secret.pdf.006.hrcx", Size: 3146080, SHA256: "ggg"},
			{Index: 7, Type: "parity", Filename: "secret.pdf.007.hrcx", Size: 3146080, SHA256: "hhh"},
		},
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	m := sampleManifest()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.manifest.json")

	if err := m.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Version != m.Version {
		t.Errorf("Version: got %q, want %q", loaded.Version, m.Version)
	}
	if loaded.Original.Filename != m.Original.Filename {
		t.Errorf("Filename: got %q, want %q", loaded.Original.Filename, m.Original.Filename)
	}
	if loaded.Original.Size != m.Original.Size {
		t.Errorf("Size: got %d, want %d", loaded.Original.Size, m.Original.Size)
	}
	if loaded.Original.SHA256 != m.Original.SHA256 {
		t.Errorf("SHA256: got %q, want %q", loaded.Original.SHA256, m.Original.SHA256)
	}
	if loaded.Erasure.DataShards != m.Erasure.DataShards {
		t.Errorf("DataShards: got %d, want %d", loaded.Erasure.DataShards, m.Erasure.DataShards)
	}
	if loaded.Erasure.ParityShards != m.Erasure.ParityShards {
		t.Errorf("ParityShards: got %d, want %d", loaded.Erasure.ParityShards, m.Erasure.ParityShards)
	}
	if loaded.Encryption.Encrypted != m.Encryption.Encrypted {
		t.Errorf("Encrypted: got %v, want %v", loaded.Encryption.Encrypted, m.Encryption.Encrypted)
	}
	if loaded.Encryption.Algorithm != m.Encryption.Algorithm {
		t.Errorf("Algorithm: got %q, want %q", loaded.Encryption.Algorithm, m.Encryption.Algorithm)
	}
	if loaded.Encryption.KDFParams.Time != m.Encryption.KDFParams.Time {
		t.Errorf("KDF Time: got %d, want %d", loaded.Encryption.KDFParams.Time, m.Encryption.KDFParams.Time)
	}
	if len(loaded.Shards) != len(m.Shards) {
		t.Fatalf("Shards: got %d, want %d", len(loaded.Shards), len(m.Shards))
	}
	for i := range m.Shards {
		if loaded.Shards[i].Index != m.Shards[i].Index {
			t.Errorf("Shard[%d] Index: got %d, want %d", i, loaded.Shards[i].Index, m.Shards[i].Index)
		}
		if loaded.Shards[i].Type != m.Shards[i].Type {
			t.Errorf("Shard[%d] Type: got %q, want %q", i, loaded.Shards[i].Type, m.Shards[i].Type)
		}
		if loaded.Shards[i].SHA256 != m.Shards[i].SHA256 {
			t.Errorf("Shard[%d] SHA256: got %q, want %q", i, loaded.Shards[i].SHA256, m.Shards[i].SHA256)
		}
	}
}

func TestValidate_Valid(t *testing.T) {
	m := sampleManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_MissingVersion(t *testing.T) {
	m := sampleManifest()
	m.Version = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestValidate_MissingFilename(t *testing.T) {
	m := sampleManifest()
	m.Original.Filename = ""
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing filename")
	}
}

func TestValidate_ShardCountMismatch(t *testing.T) {
	m := sampleManifest()
	m.Shards = m.Shards[:5] // remove 3 shards
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for shard count mismatch")
	}
}

func TestValidate_TotalShardsMismatch(t *testing.T) {
	m := sampleManifest()
	m.Erasure.TotalShards = 10
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for total shards mismatch")
	}
}

func TestValidate_BadShardIndex(t *testing.T) {
	m := sampleManifest()
	m.Shards[3].Index = 99
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for bad shard index")
	}
}

func TestValidate_BadShardType(t *testing.T) {
	m := sampleManifest()
	m.Shards[0].Type = "unknown"
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for bad shard type")
	}
}

func TestValidate_MinShardsRequired(t *testing.T) {
	m := sampleManifest()
	m.Erasure.MinShardsRequired = 3
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for wrong min_shards_required")
	}
}

func TestManifestFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"secret.pdf", "secret.pdf.manifest.json"},
		{"archive.tar.gz", "archive.tar.gz.manifest.json"},
		{"simple", "simple.manifest.json"},
	}
	for _, tc := range tests {
		got := ManifestFilename(tc.input)
		if got != tc.want {
			t.Errorf("ManifestFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFindShardByIndex(t *testing.T) {
	m := sampleManifest()

	s := m.FindShardByIndex(0)
	if s == nil || s.Filename != "secret.pdf.000.hrcx" {
		t.Errorf("FindShardByIndex(0): got %v", s)
	}

	s = m.FindShardByIndex(7)
	if s == nil || s.Type != "parity" {
		t.Errorf("FindShardByIndex(7): got %v", s)
	}

	if m.FindShardByIndex(-1) != nil {
		t.Error("FindShardByIndex(-1) should return nil")
	}
	if m.FindShardByIndex(100) != nil {
		t.Error("FindShardByIndex(100) should return nil")
	}
}

func TestLoad_NonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSave_Unwritable(t *testing.T) {
	m := sampleManifest()
	err := m.Save("/nonexistent/dir/manifest.json")
	if err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

func TestNoEncryption(t *testing.T) {
	m := sampleManifest()
	m.Encryption = EncryptionInfo{Encrypted: false}
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.manifest.json")
	if err := m.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Encryption.Encrypted {
		t.Error("expected encrypted=false")
	}
	if loaded.Encryption.Algorithm != "" {
		t.Errorf("expected empty algorithm, got %q", loaded.Encryption.Algorithm)
	}
	if loaded.Encryption.KDFParams != nil {
		t.Error("expected nil KDFParams")
	}
}
