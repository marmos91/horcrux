package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/config"
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
