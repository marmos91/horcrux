package pipeline

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/marmos91/horcrux/internal/crypto"
	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/erasure"
	"github.com/marmos91/horcrux/internal/shard"
)

// MergeOptions configures the merge operation.
type MergeOptions struct {
	ShardDir       string
	OutputFile     string
	Password       string
	Verbose        bool
	PromptPassword func() (string, error)
}

// shardInfo holds a parsed shard's metadata and path.
type shardInfo struct {
	Path   string
	Header *shard.Header
}

// mergeFileEntry tracks an opened shard file during merge.
type mergeFileEntry struct {
	file       *os.File
	payloadOff int64 // offset where payload starts
	tempFile   bool  // if true, remove on cleanup
}

// Merge reconstructs a file from shards.
func Merge(opts MergeOptions) error {
	shards, err := DiscoverShards(opts.ShardDir)
	if err != nil {
		return err
	}

	if len(shards) == 0 {
		return fmt.Errorf("no valid .hrcx shard files found in %s", opts.ShardDir)
	}

	if opts.Verbose {
		fmt.Printf("Found %d shard files\n", len(shards))
	}

	// Cross-validate headers using first valid shard as reference
	ref := shards[0].Header
	dataShards := int(ref.DataShards)
	parityShards := int(ref.ParityShards)
	totalShards := dataShards + parityShards

	for _, s := range shards[1:] {
		if err := validateConsistency(ref, s.Header, s.Path); err != nil {
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		}
	}

	indexMap := make(map[int]*shardInfo)
	for i := range shards {
		idx := int(shards[i].Header.ShardIndex)
		if _, exists := indexMap[idx]; exists {
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: duplicate shard index %d, using first found\n", idx)
			}
			continue
		}
		indexMap[idx] = &shards[i]
	}

	available := len(indexMap)
	if available < dataShards {
		return fmt.Errorf("only %d shards available, need at least %d of %d total",
			available, dataShards, totalShards)
	}

	encrypted := ref.IsEncrypted()
	var key []byte

	if encrypted {
		pwd := opts.Password
		if pwd == "" {
			if opts.PromptPassword == nil {
				return fmt.Errorf("file is encrypted but no password provided")
			}
			pwd, err = opts.PromptPassword()
			if err != nil {
				return err
			}
		}

		kdfParams := crypto.KDFParams{
			Time:        ref.ArgonTime,
			Memory:      ref.ArgonMemory,
			Parallelism: ref.ArgonParallelism,
		}
		key = crypto.DeriveKey(pwd, ref.Salt, kdfParams)

		if !crypto.VerifyPasswordTag(key, ref.PasswordTag) {
			return fmt.Errorf("wrong password")
		}

		if opts.Verbose {
			fmt.Println("Password verified.")
		}
	}

	// Verify payload checksums, excluding corrupt shards
	validMap := make(map[int]*shardInfo)
	for idx, s := range indexMap {
		reader, err := shard.OpenReader(s.Path)
		if err != nil {
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: cannot open shard %d: %v\n", idx, err)
			}
			continue
		}

		if err := reader.VerifyPayload(); err != nil {
			_ = reader.Close()
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: shard %d has corrupt payload, excluding\n", idx)
			}
			continue
		}
		_ = reader.Close()
		validMap[idx] = s
	}

	available = len(validMap)
	if available < dataShards {
		return fmt.Errorf("only %d valid shards available, need at least %d of %d total",
			available, dataShards, totalShards)
	}

	needReconstruct := false
	for i := range dataShards {
		if _, ok := validMap[i]; !ok {
			needReconstruct = true
			break
		}
	}

	shardFiles := make([]*mergeFileEntry, totalShards)

	defer func() {
		for _, fe := range shardFiles {
			if fe != nil {
				_ = fe.file.Close()
				if fe.tempFile {
					_ = os.Remove(fe.file.Name())
				}
			}
		}
	}()

	for idx, s := range validMap {
		f, err := os.Open(s.Path)
		if err != nil {
			return fmt.Errorf("opening shard %d: %w", idx, err)
		}
		shardFiles[idx] = &mergeFileEntry{file: f, payloadOff: shard.HeaderSize}
	}

	if needReconstruct {
		if opts.Verbose {
			var missing []int
			for i := range totalShards {
				if shardFiles[i] == nil {
					missing = append(missing, i)
				}
			}
			fmt.Printf("Reconstructing %d missing shards: %v\n", len(missing), missing)
		}

		if err := reconstructMissing(shardFiles, ref); err != nil {
			return fmt.Errorf("reconstruction failed: %w", err)
		}
	}

	outputPath := opts.OutputFile
	if outputPath == "" {
		outputPath = ref.OriginalFilename
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	originalSize := int64(ref.OriginalFileSize)

	if originalSize == 0 {
		if opts.Verbose {
			fmt.Println("Empty file — nothing to join.")
		}
	} else {
		dataReaders := make([]io.Reader, dataShards)
		for i := range dataShards {
			fe := shardFiles[i]
			if _, err := fe.file.Seek(fe.payloadOff, io.SeekStart); err != nil {
				return fmt.Errorf("seeking shard %d: %w", i, err)
			}
			dataReaders[i] = io.LimitReader(fe.file, int64(ref.PayloadSize))
		}

		dec, err := erasure.NewDecoder(dataShards, parityShards)
		if err != nil {
			return err
		}

		if opts.Verbose {
			fmt.Println("Joining shards...")
		}

		if encrypted {
			pr, pw := io.Pipe()

			errCh := make(chan error, 1)
			go func() {
				defer func() { _ = pw.Close() }()
				errCh <- dec.Join(pw, dataReaders, originalSize)
			}()

			decReader, err := crypto.NewDecryptReader(pr, key, ref.IV)
			if err != nil {
				_ = pr.Close()
				return fmt.Errorf("creating decrypt reader: %w", err)
			}

			if _, err := io.Copy(outFile, decReader); err != nil {
				return fmt.Errorf("decrypting: %w", err)
			}

			if err := <-errCh; err != nil {
				return fmt.Errorf("joining shards: %w", err)
			}
		} else {
			if err := dec.Join(outFile, dataReaders, originalSize); err != nil {
				return fmt.Errorf("joining shards: %w", err)
			}
		}
	}

	if opts.Verbose {
		fmt.Printf("Recovered: %s (%s)\n", outputPath, display.FormatSize(uint64(originalSize)))
	}

	return nil
}

