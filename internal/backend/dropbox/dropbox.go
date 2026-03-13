package dropbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/marmos91/horcrux/internal/backend"
)

func init() {
	backend.Register("dropbox", func(opts map[string]string) (backend.Backend, error) {
		token := opts["access-token"]
		if token == "" {
			return nil, fmt.Errorf("dropbox backend requires an access token (set DROPBOX_ACCESS_TOKEN or configure in config)")
		}
		return New(token, opts["prefix"]), nil
	})
}

// Dropbox implements backend.Backend using the Dropbox HTTP API.
type Dropbox struct {
	token  string
	prefix string
	client *http.Client
}

// New creates a Dropbox backend.
func New(token, prefix string) *Dropbox {
	return &Dropbox{
		token:  token,
		prefix: prefix,
		client: &http.Client{},
	}
}

func (d *Dropbox) remotePath(key string) string {
	return "/" + strings.Trim(d.prefix, "/") + "/" + key
}

func (d *Dropbox) Upload(ctx context.Context, localPath string, remoteKey string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	apiArg := map[string]any{
		"path":            d.remotePath(remoteKey),
		"mode":            "overwrite",
		"autorename":      false,
		"mute":            true,
		"strict_conflict": false,
	}
	argJSON, _ := json.Marshal(apiArg)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://content.dropboxapi.com/2/files/upload", f)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Dropbox-API-Arg", string(argJSON))

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("uploading to Dropbox: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Dropbox upload failed (status %d): %s", resp.StatusCode, body)
	}
	return nil
}

func (d *Dropbox) Download(ctx context.Context, remoteKey string, localPath string) error {
	apiArg := map[string]string{
		"path": d.remotePath(remoteKey),
	}
	argJSON, _ := json.Marshal(apiArg)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://content.dropboxapi.com/2/files/download", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Dropbox-API-Arg", string(argJSON))

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading from Dropbox: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Dropbox download failed (status %d): %s", resp.StatusCode, body)
	}

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

type listFolderResponse struct {
	Entries []listEntry `json:"entries"`
	HasMore bool        `json:"has_more"`
	Cursor  string      `json:"cursor"`
}

type listEntry struct {
	Tag  string `json:".tag"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (d *Dropbox) List(ctx context.Context, prefix string) ([]backend.RemoteFile, error) {
	listPath := d.prefix
	if prefix != "" {
		listPath = strings.TrimRight(listPath, "/") + "/" + prefix
	}
	if !strings.HasPrefix(listPath, "/") {
		listPath = "/" + listPath
	}
	listPath = strings.TrimRight(listPath, "/")

	body := map[string]any{
		"path":                                listPath,
		"recursive":                           false,
		"include_media_info":                  false,
		"include_deleted":                     false,
		"include_has_explicit_shared_members": false,
	}

	var files []backend.RemoteFile
	url := "https://api.dropboxapi.com/2/files/list_folder"

	for {
		bodyJSON, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+d.token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("listing Dropbox folder: %w", err)
		}

		var result listFolderResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("parsing Dropbox response: %w", err)
		}
		_ = resp.Body.Close()

		for _, entry := range result.Entries {
			if entry.Tag != "file" || !strings.HasSuffix(entry.Name, ".hrcx") {
				continue
			}
			files = append(files, backend.RemoteFile{
				Key:  entry.Name,
				Size: entry.Size,
			})
		}

		if !result.HasMore {
			break
		}
		url = "https://api.dropboxapi.com/2/files/list_folder/continue"
		body = map[string]any{"cursor": result.Cursor}
	}

	return files, nil
}

func (d *Dropbox) Delete(ctx context.Context, remoteKey string) error {
	body := map[string]string{
		"path": d.remotePath(remoteKey),
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dropboxapi.com/2/files/delete_v2", bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("deleting from Dropbox: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Dropbox delete failed (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

var _ backend.Backend = (*Dropbox)(nil)
