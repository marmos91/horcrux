package pipeline

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/marmos91/horcrux/internal/crypto"
	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/erasure"
	"github.com/marmos91/horcrux/internal/manifest"
	"github.com/marmos91/horcrux/internal/progress"
	"github.com/marmos91/horcrux/internal/shard"
	"github.com/marmos91/horcrux/internal/version"
)

// SplitOptions configures the split operation.
type SplitOptions struct {
	InputFile    string
	OutputDir    string
	DataShards   int
	ParityShards int
	Password     string
	KeyFile      string
	NoEncrypt    bool
	NoManifest   bool
	Verbose      bool
	Progress     progress.Reporter
}

// SplitResult contains metadata collected during a split operation.
type SplitResult struct {
	OriginalName     string
	OriginalSHA256   string
	OriginalSize     uint64
	DataShards       int
	ParityShards     int
	Encrypted        bool
	KeyFileUsed      bool
	ArgonTime        uint32
	ArgonMemory      uint32
	ArgonParallelism uint8
	ShardFiles       []ShardFileInfo
}

// ShardFileInfo describes a shard file produced by a split.
type ShardFileInfo struct {
	Index    int
	Type     string // "data" or "parity"
	Filename string
	Path     string
	Size     uint64
	SHA256   string
	Location string // Backend URI where shard was distributed (empty if local-only)
}

// BuildManifest constructs a Manifest from a SplitResult.
func (r *SplitResult) BuildManifest() *manifest.Manifest {
	m := &manifest.Manifest{
		Version:        manifest.SchemaVersion,
		HorcruxVersion: version.Version,
		CreatedAt:      time.Now().UTC(),
		Original: manifest.OriginalFile{
			Filename: r.OriginalName,
			Size:     r.OriginalSize,
			SHA256:   r.OriginalSHA256,
		},
		Erasure: manifest.ErasureConfig{
			DataShards:        r.DataShards,
			ParityShards:      r.ParityShards,
			TotalShards:       r.DataShards + r.ParityShards,
			MinShardsRequired: r.DataShards,
		},
		Shards: make([]manifest.ShardEntry, len(r.ShardFiles)),
	}

	if r.Encrypted {
		m.Encryption = manifest.EncryptionInfo{
			Encrypted:   true,
			KeyFileUsed: r.KeyFileUsed,
			Algorithm:   "AES-256-CTR",
			KDF:         "Argon2id",
			KDFParams: &manifest.KDFParams{
				Time:        r.ArgonTime,
				MemoryKB:    r.ArgonMemory,
				Parallelism: r.ArgonParallelism,
			},
		}
	}

	for i, sf := range r.ShardFiles {
		m.Shards[i] = manifest.ShardEntry{
			Index:    sf.Index,
			Type:     sf.Type,
			Filename: sf.Filename,
			Size:     sf.Size,
			SHA256:   sf.SHA256,
			Location: sf.Location,
		}
	}

	return m
}

