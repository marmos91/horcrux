package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/marmos91/horcrux/internal/manifest"
	"github.com/marmos91/horcrux/internal/shard"
)

// ShardStatus describes the verification state of a single shard slot.
type ShardStatus struct {
	Index          int
	Path           string
	Filename       string
	Type           string // "data" or "parity"
	HeaderValid    bool
	PayloadValid   bool
	ConsistencyOK  bool
	ManifestHashOK *bool // nil if no manifest or shard missing
	Error          error
}

// VerifyResult holds the outcome of verifying a shard set.
type VerifyResult struct {
	Dir            string
	RelPath        string // relative path from batch root (batch mode only)
	OriginalName   string
	OriginalSize   uint64
	DataShards     int
	ParityShards   int
	TotalShards    int
	ShardsFound    int
	ShardsValid    int
	ShardsCorrupt  int
	ShardsMissing  int
	Encrypted      bool
	Recoverable    bool
	ManifestFound  bool
	ShardStatuses  []ShardStatus
	MissingIndices []int
	CorruptIndices []int
}

// Verify checks shard integrity and recoverability in a single directory.
func Verify(dir string) (*VerifyResult, error) {
	shards, err := DiscoverShards(dir)
	if err != nil {
		return nil, err
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("no valid .hrcx shard files found in %s", dir)
	}

	ref := referenceHeader(shards)
	dataShards := int(ref.DataShards)
	parityShards := int(ref.ParityShards)
	totalShards := dataShards + parityShards

	// Cross-validate headers against reference
	inconsistent := make(map[int]struct{})
	for _, s := range shards {
		if s.Header == ref {
			continue
		}
		if err := validateConsistency(ref, s.Header, s.Path); err != nil {
			inconsistent[int(s.Header.ShardIndex)] = struct{}{}
		}
	}

	// Build index map for all valid shards (including inconsistent ones for reporting).
	// Skip out-of-range and duplicate shards.
	indexMap := make(map[int]*shardInfo)
	for i := range shards {
		idx := int(shards[i].Header.ShardIndex)
		if idx >= totalShards {
			continue
		}
		if _, dup := indexMap[idx]; dup {
			continue
		}
		indexMap[idx] = &shards[i]
	}

	// Verify payload checksums (skip inconsistent shards; they count as corrupt)
	payloadValid := make(map[int]bool, len(indexMap))
	var corruptIndices []int
	for idx, s := range indexMap {
		if _, isInconsistent := inconsistent[idx]; isInconsistent {
			corruptIndices = append(corruptIndices, idx)
			continue
		}
		if verifyShardPayload(s.Path) {
			payloadValid[idx] = true
		} else {
			corruptIndices = append(corruptIndices, idx)
		}
	}
	sort.Ints(corruptIndices)

	// Determine missing indices (only truly absent shards)
	var missingIndices []int
	for i := range totalShards {
		if _, found := indexMap[i]; !found {
			missingIndices = append(missingIndices, i)
		}
	}

	m, manifestFound := loadManifestFromDir(dir, ref.OriginalFilename)

	// Build per-shard status for all slots 0..total-1
	statuses := make([]ShardStatus, totalShards)
	for i := range totalShards {
		st := ShardStatus{
			Index: i,
			Type:  shardType(i, dataShards),
		}

		s, found := indexMap[i]
		if !found {
			st.Filename = "(missing)"
			statuses[i] = st
			continue
		}

		st.Path = s.Path
		st.Filename = filepath.Base(s.Path)
		st.HeaderValid = s.Header.ChecksumValid
		_, isInconsistent := inconsistent[i]
		st.ConsistencyOK = !isInconsistent
		st.PayloadValid = !isInconsistent && payloadValid[i]
		st.ManifestHashOK = checkManifestHash(m, i, s.Path)
		statuses[i] = st
	}

	validCount := len(payloadValid)
	return &VerifyResult{
		Dir:            dir,
		OriginalName:   ref.OriginalFilename,
		OriginalSize:   ref.OriginalFileSize,
		DataShards:     dataShards,
		ParityShards:   parityShards,
		TotalShards:    totalShards,
		ShardsFound:    len(indexMap),
		ShardsValid:    validCount,
		ShardsCorrupt:  len(corruptIndices),
		ShardsMissing:  len(missingIndices),
		Encrypted:      ref.IsEncrypted(),
		Recoverable:    validCount >= dataShards,
		ManifestFound:  manifestFound,
		ShardStatuses:  statuses,
		MissingIndices: missingIndices,
		CorruptIndices: corruptIndices,
	}, nil
}

// VerifyBatch verifies all shard directories under a batch root.
func VerifyBatch(dir string) ([]VerifyResult, error) {
	shardDirs, err := findShardDirs(dir)
	if err != nil {
		return nil, err
	}
	if len(shardDirs) == 0 {
		return nil, fmt.Errorf("no shard directories found in %s", dir)
	}

	results := make([]VerifyResult, 0, len(shardDirs))
	for _, sd := range shardDirs {
		rel, err := filepath.Rel(dir, sd)
		if err != nil {
			return nil, fmt.Errorf("computing relative path: %w", err)
		}

		r, err := Verify(sd)
		if err != nil {
			return nil, fmt.Errorf("verify %s: %w", rel, err)
		}
		r.RelPath = rel
		results = append(results, *r)
	}

	return results, nil
}

// referenceHeader returns the header from the first shard with a valid checksum,
// falling back to the first shard if none have valid checksums.
func referenceHeader(shards []shardInfo) *shard.Header {
	for i := range shards {
		if shards[i].Header.ChecksumValid {
			return shards[i].Header
		}
	}
	return shards[0].Header
}

// verifyShardPayload opens a shard file and verifies its payload checksum.
func verifyShardPayload(path string) bool {
	reader, err := shard.OpenReader(path)
	if err != nil {
		return false
	}
	defer func() { _ = reader.Close() }()
	return reader.VerifyPayload() == nil
}

// checkManifestHash compares the SHA-256 of a shard file against the manifest entry.
// Returns nil if no manifest is loaded, the shard is unlisted, or the file cannot be hashed.
func checkManifestHash(m *manifest.Manifest, index int, path string) *bool {
	if m == nil {
		return nil
	}
	entry := m.FindShardByIndex(index)
	if entry == nil {
		return nil
	}
	fileHash, _, err := HashFile(path)
	if err != nil {
		return nil
	}
	return boolPtr(fileHash == entry.SHA256)
}

func boolPtr(v bool) *bool { return &v }

// loadManifestFromDir attempts to load a manifest file from dir.
func loadManifestFromDir(dir, originalFilename string) (*manifest.Manifest, bool) {
	// Try the standard manifest filename
	mPath := filepath.Join(dir, manifest.ManifestFilename(originalFilename))
	m, err := manifest.Load(mPath)
	if err == nil {
		return m, true
	}

	// Fall back to scanning for any *.manifest.json that matches the shard set
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".manifest.json") {
			m, err := manifest.Load(filepath.Join(dir, e.Name()))
			if err == nil && m.Original.Filename == originalFilename {
				return m, true
			}
		}
	}

	return nil, false
}
