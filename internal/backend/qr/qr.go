// Package qr implements a backend.Backend that stores shards as QR code images.
package qr

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
	"github.com/marmos91/horcrux/internal/qr"
	"github.com/marmos91/horcrux/internal/shard"
)

func init() {
	backend.Register("qr", func(opts map[string]string) (backend.Backend, error) {
		root := opts["prefix"]
		if root == "" {
			root = opts["bucket"]
		}
		if root == "" {
			return nil, fmt.Errorf("qr backend requires a path (e.g. qr:///tmp/qrcodes)")
		}
		format := opts["format"]
		if format == "" {
			format = "png"
		}
		if format != "png" && format != "svg" {
			return nil, fmt.Errorf("qr backend: unsupported format %q (use png or svg)", format)
		}
		return &QR{root: root, format: format}, nil
	})
}

// QR implements backend.Backend by encoding shards as QR code images.
type QR struct {
	root   string
	format string
}

func (q *QR) safePath(remoteKey string) (string, error) {
	joined := filepath.Join(q.root, remoteKey)
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	absRoot, err := filepath.Abs(q.root)
	if err != nil {
		return "", fmt.Errorf("resolving root: %w", err)
	}
	if !strings.HasPrefix(absJoined, absRoot+string(filepath.Separator)) && absJoined != absRoot {
		return "", fmt.Errorf("remote key %q escapes backend root", remoteKey)
	}
	return joined, nil
}

// imageFilename converts a shard key like "file.txt.000.hrcx" to "file.txt.000.png".
func (q *QR) imageFilename(remoteKey string) string {
	base := strings.TrimSuffix(remoteKey, ".hrcx")
	return base + "." + q.format
}

func (q *QR) Upload(_ context.Context, localPath string, remoteKey string) error {
	if err := os.MkdirAll(q.root, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := qr.CheckShardFits(localPath); err != nil {
		return fmt.Errorf("qr backend: %w", err)
	}

	label, err := readShardLabel(localPath, remoteKey)
	if err != nil {
		return err
	}

	outputName := q.imageFilename(remoteKey)
	outputPath, err := q.safePath(outputName)
	if err != nil {
		return err
	}

	if q.format == "svg" {
		return qr.WriteSVG(localPath, outputPath, label)
	}

	img, err := qr.EncodeShard(localPath, qr.DefaultQRSize)
	if err != nil {
		return fmt.Errorf("encoding shard to QR: %w", err)
	}

	annotated := qr.Annotate(img, label)

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := png.Encode(f, annotated); err != nil {
		return fmt.Errorf("writing PNG: %w", err)
	}
	return f.Close()
}

func (q *QR) Download(_ context.Context, remoteKey string, localPath string) error {
	imagePath, err := q.findImage(remoteKey)
	if err != nil {
		return err
	}

	data, err := qr.DecodeShard(imagePath)
	if err != nil {
		return fmt.Errorf("decoding QR from %s: %w", imagePath, err)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return fmt.Errorf("writing shard file: %w", err)
	}

	return nil
}

func (q *QR) List(_ context.Context, prefix string) ([]backend.RemoteFile, error) {
	searchDir := q.root
	if prefix != "" {
		searchDir = filepath.Join(q.root, prefix)
	}

	var files []backend.RemoteFile
	imageExts := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".svg": true}

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

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !imageExts[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		baseName := strings.TrimSuffix(d.Name(), ext) + ".hrcx"

		files = append(files, backend.RemoteFile{
			Key:  baseName,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing QR images: %w", err)
	}
	return files, nil
}

func (q *QR) Delete(_ context.Context, remoteKey string) error {
	imagePath, err := q.findImage(remoteKey)
	if err != nil {
		return err
	}
	if err := os.Remove(imagePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", backend.ErrNotFound, remoteKey)
		}
		return fmt.Errorf("deleting file: %w", err)
	}
	return nil
}

// findImage locates the image file for a given shard key by trying known extensions.
func (q *QR) findImage(remoteKey string) (string, error) {
	base := strings.TrimSuffix(remoteKey, ".hrcx")
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".svg"} {
		candidate := base + ext
		p, err := q.safePath(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: no QR image found for %s", backend.ErrNotFound, remoteKey)
}

// readShardLabel reads the shard header to build annotation metadata.
func readShardLabel(shardPath, remoteKey string) (qr.ShardLabel, error) {
	f, err := os.Open(shardPath)
	if err != nil {
		return qr.ShardLabel{}, fmt.Errorf("opening shard for header: %w", err)
	}
	defer func() { _ = f.Close() }()

	hdr, err := shard.ReadHeader(f)
	if err != nil {
		// If header can't be read, use basic info
		return qr.ShardLabel{
			Filename:   remoteKey,
			ShardIndex: 0,
			ShardTotal: 1,
			ShardType:  "unknown",
		}, nil
	}

	shardType := "data"
	if hdr.ShardIndex >= hdr.DataShards {
		shardType = "parity"
	}

	return qr.ShardLabel{
		Filename:   hdr.OriginalFilename,
		ShardIndex: int(hdr.ShardIndex),
		ShardTotal: int(hdr.DataShards) + int(hdr.ParityShards),
		ShardType:  shardType,
	}, nil
}

var _ backend.Backend = (*QR)(nil)
