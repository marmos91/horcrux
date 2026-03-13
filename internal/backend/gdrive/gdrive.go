package gdrive

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func init() {
	backend.Register("gdrive", func(opts map[string]string) (backend.Backend, error) {
		folderID := opts["bucket"]
		if folderID == "" {
			// Try prefix as folder ID for gdrive:///folder-id format
			folderID = strings.TrimPrefix(opts["prefix"], "/")
		}
		if folderID == "" {
			return nil, fmt.Errorf("gdrive backend requires a folder ID (e.g. gdrive://folder-id)")
		}
		return New(context.Background(), folderID, opts)
	})
}

// GDrive implements backend.Backend using Google Drive API v3.
type GDrive struct {
	service  *drive.Service
	folderID string
}

// New creates a Google Drive backend.
func New(ctx context.Context, folderID string, opts map[string]string) (*GDrive, error) {
	clientOpts := []option.ClientOption{option.WithScopes(drive.DriveFileScope)}

	if saJSON := opts["service-account-json"]; saJSON != "" {
		// Credentials come from user's own config — safe to use despite deprecation.
		clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(saJSON))) //nolint:staticcheck // SA1019: user-controlled credentials
	} else if credFile := opts["credentials-file"]; credFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(credFile)) //nolint:staticcheck // SA1019: user-controlled credentials
	}
	// Otherwise falls back to Application Default Credentials automatically

	service, err := drive.NewService(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating Drive service: %w", err)
	}

	return &GDrive{
		service:  service,
		folderID: folderID,
	}, nil
}

func (g *GDrive) Upload(ctx context.Context, localPath string, remoteKey string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Check if file already exists to update it
	existing, err := g.findFile(ctx, remoteKey)
	if err != nil {
		return err
	}

	if existing != "" {
		_, err = g.service.Files.Update(existing, &drive.File{
			Name: remoteKey,
		}).Media(f).Context(ctx).Do()
	} else {
		_, err = g.service.Files.Create(&drive.File{
			Name:    remoteKey,
			Parents: []string{g.folderID},
		}).Media(f).Context(ctx).Do()
	}
	if err != nil {
		return fmt.Errorf("uploading to Google Drive: %w", err)
	}
	return nil
}

func (g *GDrive) Download(ctx context.Context, remoteKey string, localPath string) error {
	fileID, err := g.findFile(ctx, remoteKey)
	if err != nil {
		return err
	}
	if fileID == "" {
		return fmt.Errorf("%w: %s", backend.ErrNotFound, remoteKey)
	}

	resp, err := g.service.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return fmt.Errorf("downloading from Google Drive: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing downloaded data: %w", err)
	}
	return out.Close()
}

func (g *GDrive) List(ctx context.Context, prefix string) ([]backend.RemoteFile, error) {
	query := fmt.Sprintf("'%s' in parents and trashed = false", escapeQuery(g.folderID))
	if prefix != "" {
		query += fmt.Sprintf(" and name contains '%s'", escapeQuery(prefix))
	}

	var files []backend.RemoteFile
	pageToken := ""

	for {
		call := g.service.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, size)").
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		result, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("listing Google Drive files: %w", err)
		}

		for _, f := range result.Files {
			if !strings.HasSuffix(f.Name, ".hrcx") {
				continue
			}
			files = append(files, backend.RemoteFile{
				Key:  f.Name,
				Size: f.Size,
			})
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return files, nil
}

func (g *GDrive) Delete(ctx context.Context, remoteKey string) error {
	fileID, err := g.findFile(ctx, remoteKey)
	if err != nil {
		return err
	}
	if fileID == "" {
		return fmt.Errorf("%w: %s", backend.ErrNotFound, remoteKey)
	}

	if err := g.service.Files.Delete(fileID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("deleting from Google Drive: %w", err)
	}
	return nil
}

// escapeQuery escapes single quotes in Google Drive query values.
func escapeQuery(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

func (g *GDrive) findFile(ctx context.Context, name string) (string, error) {
	query := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", escapeQuery(g.folderID), escapeQuery(name))
	result, err := g.service.Files.List().Q(query).Fields("files(id)").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("searching Google Drive: %w", err)
	}
	if len(result.Files) == 0 {
		return "", nil
	}
	return result.Files[0].Id, nil
}

var _ backend.Backend = (*GDrive)(nil)