// MergeDryRunResult holds the computed metadata for a dry-run merge.
type MergeDryRunResult struct {
	OriginalName        string
	OriginalSize        uint64
	DataShards          int
	ParityShards        int
	TotalShards         int
	ShardsFound         int
	ShardsValid         int
	MissingIndices      []int
	CorruptIndices      []int
	Encrypted           bool
	Recoverable         bool
	NeedsReconstruction bool
	OutputFile          string
	RelPath             string // Relative shard dir path from input root (batch mode only)
}

// DryRunMerge analyzes shards and reports recoverability without writing files.
func DryRunMerge(opts MergeOptions) (*MergeDryRunResult, error) {
	shards, err := DiscoverShards(opts.ShardDir)
	if err != nil {
		return nil, err
	}

	if len(shards) == 0 {
		return nil, fmt.Errorf("no valid .hrcx shard files found in %s", opts.ShardDir)
	}

	ref := shards[0].Header
	dataShards := int(ref.DataShards)
	parityShards := int(ref.ParityShards)
	totalShards := dataShards + parityShards

	// Cross-validate headers; exclude inconsistent shards
	inconsistentIndices := make(map[int]struct{})
	for _, s := range shards[1:] {
		if err := validateConsistency(ref, s.Header, s.Path); err != nil {
			inconsistentIndices[int(s.Header.ShardIndex)] = struct{}{}
		}
	}

	// Build index map, detect duplicates, skip inconsistent and out-of-range shards
	indexMap := make(map[int]*shardInfo)
	for i := range shards {
		idx := int(shards[i].Header.ShardIndex)
		if idx < 0 || idx >= totalShards {
			continue
		}
		if _, inconsistent := inconsistentIndices[idx]; inconsistent {
			continue
		}
		if _, exists := indexMap[idx]; exists {
			continue
		}
		indexMap[idx] = &shards[i]
	}

	// Verify payload checksums
	validMap := make(map[int]*shardInfo)
	var corruptIndices []int
	for idx, s := range indexMap {
		reader, err := shard.OpenReader(s.Path)
		if err != nil {
			corruptIndices = append(corruptIndices, idx)
			continue
		}

		if err := reader.VerifyPayload(); err != nil {
			_ = reader.Close()
			corruptIndices = append(corruptIndices, idx)
			continue
		}
		_ = reader.Close()
		validMap[idx] = s
	}

	// Determine truly missing indices (not found on disk at all)
	var missingIndices []int
	for i := 0; i < totalShards; i++ {
		if _, inIndex := indexMap[i]; !inIndex {
			missingIndices = append(missingIndices, i)
		}
	}

	recoverable := len(validMap) >= dataShards

	needsReconstruction := false
	if recoverable {
		for i := 0; i < dataShards; i++ {
			if _, ok := validMap[i]; !ok {
				needsReconstruction = true
				break
			}
		}
	}

	outputFile := opts.OutputFile
	if outputFile == "" {
		outputFile = ref.OriginalFilename
	}

	sort.Ints(missingIndices)
	sort.Ints(corruptIndices)

	return &MergeDryRunResult{
		OriginalName:        ref.OriginalFilename,
		OriginalSize:        ref.OriginalFileSize,
		DataShards:          dataShards,
		ParityShards:        parityShards,
		TotalShards:         totalShards,
		ShardsFound:         len(indexMap),
		ShardsValid:         len(validMap),
		MissingIndices:      missingIndices,
		CorruptIndices:      corruptIndices,
		Encrypted:           ref.IsEncrypted(),
		Recoverable:         recoverable,
		NeedsReconstruction: needsReconstruction,
		OutputFile:          outputFile,
	}, nil
}

