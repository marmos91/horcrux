package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/marmos91/horcrux/internal/crypto"
	"github.com/marmos91/horcrux/internal/display"
	"github.com/marmos91/horcrux/internal/erasure"
	"github.com/marmos91/horcrux/internal/shard"
)

// SplitOptions configures the split operation.
type SplitOptions struct {
	InputFile    string
	OutputDir    string
	DataShards   int
	ParityShards int
	Password     string
	NoEncrypt    bool
	Verbose      bool
}

// Split splits a file into encrypted, erasure-coded shards.
func Split(opts SplitOptions) error {
	inputFile, err := os.Open(opts.InputFile)
	if err != nil {
		return fmt.Errorf("opening input file: %w", err)
	}
	defer func() { _ = inputFile.Close() }()

	inputInfo, err := inputFile.Stat()
	if err != nil {
		return fmt.Errorf("stat input file: %w", err)
	}
	originalSize := uint64(inputInfo.Size())
	originalName := filepath.Base(opts.InputFile)

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	var (
		key       []byte
		salt      [32]byte
		iv        [16]byte
		pwTag     [8]byte
		kdfParams crypto.KDFParams
	)

	encrypt := !opts.NoEncrypt
	if encrypt {
		kdfParams = crypto.DefaultKDFParams()
		salt, err = crypto.GenerateSalt()
		if err != nil {
			return fmt.Errorf("generating salt: %w", err)
		}
		iv, err = crypto.GenerateIV()
		if err != nil {
			return fmt.Errorf("generating IV: %w", err)
		}
		key = crypto.DeriveKey(opts.Password, salt, kdfParams)
		pwTag = crypto.PasswordTag(key)
	}

	totalShards := opts.DataShards + opts.ParityShards

	// RS Split distributes input evenly across N data shards, rounding up.
	// CTR mode produces no padding, so encrypted size equals original size.
	perShard := (int64(originalSize) + int64(opts.DataShards) - 1) / int64(opts.DataShards)

	writers := make([]*shard.Writer, totalShards)
	for i := 0; i < totalShards; i++ {
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
			for j := 0; j < i; j++ {
				_ = writers[j].Close()
			}
			return fmt.Errorf("creating shard %d: %w", i, err)
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

	if opts.Verbose {
		fmt.Printf("Splitting %s (%s) into %d+%d shards\n",
			originalName, display.FormatSize(originalSize), opts.DataShards, opts.ParityShards)
	}

	if originalSize == 0 {
		if opts.Verbose {
			fmt.Println("Empty file — writing headers only.")
		}
		for i, w := range writers {
			if err := w.WriteTrailer(); err != nil {
				return fmt.Errorf("writing trailer for shard %d: %w", i, err)
			}
		}
	} else {
		dataWriters := make([]io.Writer, opts.DataShards)
		for i := 0; i < opts.DataShards; i++ {
			dataWriters[i] = writers[i]
		}

		enc, err := erasure.NewEncoder(opts.DataShards, opts.ParityShards)
		if err != nil {
			return err
		}

		// Optionally encrypt, then split into data shards
		var inputReader io.Reader = inputFile
		if encrypt {
			inputReader, err = crypto.NewEncryptReader(inputFile, key, iv)
			if err != nil {
				return fmt.Errorf("creating encryption reader: %w", err)
			}
		}

		if opts.Verbose {
			fmt.Println("Writing data shards...")
		}

		if err := enc.Split(inputReader, dataWriters, int64(originalSize)); err != nil {
			return fmt.Errorf("splitting data: %w", err)
		}

		// Seek data shard files back to payload start for parity computation
		dataReaders := make([]io.Reader, opts.DataShards)
		for i := 0; i < opts.DataShards; i++ {
			f := writers[i].File()
			if _, err := f.Seek(shard.HeaderSize, io.SeekStart); err != nil {
				return fmt.Errorf("seeking data shard %d: %w", i, err)
			}
			dataReaders[i] = io.LimitReader(f, perShard)
		}

		parityWriters := make([]io.Writer, opts.ParityShards)
		for i := 0; i < opts.ParityShards; i++ {
			parityWriters[i] = writers[opts.DataShards+i]
		}

		if opts.Verbose {
			fmt.Println("Computing parity shards...")
		}

		if err := enc.Encode(dataReaders, parityWriters); err != nil {
			return fmt.Errorf("encoding parity: %w", err)
		}

		if opts.Verbose {
			fmt.Println("Writing checksums...")
		}

		for i, w := range writers {
			if err := w.WriteTrailer(); err != nil {
				return fmt.Errorf("writing trailer for shard %d: %w", i, err)
			}
		}
	}

	for i := range writers {
		_ = writers[i].Close()
		writers[i] = nil
	}

	if opts.Verbose {
		for i := 0; i < totalShards; i++ {
			shardPath := filepath.Join(opts.OutputDir, shardFilename(originalName, i))
			fmt.Printf("  Created: %s\n", shardPath)
		}
		fmt.Println("Split complete.")
	}

	return nil
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

	var perShardPayload uint64
	if originalSize > 0 {
		perShardPayload = (originalSize + uint64(opts.DataShards) - 1) / uint64(opts.DataShards)
	}
	perShardFileSize := shard.HeaderSize + perShardPayload + shard.TrailerSize
	totalOutputSize := perShardFileSize * uint64(totalShards)

	shardPaths := make([]string, totalShards)
	for i := 0; i < totalShards; i++ {
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
		OutputDir:        opts.OutputDir,
		ShardPaths:       shardPaths,
	}, nil
}

func shardFilename(originalName string, index int) string {
	return fmt.Sprintf("%s.%03d.hrcx", originalName, index)
}
