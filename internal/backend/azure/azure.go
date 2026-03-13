package azure

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/marmos91/horcrux/internal/backend"
)

func init() {
	backend.Register("azure", func(opts map[string]string) (backend.Backend, error) {
		container := opts["bucket"]
		if container == "" {
			return nil, fmt.Errorf("azure backend requires a container (e.g. azure://my-container/prefix)")
		}
		return New(container, opts["prefix"], opts)
	})
}

// Azure implements backend.Backend using Azure Blob Storage.
type Azure struct {
	client    *azblob.Client
	container string
	prefix    string
}

// New creates an Azure Blob backend.
func New(container, prefix string, opts map[string]string) (*Azure, error) {
	var client *azblob.Client
	var err error

	if connStr := opts["connection-string"]; connStr != "" {
		client, err = azblob.NewClientFromConnectionString(connStr, nil)
	} else {
		account := opts["account-name"]
		key := opts["account-key"]
		if account == "" || key == "" {
			return nil, fmt.Errorf("azure backend requires account-name and account-key (or connection-string)")
		}

		cred, credErr := azblob.NewSharedKeyCredential(account, key)
		if credErr != nil {
			return nil, fmt.Errorf("creating Azure credentials: %w", credErr)
		}

		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", account)
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("creating Azure client: %w", err)
	}

	return &Azure{
		client:    client,
		container: container,
		prefix:    prefix,
	}, nil
}

func (a *Azure) blobName(key string) string {
	if a.prefix != "" {
		return a.prefix + "/" + key
	}
	return key
}

func (a *Azure) Upload(ctx context.Context, localPath string, remoteKey string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = a.client.UploadFile(ctx, a.container, a.blobName(remoteKey), f, nil)
	if err != nil {
		return fmt.Errorf("uploading to Azure: %w", err)
	}
	return nil
}

func (a *Azure) Download(ctx context.Context, remoteKey string, localPath string) error {
	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer func() { _ = out.Close() }()

	_, err = a.client.DownloadFile(ctx, a.container, a.blobName(remoteKey), out, nil)
	if err != nil {
		return fmt.Errorf("downloading from Azure: %w", err)
	}
	return nil
}

func (a *Azure) List(ctx context.Context, prefix string) ([]backend.RemoteFile, error) {
	fullPrefix := a.prefix
	if prefix != "" {
		if fullPrefix != "" {
			fullPrefix += "/" + prefix
		} else {
			fullPrefix = prefix
		}
	}

	var files []backend.RemoteFile
	pager := a.client.NewListBlobsFlatPager(a.container, &azblob.ListBlobsFlatOptions{
		Prefix: &fullPrefix,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing Azure blobs: %w", err)
		}
		for _, blob := range page.Segment.BlobItems {
			name := *blob.Name
			if !strings.HasSuffix(name, ".hrcx") {
				continue
			}
			relKey := name
			if a.prefix != "" {
				relKey = strings.TrimPrefix(name, a.prefix+"/")
			}
			var size int64
			if blob.Properties.ContentLength != nil {
				size = *blob.Properties.ContentLength
			}
			files = append(files, backend.RemoteFile{
				Key:  relKey,
				Size: size,
			})
		}
	}

	return files, nil
}

func (a *Azure) Delete(ctx context.Context, remoteKey string) error {
	_, err := a.client.DeleteBlob(ctx, a.container, a.blobName(remoteKey), nil)
	if err != nil {
		return fmt.Errorf("deleting from Azure: %w", err)
	}
	return nil
}

var _ backend.Backend = (*Azure)(nil)
