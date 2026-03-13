package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Manifest describes a set of shards produced by a split operation.
type Manifest struct {
	Version        string         `json:"version"`
	HorcruxVersion string         `json:"horcrux_version"`
	CreatedAt      time.Time      `json:"created_at"`
	Original       OriginalFile   `json:"original"`
	Erasure        ErasureConfig  `json:"erasure"`
	Encryption     EncryptionInfo `json:"encryption"`
	Shards         []ShardEntry   `json:"shards"`
}

// OriginalFile records metadata about the file that was split.
type OriginalFile struct {
	Filename string `json:"filename"`
	Size     uint64 `json:"size"`
	SHA256   string `json:"sha256"`
}

// ErasureConfig records the Reed-Solomon parameters used.
type ErasureConfig struct {
	DataShards        int `json:"data_shards"`
	ParityShards      int `json:"parity_shards"`
	TotalShards       int `json:"total_shards"`
	MinShardsRequired int `json:"min_shards_required"`
}

// EncryptionInfo records the encryption algorithm and KDF parameters.
// Never contains secrets (no salt, IV, or key material).
type EncryptionInfo struct {
	Encrypted bool       `json:"encrypted"`
	Algorithm string     `json:"algorithm,omitempty"`
	KDF       string     `json:"kdf,omitempty"`
	KDFParams *KDFParams `json:"kdf_params,omitempty"`
}

// KDFParams records the Argon2id parameters used for key derivation.
type KDFParams struct {
	Time        uint32 `json:"time"`
	MemoryKB    uint32 `json:"memory_kb"`
	Parallelism uint8  `json:"parallelism"`
}

// ShardEntry describes a single shard file.
type ShardEntry struct {
	Index    int    `json:"index"`
	Type     string `json:"type"`
	Filename string `json:"filename"`
	Size     uint64 `json:"size"`
	SHA256   string `json:"sha256"`
	Location string `json:"location,omitempty"`
}

// SchemaVersion is the current manifest schema version.
const SchemaVersion = "1.0.0"

// Save writes the manifest as indented JSON to the given path.
func (m *Manifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	return nil
}

// Load reads and parses a manifest JSON file.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return &m, nil
}

// SupportedVersions lists all manifest schema versions this build can handle.
var SupportedVersions = map[string]bool{
	SchemaVersion: true,
}

// Validate checks structural consistency of the manifest.
func (m *Manifest) Validate() error {
	if m.Version == "" {
		return fmt.Errorf("missing version")
	}
	if !SupportedVersions[m.Version] {
		return fmt.Errorf("unsupported manifest version %q (supported: %s)", m.Version, SchemaVersion)
	}
	if m.Original.Filename == "" {
		return fmt.Errorf("missing original filename")
	}
	if m.Erasure.TotalShards <= 0 {
		return fmt.Errorf("total_shards must be > 0")
	}
	if m.Erasure.DataShards <= 0 {
		return fmt.Errorf("data_shards must be > 0")
	}
	if m.Erasure.ParityShards < 0 {
		return fmt.Errorf("parity_shards must be >= 0")
	}
	if m.Erasure.DataShards+m.Erasure.ParityShards != m.Erasure.TotalShards {
		return fmt.Errorf("data_shards + parity_shards != total_shards")
	}
	if m.Erasure.MinShardsRequired != m.Erasure.DataShards {
		return fmt.Errorf("min_shards_required must equal data_shards")
	}
	if len(m.Shards) != m.Erasure.TotalShards {
		return fmt.Errorf("shard count %d != total_shards %d", len(m.Shards), m.Erasure.TotalShards)
	}
	for i, s := range m.Shards {
		if s.Index != i {
			return fmt.Errorf("shard %d has index %d", i, s.Index)
		}
		if s.Type != "data" && s.Type != "parity" {
			return fmt.Errorf("shard %d has invalid type %q", i, s.Type)
		}
		if s.Filename == "" {
			return fmt.Errorf("shard %d has empty filename", i)
		}
	}
	return nil
}

// ManifestFilename returns the manifest filename for a given original filename.
func ManifestFilename(originalName string) string {
	return originalName + ".manifest.json"
}

// FindShardByIndex returns the shard entry with the given index, or nil.
func (m *Manifest) FindShardByIndex(index int) *ShardEntry {
	if index < 0 || index >= len(m.Shards) {
		return nil
	}
	return &m.Shards[index]
}

// Summary returns a human-readable summary of the manifest.
func (m *Manifest) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Manifest: %s\n", m.Original.Filename)
	fmt.Fprintf(&b, "  Original size: %d bytes\n", m.Original.Size)
	fmt.Fprintf(&b, "  SHA-256:       %s\n", m.Original.SHA256)
	fmt.Fprintf(&b, "  Shards:        %d data + %d parity = %d total (need %d)\n",
		m.Erasure.DataShards, m.Erasure.ParityShards,
		m.Erasure.TotalShards, m.Erasure.MinShardsRequired)
	fmt.Fprintf(&b, "  Encrypted:     %v\n", m.Encryption.Encrypted)
	return b.String()
}
