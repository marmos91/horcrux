package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/marmos91/horcrux/internal/shard"
	"golang.org/x/sync/errgroup"
)

// FileResult holds the outcome of a single file operation in a batch.
type FileResult struct {
	File  string
	Error error
}

// SplitDirOptions configures a batch split of an entire directory.
type SplitDirOptions struct {
	InputDir     string
	OutputDir    string
	DataShards   int
	ParityShards int
	Password     string
	NoEncrypt    bool
	Verbose      bool
	Workers      int
	FailFast     bool
}

// MergeDirOptions configures a batch merge of multiple shard directories.
type MergeDirOptions struct {
	InputDir       string
	OutputDir      string
	Password       string
	Verbose        bool
	Workers        int
	FailFast       bool
	PromptPassword func() (string, error)
}

// SplitDir recursively splits all regular files in a directory tree.
// Output mirrors the input structure, with each file's shards placed in a
// subdirectory named after the file.
func SplitDir(opts SplitDirOptions) ([]FileResult, error) {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Collect all regular files
	var files []string
	err := filepath.WalkDir(opts.InputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking input directory: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in %s", opts.InputDir)
	}

	results := make([]FileResult, len(files))

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(workers)

	for i, file := range files {
		rel, err := filepath.Rel(opts.InputDir, file)
		if err != nil {
			return nil, fmt.Errorf("computing relative path: %w", err)
		}
		results[i].File = rel

		g.Go(func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Output: outputDir/rel/dir/filename/ (shards go inside)
			outSubDir := filepath.Join(opts.OutputDir, rel)
			splitErr := Split(SplitOptions{
				InputFile:    file,
				OutputDir:    outSubDir,
				DataShards:   opts.DataShards,
				ParityShards: opts.ParityShards,
				Password:     opts.Password,
				NoEncrypt:    opts.NoEncrypt,
				Verbose:      opts.Verbose,
			})
			results[i].Error = splitErr

			if splitErr != nil && opts.FailFast {
				return splitErr
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil && opts.FailFast {
		return results, err
	}

	return results, nil
}

// IsBatchMergeDir returns true if dir contains subdirectories with .hrcx files
// but no top-level .hrcx files — indicating a batch merge layout.
func IsBatchMergeDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	hasTopLevelHrcx := false
	hasSubdirs := false

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hrcx") {
			hasTopLevelHrcx = true
			break
		}
		if e.IsDir() {
			hasSubdirs = true
		}
	}

	return !hasTopLevelHrcx && hasSubdirs
}

// MergeDir performs a batch merge of multiple shard directories.
// It recursively finds leaf directories containing .hrcx files and merges each.
func MergeDir(opts MergeDirOptions) ([]FileResult, error) {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Resolve password once before dispatching workers
	password := opts.Password
	if password == "" && opts.PromptPassword != nil {
		// Check if any shard set is encrypted by peeking at the first one we find
		needsPassword, err := anyShardEncrypted(opts.InputDir)
		if err != nil {
			return nil, err
		}
		if needsPassword {
			password, err = opts.PromptPassword()
			if err != nil {
				return nil, fmt.Errorf("reading password: %w", err)
			}
		}
	}

	// Find all leaf directories containing .hrcx files
	shardDirs, err := findShardDirs(opts.InputDir)
	if err != nil {
		return nil, err
	}

	if len(shardDirs) == 0 {
		return nil, fmt.Errorf("no shard directories found in %s", opts.InputDir)
	}

	results := make([]FileResult, len(shardDirs))

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(workers)

	for i, shardDir := range shardDirs {
		rel, err := filepath.Rel(opts.InputDir, shardDir)
		if err != nil {
			return nil, fmt.Errorf("computing relative path: %w", err)
		}
		results[i].File = rel

		g.Go(func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Discover original filename from shards to build output path
			origName, nameErr := peekOriginalFilename(shardDir)
			if nameErr != nil {
				results[i].Error = nameErr
				if opts.FailFast {
					return nameErr
				}
				return nil
			}

			outDir := filepath.Join(opts.OutputDir, filepath.Dir(rel))
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				results[i].Error = err
				if opts.FailFast {
					return err
				}
				return nil
			}

			mergeErr := Merge(MergeOptions{
				ShardDir:   shardDir,
				OutputFile: filepath.Join(outDir, origName),
				Password:   password,
				Verbose:    opts.Verbose,
			})
			results[i].Error = mergeErr

			if mergeErr != nil && opts.FailFast {
				return mergeErr
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil && opts.FailFast {
		return results, err
	}

	return results, nil
}

// findShardDirs recursively discovers all directories containing .hrcx files.
func findShardDirs(root string) ([]string, error) {
	dirSet := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".hrcx") {
			dirSet[filepath.Dir(path)] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking shard directory: %w", err)
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	// Sort for deterministic output
	sortStrings(dirs)

	return dirs, nil
}

// anyShardEncrypted checks if any .hrcx file under root is encrypted.
func anyShardEncrypted(root string) (bool, error) {
	var found bool
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".hrcx") {
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			header, err := shard.ReadHeader(f)
			if err != nil {
				return nil
			}
			if header.IsEncrypted() {
				found = true
			}
		}
		return nil
	})
	return found, err
}

// peekOriginalFilename reads the first .hrcx file in a directory and returns
// the original filename from its header.
func peekOriginalFilename(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hrcx") {
			f, err := os.Open(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			header, err := shard.ReadHeader(f)
			f.Close()
			if err != nil {
				continue
			}
			return header.OriginalFilename, nil
		}
	}
	return "", fmt.Errorf("no valid .hrcx files in %s", dir)
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
