package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
)

func init() {
	backend.Register("file", func(opts map[string]string) (backend.Backend, error) {
		root := opts["prefix"]
		if root == "" {
			root = opts["bucket"]
		}
		if root == "" {
			return nil, fmt.Errorf("file backend requires a path (e.g. file:///tmp/shards)")
		}
		return New(root), nil
	})
}

// Local implements backend.Backend using the local filesystem.
type Local struct {
	root string
}

// New creates a local filesystem backend rooted at the given directory.
func New(root string) *Local {
	return &Local{root: root}
}

// safePath validates that remoteKey does not escape the backend root via path
// traversal and returns the joined absolute path.
func (l *Local) safePath(remoteKey string) (string, error) {
	joined := filepath.Join(l.root, remoteKey)
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	absRoot, err := filepath.Abs(l.root)
	if err != nil {
		return "", fmt.Errorf("resolving root: %w", err)
	}
	// Ensure the resolved path is under root (with trailing separator to avoid prefix false positives)
	if !strings.HasPrefix(absJoined, absRoot+string(filepath.Separator)) && absJoined != absRoot {
		return "", fmt.Errorf("remote key %q escapes backend root", remoteKey)
	}
	return joined, nil
}

func (l *Local) Upload(_ context.Context, localPath string, remoteKey string) error {
	dst, err := l.safePath(remoteKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer func() { _ = src.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, src); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}
	return out.Close()
}

func (l *Local) Download(_ context.Context, remoteKey string, localPath string) error {
	src, err := l.safePath(remoteKey)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", backend.ErrNotFound, remoteKey)
		}
		return fmt.Errorf("opening source: %w", err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}
	return out.Close()
}

func (l *Local) List(_ context.Context, prefix string) ([]backend.RemoteFile, error) {
	searchDir := l.root
	if prefix != "" {
		searchDir = filepath.Join(l.root, prefix)
	}

	var files []backend.RemoteFile
	err := filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".hrcx") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(l.root, path)
		if err != nil {
			return err
		}

		files = append(files, backend.RemoteFile{
			Key:  rel,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing files: %w", err)
	}
	return files, nil
}

func (l *Local) Delete(_ context.Context, remoteKey string) error {
	target, err := l.safePath(remoteKey)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", backend.ErrNotFound, remoteKey)
		}
		return fmt.Errorf("deleting file: %w", err)
	}
	return nil
}

var _ backend.Backend = (*Local)(nil)
