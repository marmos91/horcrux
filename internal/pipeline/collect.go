package pipeline

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/config"
	"github.com/marmos91/horcrux/internal/manifest"
	"golang.org/x/sync/errgroup"
)

// CollectFromManifest downloads shards listed in a manifest from their Location
// fields into tempDir. Only shards with a non-empty Location are downloaded.
// Each downloaded shard is verified against its expected SHA-256 hash.
// Returns an error if no shards have a Location field.
func CollectFromManifest(ctx context.Context, m *manifest.Manifest, tempDir string, cfg *config.BackendConfig) error {
	// Check that at least one shard has a location
	hasLocation := false
	for _, entry := range m.Shards {
		if entry.Location != "" {
			hasLocation = true
			break
		}
	}
	if !hasLocation {
		return fmt.Errorf("no shards in manifest have a Location field; cannot collect from backends")
	}

	// Cache backends by base URI to avoid recreating clients per shard
	var mu sync.Mutex
	backendCache := make(map[string]backend.Backend)

	g, ctx := errgroup.WithContext(ctx)

	for _, entry := range m.Shards {
		if entry.Location == "" {
			continue
		}

		localPath := filepath.Join(tempDir, entry.Filename)
		expectedSHA := entry.SHA256

		g.Go(func() error {
			b, remoteKey, err := openBackendForLocationCached(entry.Location, cfg, &mu, backendCache)
			if err != nil {
				return fmt.Errorf("shard %d: %w", entry.Index, err)
			}

			if err := b.Download(ctx, remoteKey, localPath); err != nil {
				return fmt.Errorf("downloading shard %d from %s: %w", entry.Index, entry.Location, err)
			}

			// Verify SHA-256 after download
			hash, _, err := HashFile(localPath)
			if err != nil {
				return fmt.Errorf("hashing downloaded shard %d: %w", entry.Index, err)
			}
			if hash != expectedSHA {
				return fmt.Errorf("shard %d: SHA-256 mismatch after download (expected %s, got %s)",
					entry.Index, expectedSHA, hash)
			}

			return nil
		})
	}

	return g.Wait()
}

// CollectFromBackends lists .hrcx files on each backend URI and downloads
// all of them to tempDir. Detects filename collisions across backends.
// If cfg is non-nil, backend-specific options are merged from config.
func CollectFromBackends(ctx context.Context, uris []string, tempDir string, cfg *config.BackendConfig) error {
	g, ctx := errgroup.WithContext(ctx)

	// Track seen filenames to detect collisions
	var mu sync.Mutex
	seen := make(map[string]string) // base filename → source URI

	for _, uri := range uris {
		var b backend.Backend
		var err error
		if cfg != nil {
			b, err = backend.NewFromConfig(uri, cfg)
		} else {
			b, err = backend.Open(uri, nil)
		}
		if err != nil {
			return fmt.Errorf("opening backend %s: %w", uri, err)
		}

		files, err := b.List(ctx, "")
		if err != nil {
			return fmt.Errorf("listing %s: %w", uri, err)
		}

		for _, f := range files {
			baseName := filepath.Base(f.Key)

			mu.Lock()
			if existingURI, exists := seen[baseName]; exists {
				mu.Unlock()
				return fmt.Errorf("filename collision: %q found on both %s and %s", baseName, existingURI, uri)
			}
			seen[baseName] = uri
			mu.Unlock()

			localPath := filepath.Join(tempDir, baseName)

			g.Go(func() error {
				if err := b.Download(ctx, f.Key, localPath); err != nil {
					return fmt.Errorf("downloading %s from %s: %w", f.Key, uri, err)
				}
				return nil
			})
		}
	}

	return g.Wait()
}

// openBackendForLocationCached is like openBackendForLocation but caches
// backend instances by base URI to avoid recreating clients per shard.
func openBackendForLocationCached(location string, cfg *config.BackendConfig, mu *sync.Mutex, cache map[string]backend.Backend) (backend.Backend, string, error) {
	baseURI, remoteKey, err := parseLocationURI(location)
	if err != nil {
		return nil, "", err
	}

	mu.Lock()
	b, ok := cache[baseURI]
	mu.Unlock()

	if !ok {
		b, err = backend.NewFromConfig(baseURI, cfg)
		if err != nil {
			return nil, "", err
		}
		mu.Lock()
		cache[baseURI] = b
		mu.Unlock()
	}

	return b, remoteKey, nil
}

// parseLocationURI splits a shard location URI into a base URI (for opening a
// backend) and a remote key (the shard filename).
//
// Location format: "s3://bucket/prefix/filename.hrcx"
// Returns: ("s3://bucket/prefix", "filename.hrcx", nil)
func parseLocationURI(location string) (baseURI, remoteKey string, err error) {
	scheme, bucket, uriPath, err := backend.ParseURI(location)
	if err != nil {
		return "", "", err
	}

	// The remote key is the last path component (the shard filename).
	// Use path.Base (not filepath.Base) since URI paths always use forward slashes.
	remoteKey = path.Base(uriPath)
	if remoteKey == "" || remoteKey == "." || remoteKey == "/" {
		return "", "", fmt.Errorf("location %q does not contain a shard filename", location)
	}

	prefix := strings.TrimSuffix(strings.TrimSuffix(uriPath, remoteKey), "/")

	// Reconstruct the base URI without the filename.
	// For bucket-based schemes: scheme://bucket[/prefix]
	// For path-based schemes:   scheme:///path (prefix already has leading slash)
	baseURI = scheme + "://"
	switch {
	case bucket != "":
		baseURI += bucket
		if prefix != "" {
			baseURI += "/" + prefix
		}
	case strings.HasPrefix(prefix, "/"):
		// Path-based scheme (e.g. file:///tmp/shards): prefix already starts with /
		baseURI += prefix
	default:
		baseURI += "/" + prefix
	}

	return baseURI, remoteKey, nil
}