// Split splits a file into encrypted, erasure-coded shards.
func Split(opts SplitOptions) (result *SplitResult, err error) {
	inputFile, err := os.Open(opts.InputFile)
	if err != nil {
		return nil, fmt.Errorf("opening input file: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	inputInfo, err := inputFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat input file: %w", err)
	}
	originalSize := uint64(inputInfo.Size())
	originalName := filepath.Base(opts.InputFile)

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	var (
		key             []byte
		salt            [32]byte
		iv              [16]byte
		pwTag           [8]byte
		kdfParams       crypto.KDFParams
		keyFileMaterial []byte
	)

	encrypt := !opts.NoEncrypt
	if encrypt {
		if opts.KeyFile != "" {
			kfHash, kfErr := crypto.ReadKeyFile(opts.KeyFile)
			if kfErr != nil {
				return nil, kfErr
			}
			keyFileMaterial = kfHash[:]
		}

		kdfParams = crypto.DefaultKDFParams()
		salt, err = crypto.GenerateSalt()
		if err != nil {
			return nil, fmt.Errorf("generating salt: %w", err)
		}
		iv, err = crypto.GenerateIV()
		if err != nil {
			return nil, fmt.Errorf("generating IV: %w", err)
		}
		key = crypto.DeriveKey(opts.Password, keyFileMaterial, salt, kdfParams)
		pwTag = crypto.PasswordTag(key)
	}

	totalShards := opts.DataShards + opts.ParityShards

	// RS Split distributes input evenly across N data shards, rounding up.
	// CTR mode produces no padding, so encrypted size equals original size.
	perShard := (int64(originalSize) + int64(opts.DataShards) - 1) / int64(opts.DataShards)

	writers := make([]*shard.Writer, totalShards)
	for i := range totalShards {
		hdr := &shard.Header{
			Version:          shard.Version,
			ShardIndex:       uint8(i),
			DataShards:       uint8(opts.DataShards),
			ParityShards:     uint8(opts.ParityShards),
			OriginalFileSize: originalSize,
			PayloadSize:      uint64(perShard),
			OriginalFilename: originalName,
		}

		if encrypt {
			hdr.SetEncrypted(true)
			hdr.SetKeyFileUsed(len(keyFileMaterial) > 0)
			hdr.SetPasswordUsed(opts.Password != "")
			hdr.Salt = salt
			hdr.IV = iv
			hdr.ArgonTime = kdfParams.Time
			hdr.ArgonMemory = kdfParams.Memory
			hdr.ArgonParallelism = kdfParams.Parallelism
			hdr.PasswordTag = pwTag
		}

		shardPath := filepath.Join(opts.OutputDir, shardFilename(originalName, i))
		w, err := shard.CreateWriter(shardPath, hdr)
		if err != nil {
			for j := range i {
				_ = writers[j].Close()
			}
			return nil, fmt.Errorf("creating shard %d: %w", i, err)
		}
		writers[i] = w
	}

	defer func() {
		for _, w := range writers {
			if w != nil {
				_ = w.Close()
			}
		}
	}()

	prog := progress.OrNop(opts.Progress)
	showVerbose := opts.Verbose && opts.Progress == nil

	if showVerbose {
		fmt.Printf("Splitting %s (%s) into %d+%d shards\n",
			originalName, display.FormatSize(originalSize), opts.DataShards, opts.ParityShards)
	}

	fileProgress := prog.StartFile(originalName, int64(originalSize))
	defer func() {
		fileProgress.Finish()
		prog.FinishFile(originalName, err)
	}()

	// Hash the original plaintext for the manifest
	originalHash := sha256.New()

	if originalSize == 0 {
		if showVerbose {
			fmt.Println("Empty file — writing headers only.")
		}
		for i, w := range writers {
			if err := w.WriteTrailer(); err != nil {
				return nil, fmt.Errorf("writing trailer for shard %d: %w", i, err)
			}
		}
	} else {
		dataWriters := make([]io.Writer, opts.DataShards)
		for i := range opts.DataShards {
			dataWriters[i] = writers[i]
		}

		enc, err := erasure.NewEncoder(opts.DataShards, opts.ParityShards)
		if err != nil {
			return nil, err
		}

		// Hash original plaintext before encryption
		inputReader := io.TeeReader(inputFile, originalHash)

		// Optionally encrypt, then split into data shards
		if encrypt {
			inputReader, err = crypto.NewEncryptReader(inputReader, key, iv)
			if err != nil {
				return nil, fmt.Errorf("creating encryption reader: %w", err)
			}
		}

		// Wrap the input reader (not data writers) for accurate byte counting
		inputReader = fileProgress.WrapReader(inputReader)

		if showVerbose {
			fmt.Println("Writing data shards...")
		}

		if err := enc.Split(inputReader, dataWriters, int64(originalSize)); err != nil {
			return nil, fmt.Errorf("splitting data: %w", err)
		}

		// Seek data shard files back to payload start for parity computation
		dataReaders := make([]io.Reader, opts.DataShards)
		for i := range opts.DataShards {
			f := writers[i].File()
			if _, err := f.Seek(shard.HeaderSize, io.SeekStart); err != nil {
				return nil, fmt.Errorf("seeking data shard %d: %w", i, err)
			}
			dataReaders[i] = io.LimitReader(f, perShard)
		}

		parityWriters := make([]io.Writer, opts.ParityShards)
		for i := range opts.ParityShards {
			parityWriters[i] = writers[opts.DataShards+i]
		}

		if showVerbose {
			fmt.Println("Computing parity shards...")
		}

		if err := enc.Encode(dataReaders, parityWriters); err != nil {
			return nil, fmt.Errorf("encoding parity: %w", err)
		}

		if showVerbose {
			fmt.Println("Writing checksums...")
		}

		for i, w := range writers {
			if err := w.WriteTrailer(); err != nil {
				return nil, fmt.Errorf("writing trailer for shard %d: %w", i, err)
			}
		}
	}

	for i := range writers {
		_ = writers[i].Close()
		writers[i] = nil
	}

	// Build shard file info by hashing each completed shard file.
	// Skip when --no-manifest to avoid re-reading all shard data from disk.
	var shardFiles []ShardFileInfo
	if !opts.NoManifest {
		shardFiles = make([]ShardFileInfo, totalShards)
		for i := range totalShards {
			shardName := shardFilename(originalName, i)
			shardPath := filepath.Join(opts.OutputDir, shardName)

			fileHash, fileSize, err := HashFile(shardPath)
			if err != nil {
				return nil, fmt.Errorf("hashing shard %d: %w", i, err)
			}

			shardFiles[i] = ShardFileInfo{
				Index:    i,
				Type:     shardType(i, opts.DataShards),
				Filename: shardName,
				Path:     shardPath,
				Size:     fileSize,
				SHA256:   fileHash,
			}
		}
	}

	if showVerbose {
		for i := range totalShards {
			shardPath := filepath.Join(opts.OutputDir, shardFilename(originalName, i))
			fmt.Printf("  Created: %s\n", shardPath)
		}
		fmt.Println("Split complete.")
	}

	result = &SplitResult{
		OriginalName:   originalName,
		OriginalSHA256: fmt.Sprintf("%x", originalHash.Sum(nil)),
		OriginalSize:   originalSize,
		DataShards:     opts.DataShards,
		ParityShards:   opts.ParityShards,
		Encrypted:      encrypt,
		KeyFileUsed:    encrypt && opts.KeyFile != "",
		ShardFiles:     shardFiles,
	}
	if encrypt {
		result.ArgonTime = kdfParams.Time
		result.ArgonMemory = kdfParams.Memory
		result.ArgonParallelism = kdfParams.Parallelism
	}

	return result, nil
}

