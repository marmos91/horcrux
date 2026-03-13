package pipeline

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/manifest"
	"golang.org/x/sync/errgroup"
)

// CollectFromManifest downloads shards listed in a manifest from their Location
// fields. Only shards with a non-empty Location are downloaded. Returns the
// temp directory containing the downloaded shard files.
func CollectFromManifest(ctx context.Context, m *manifest.Manifest, tempDir string) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, entry := range m.Shards {
		if entry.Location == "" {
			continue
		}

		localPath := filepath.Join(tempDir, entry.Filename)
		expectedSHA := entry.SHA256

		g.Go(func() error {
			b, remoteKey, err := openBackendForLocation(entry.Location)
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
// all of them to tempDir.
func CollectFromBackends(ctx context.Context, uris []string, tempDir string) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, uri := range uris {
		b, err := backend.Open(uri, nil)
		if err != nil {
			return fmt.Errorf("opening backend %s: %w", uri, err)
		}

		files, err := b.List(ctx, "")
		if err != nil {
			return fmt.Errorf("listing %s: %w", uri, err)
		}

		for _, f := range files {
			localPath := filepath.Join(tempDir, filepath.Base(f.Key))

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

// openBackendForLocation parses a shard location URI and returns a backend
// and the remote key to use for download.
//
// Location format: "s3://bucket/prefix/filename.hrcx"
// The remote key is the filename portion after the backend prefix.
func openBackendForLocation(location string) (backend.Backend, string, error) {
	scheme, bucket, uriPath, err := backend.ParseURI(location)
	if err != nil {
		return nil, "", err
	}

	// The remote key is the last path component (the shard filename).
	// Use path.Base (not filepath.Base) since URI paths always use forward slashes.
	remoteKey := path.Base(uriPath)
	prefix := strings.TrimSuffix(strings.TrimSuffix(uriPath, remoteKey), "/")

	var baseURI string
	switch {
	case bucket != "" && prefix != "":
		baseURI = fmt.Sprintf("%s://%s/%s", scheme, bucket, prefix)
	case bucket != "":
		baseURI = fmt.Sprintf("%s://%s", scheme, bucket)
	case prefix != "":
		baseURI = fmt.Sprintf("%s://%s", scheme, prefix)
	default:
		baseURI = fmt.Sprintf("%s:///", scheme)
	}

	b, err := backend.Open(baseURI, nil)
	if err != nil {
		return nil, "", err
	}

	return b, remoteKey, nil
}
