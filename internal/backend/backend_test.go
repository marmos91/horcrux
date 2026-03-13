package backend_test

import (
	"testing"

	"github.com/marmos91/horcrux/internal/backend"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		scheme  string
		bucket  string
		path    string
		wantErr bool
	}{
		{
			name:   "s3 with bucket and prefix",
			raw:    "s3://my-bucket/prefix/key",
			scheme: "s3",
			bucket: "my-bucket",
			path:   "prefix/key",
		},
		{
			name:   "s3 with bucket only",
			raw:    "s3://my-bucket",
			scheme: "s3",
			bucket: "my-bucket",
			path:   "",
		},
		{
			name:   "file with absolute path",
			raw:    "file:///tmp/shards",
			scheme: "file",
			bucket: "",
			path:   "/tmp/shards",
		},
		{
			name:   "azure with container and prefix",
			raw:    "azure://my-container/shards",
			scheme: "azure",
			bucket: "my-container",
			path:   "shards",
		},
		{
			name:   "dropbox with path",
			raw:    "dropbox:///folder/sub",
			scheme: "dropbox",
			bucket: "",
			path:   "/folder/sub",
		},
		{
			name:    "missing scheme",
			raw:     "no-scheme",
			wantErr: true,
		},
		{
			name:    "empty scheme",
			raw:     "://bucket",
			wantErr: true,
		},
		{
			name:   "ftp with host and port in authority",
			raw:    "ftp://example.com:2121/uploads/shards",
			scheme: "ftp",
			bucket: "example.com:2121",
			path:   "uploads/shards",
		},
		{
			name:   "ftp with host only",
			raw:    "ftp://example.com/shards",
			scheme: "ftp",
			bucket: "example.com",
			path:   "shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, bucket, path, err := backend.ParseURI(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if scheme != tt.scheme {
				t.Errorf("scheme = %q, want %q", scheme, tt.scheme)
			}
			if bucket != tt.bucket {
				t.Errorf("bucket = %q, want %q", bucket, tt.bucket)
			}
			if path != tt.path {
				t.Errorf("path = %q, want %q", path, tt.path)
			}
		})
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	backend.Register("test-scheme", func(opts map[string]string) (backend.Backend, error) {
		return nil, nil
	})

	ctor, err := backend.Get("test-scheme")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if ctor == nil {
		t.Fatal("constructor is nil")
	}

	_, err = backend.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent scheme")
	}
}