// DiscoverShards finds and parses all .hrcx shard files in a directory,
// returning them sorted by shard index.
func DiscoverShards(dir string) ([]shardInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading shard directory: %w", err)
	}

	var shards []shardInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".hrcx") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		header, err := shard.ReadHeader(f)
		_ = f.Close()
		if err != nil {
			if errors.Is(err, shard.ErrHeaderChecksum) && header != nil {
				shards = append(shards, shardInfo{Path: path, Header: header})
			}
			continue
		}

		shards = append(shards, shardInfo{Path: path, Header: header})
	}

	sort.Slice(shards, func(i, j int) bool {
		return shards[i].Header.ShardIndex < shards[j].Header.ShardIndex
	})

	return shards, nil
}

func validateConsistency(ref, other *shard.Header, path string) error {
	if ref.DataShards != other.DataShards || ref.ParityShards != other.ParityShards {
		return fmt.Errorf("%s: shard count mismatch (expected %d+%d, got %d+%d)",
			path, ref.DataShards, ref.ParityShards, other.DataShards, other.ParityShards)
	}
	if ref.OriginalFileSize != other.OriginalFileSize {
		return fmt.Errorf("%s: file size mismatch", path)
	}
	if ref.OriginalFilename != other.OriginalFilename {
		return fmt.Errorf("%s: filename mismatch", path)
	}
	if ref.Salt != other.Salt {
		return fmt.Errorf("%s: salt mismatch (shards from different split operations?)", path)
	}
	return nil
}

func reconstructMissing(shardFiles []*mergeFileEntry, ref *shard.Header) error {
	dataShards := int(ref.DataShards)
	parityShards := int(ref.ParityShards)
	totalShards := dataShards + parityShards

	dec, err := erasure.NewDecoder(dataShards, parityShards)
	if err != nil {
		return err
	}

	rsReaders := make([]io.Reader, totalShards)
	rsWriters := make([]io.Writer, totalShards)

	for i := range totalShards {
		if shardFiles[i] != nil {
			fe := shardFiles[i]
			if _, err := fe.file.Seek(fe.payloadOff, io.SeekStart); err != nil {
				return fmt.Errorf("seeking shard %d: %w", i, err)
			}
			rsReaders[i] = io.LimitReader(fe.file, int64(ref.PayloadSize))
		} else {
			tmp, err := os.CreateTemp("", fmt.Sprintf("hrcx-reconstruct-%d-*", i))
			if err != nil {
				return fmt.Errorf("creating temp file for shard %d: %w", i, err)
			}
			rsWriters[i] = tmp
			shardFiles[i] = &mergeFileEntry{file: tmp, payloadOff: 0, tempFile: true}
		}
	}

	return dec.Reconstruct(rsReaders, rsWriters)
}
