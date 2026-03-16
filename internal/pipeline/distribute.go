package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/config"
	"github.com/marmos91/horcrux/internal/manifest"
	"golang.org/x/sync/errgroup"
)

// DistributeShards uploads shard files to backends using round-robin assignment.
// Each shard's Location field is updated with the backend URI.
func DistributeShards(ctx context.Context, shardFiles []ShardFileInfo, backends []BackendWithURI) ([]ShardFileInfo, error) {
	if len(backends) == 0 {
		return shardFiles, nil
	}

	result := make([]ShardFileInfo, len(shardFiles))
	copy(result, shardFiles)

	g, ctx := errgroup.WithContext(ctx)

	for i := range result {
		bk := backends[i%len(backends)]
		sf := &result[i]

		g.Go(func() error {
			if err := bk.Backend.Upload(ctx, sf.Path, sf.Filename); err != nil {
				return fmt.Errorf("uploading shard %d to %s: %w", sf.Index, bk.URI, err)
			}
			sf.Location = bk.URI + "/" + sf.Filename
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

// CleanupLocalShards removes local shard files after successful distribution.
func CleanupLocalShards(shardFiles []ShardFileInfo) error {
	for _, sf := range shardFiles {
		if err := os.Remove(sf.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing local shard %s: %w", sf.Path, err)
		}
	}
	return nil
}

// BackendWithURI pairs a backend instance with its original URI for location tracking.
type BackendWithURI struct {
	Backend backend.Backend
	URI     string
}

// OpenBackends parses URIs and opens backend instances.
// Backend-specific options (credentials, env overrides, etc.) are always merged
// via NewFromConfig, even when cfg is nil, so that environment variables work.
func OpenBackends(uris []string, cfg *config.BackendConfig) ([]BackendWithURI, error) {
	backends := make([]BackendWithURI, 0, len(uris))
	for _, uri := range uris {
		b, err := backend.NewFromConfig(uri, cfg)
		if err != nil {
			return nil, fmt.Errorf("opening backend %s: %w", uri, err)
		}
		cleaned := strings.TrimRight(uri, "/")
		backends = append(backends, BackendWithURI{Backend: b, URI: cleaned})
	}
	return backends, nil
}

// WeightedURI associates a backend URI with a distribution weight.
type WeightedURI struct {
	URI    string
	Weight int
}

// ParseWeightedURIs parses URIs with optional *N weight suffixes.
// "s3://bucket*3" → {URI: "s3://bucket", Weight: 3}
// "s3://bucket"   → {URI: "s3://bucket", Weight: 1}
func ParseWeightedURIs(raw []string) ([]WeightedURI, error) {
	result := make([]WeightedURI, 0, len(raw))
	for _, entry := range raw {
		uri, weight := entry, 1

		// Split on last '*' — careful not to split scheme://host or query params
		if idx := strings.LastIndex(entry, "*"); idx > 0 {
			if n, err := strconv.Atoi(entry[idx+1:]); err == nil {
				uri = entry[:idx]
				weight = n
			}
		}

		if weight < 1 {
			return nil, fmt.Errorf("weight must be >= 1, got %d for %s", weight, uri)
		}

		result = append(result, WeightedURI{URI: uri, Weight: weight})
	}
	return result, nil
}

// OpenWeightedBackends parses weighted URIs and opens backend instances.
// Each backend is opened once, then expanded in the returned slice by its weight
// so that round-robin distribution naturally achieves proportional allocation.
func OpenWeightedBackends(raw []string, cfg *config.BackendConfig) ([]BackendWithURI, error) {
	weighted, err := ParseWeightedURIs(raw)
	if err != nil {
		return nil, err
	}

	var backends []BackendWithURI
	for _, w := range weighted {
		b, err := backend.NewFromConfig(w.URI, cfg)
		if err != nil {
			return nil, fmt.Errorf("opening backend %s: %w", w.URI, err)
		}
		cleaned := strings.TrimRight(w.URI, "/")
		entry := BackendWithURI{Backend: b, URI: cleaned}
		for range w.Weight {
			backends = append(backends, entry)
		}
	}
	return backends, nil
}

// DistributeManifest uploads a manifest file to each unique backend.
// Backends are deduplicated by URI since weighted expansion may repeat them.
func DistributeManifest(ctx context.Context, manifestPath string, backends []BackendWithURI) error {
	seen := make(map[string]bool)
	var unique []BackendWithURI
	for _, b := range backends {
		if !seen[b.URI] {
			seen[b.URI] = true
			unique = append(unique, b)
		}
	}

	manifestFilename := filepath.Base(manifestPath)
	g, ctx := errgroup.WithContext(ctx)

	for _, bk := range unique {
		g.Go(func() error {
			if err := bk.Backend.Upload(ctx, manifestPath, manifestFilename); err != nil {
				return fmt.Errorf("uploading manifest to %s: %w", bk.URI, err)
			}
			return nil
		})
	}

	return g.Wait()
}

// CollectManifest tries to download a manifest file from the given backend URIs.
// It derives the manifest filename from originalName and tries each backend in order,
// returning the first successfully downloaded and parsed manifest.
func CollectManifest(ctx context.Context, uris []string, originalName string, tempDir string, cfg *config.BackendConfig) (*manifest.Manifest, error) {
	manifestName := manifest.ManifestFilename(originalName)
	manifestPath := filepath.Join(tempDir, manifestName)

	for _, uri := range uris {
		b, err := backend.NewFromConfig(uri, cfg)
		if err != nil {
			continue
		}

		if err := b.Download(ctx, manifestName, manifestPath); err != nil {
			_ = os.Remove(manifestPath)
			continue
		}

		m, err := manifest.Load(manifestPath)
		if err != nil {
			_ = os.Remove(manifestPath)
			continue
		}

		return m, nil
	}

	return nil, fmt.Errorf("no manifest found on any backend for %s", originalName)
}

// DiscoverManifestOnBackends lists shard files on backends, derives the original
// filename from shard naming conventions, then tries to download the manifest.
// Returns nil, nil if no manifest is found (not an error).
func DiscoverManifestOnBackends(ctx context.Context, uris []string, tempDir string, cfg *config.BackendConfig) (*manifest.Manifest, error) {
	for _, uri := range uris {
		b, err := backend.NewFromConfig(uri, cfg)
		if err != nil {
			continue
		}

		files, err := b.List(ctx, "")
		if err != nil || len(files) == 0 {
			continue
		}

		// Derive original name from first shard: "file.txt.000.hrcx" → "file.txt"
		originalName := deriveOriginalName(filepath.Base(files[0].Key))
		if originalName == "" {
			continue
		}

		manifestName := manifest.ManifestFilename(originalName)
		localPath := filepath.Join(tempDir, manifestName)

		if err := b.Download(ctx, manifestName, localPath); err != nil {
			_ = os.Remove(localPath)
			continue
		}

		m, err := manifest.Load(localPath)
		if err != nil {
			_ = os.Remove(localPath)
			continue
		}

		return m, nil
	}

	return nil, nil
}

// deriveOriginalName extracts the original filename from a shard filename.
// "secret.pdf.003.hrcx" → "secret.pdf"
func deriveOriginalName(shardFilename string) string {
	name := strings.TrimSuffix(shardFilename, ".hrcx")
	if name == shardFilename {
		return ""
	}
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return ""
	}
	suffix := name[dot+1:]
	if len(suffix) != 3 {
		return ""
	}
	if _, err := strconv.Atoi(suffix); err != nil {
		return ""
	}
	return name[:dot]
}