// SplitDryRunResult holds the computed metadata for a dry-run split.
type SplitDryRunResult struct {
	OriginalName     string
	OriginalSize     uint64
	DataShards       int
	ParityShards     int
	TotalShards      int
	PerShardPayload  uint64
	PerShardFileSize uint64
	TotalOutputSize  uint64
	Encrypted        bool
	KeyFileUsed      bool
	OutputDir        string
	ShardPaths       []string
	RelPath          string // Relative path from input root (batch mode only)
}

// DryRunSplit computes what a split would produce without writing any files.
func DryRunSplit(opts SplitOptions) (*SplitDryRunResult, error) {
	info, err := os.Stat(opts.InputFile)
	if err != nil {
		return nil, fmt.Errorf("stat input file: %w", err)
	}

	originalSize := uint64(info.Size())
	originalName := filepath.Base(opts.InputFile)
	totalShards := opts.DataShards + opts.ParityShards
	encrypted := !opts.NoEncrypt
	keyFileUsed := encrypted && opts.KeyFile != ""

	var perShardPayload uint64
	if originalSize > 0 {
		perShardPayload = (originalSize + uint64(opts.DataShards) - 1) / uint64(opts.DataShards)
	}
	perShardFileSize := shard.HeaderSize + perShardPayload + shard.TrailerSize
	totalOutputSize := perShardFileSize * uint64(totalShards)

	shardPaths := make([]string, totalShards)
	for i := range totalShards {
		shardPaths[i] = filepath.Join(opts.OutputDir, shardFilename(originalName, i))
	}

	return &SplitDryRunResult{
		OriginalName:     originalName,
		OriginalSize:     originalSize,
		DataShards:       opts.DataShards,
		ParityShards:     opts.ParityShards,
		TotalShards:      totalShards,
		PerShardPayload:  perShardPayload,
		PerShardFileSize: perShardFileSize,
		TotalOutputSize:  totalOutputSize,
		Encrypted:        encrypted,
		KeyFileUsed:      keyFileUsed,
		OutputDir:        opts.OutputDir,
		ShardPaths:       shardPaths,
	}, nil
}

func shardFilename(originalName string, index int) string {
	return fmt.Sprintf("%s.%03d.hrcx", originalName, index)
}

// shardType returns "data" for indices below dataShards, "parity" otherwise.
func shardType(index, dataShards int) string {
	if index < dataShards {
		return "data"
	}
	return "parity"
}

// HashFile computes the SHA-256 hash and size of an entire file.
func HashFile(path string) (string, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), uint64(n), nil
}
