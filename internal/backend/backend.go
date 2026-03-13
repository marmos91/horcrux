package backend

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"

	"github.com/marmos91/horcrux/internal/config"
)

// Backend abstracts a remote (or local) storage provider for shard files.
type Backend interface {
	Upload(ctx context.Context, localPath string, remoteKey string) error
	Download(ctx context.Context, remoteKey string, localPath string) error
	List(ctx context.Context, prefix string) ([]RemoteFile, error)
	Delete(ctx context.Context, remoteKey string) error
}

// RemoteFile describes a file stored on a backend.
type RemoteFile struct {
	Key  string
	Size int64
}

// Constructor creates a Backend from an options map.
// The map keys and values are backend-specific (e.g. "region", "bucket").
type Constructor func(opts map[string]string) (Backend, error)

var (
	mu       sync.RWMutex
	registry = map[string]Constructor{}
)

// Register adds a backend constructor for the given URI scheme (e.g. "s3", "azure").
func Register(scheme string, ctor Constructor) {
	mu.Lock()
	defer mu.Unlock()
	registry[scheme] = ctor
}

// Get returns the constructor for a URI scheme, or an error if unregistered.
func Get(scheme string) (Constructor, error) {
	mu.RLock()
	defer mu.RUnlock()
	ctor, ok := registry[scheme]
	if !ok {
		return nil, fmt.Errorf("unknown backend scheme %q", scheme)
	}
	return ctor, nil
}

// ParseURI splits a backend URI into scheme, bucket, and path.
//
//	"s3://my-bucket/prefix/key" → ("s3", "my-bucket", "prefix/key")
//	"file:///tmp/shards"        → ("file", "", "/tmp/shards")
//	"dropbox:///folder/sub"     → ("dropbox", "", "/folder/sub")
func ParseURI(raw string) (scheme, bucket, path string, err error) {
	idx := strings.Index(raw, "://")
	if idx < 1 {
		return "", "", "", fmt.Errorf("invalid backend URI %q: missing scheme", raw)
	}
	scheme = raw[:idx]
	rest := raw[idx+3:]

	// For schemes with authority (bucket/container): scheme://bucket/path
	// For schemes without authority: scheme:///path (rest starts with "/")
	if strings.HasPrefix(rest, "/") {
		// No authority — rest is the full path (e.g. file:///tmp/x → "/tmp/x")
		return scheme, "", rest, nil
	}

	// Split on first "/"
	if before, after, found := strings.Cut(rest, "/"); found {
		bucket = before
		path = after
	} else {
		bucket = rest
	}
	return scheme, bucket, path, nil
}

// Open parses a URI, looks up the registered constructor, and creates a Backend.
// Extra options (e.g. from config) can be passed; the URI's bucket and path are
// injected as "bucket" and "prefix".
func Open(uri string, extraOpts map[string]string) (Backend, error) {
	scheme, bucket, path, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}

	ctor, err := Get(scheme)
	if err != nil {
		return nil, err
	}

	opts := make(map[string]string)
	maps.Copy(opts, extraOpts)
	opts["bucket"] = bucket
	opts["prefix"] = path

	return ctor(opts)
}

// NewFromConfig creates a Backend for the given URI, merging options from
// the config file. Environment variables override config values.
func NewFromConfig(uri string, cfg *config.BackendConfig) (Backend, error) {
	scheme, _, _, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}

	opts := configOptsForScheme(scheme, cfg)
	return Open(uri, opts)
}

// configOptsForScheme extracts backend-specific options from config,
// with environment variable overrides. Environment variables are always
// applied, even when cfg is nil, so env-only configuration works.
func configOptsForScheme(scheme string, cfg *config.BackendConfig) map[string]string {
	opts := make(map[string]string)

	switch scheme {
	case "s3":
		if cfg != nil && cfg.S3 != nil {
			setIfNotNil(opts, "region", cfg.S3.Region)
			setIfNotNil(opts, "access-key-id", cfg.S3.AccessKeyID)
			setIfNotNil(opts, "secret-access-key", cfg.S3.SecretAccessKey)
			setIfNotNil(opts, "endpoint", cfg.S3.Endpoint)
			if cfg.S3.ForcePathStyle != nil && *cfg.S3.ForcePathStyle {
				opts["force-path-style"] = "true"
			}
		}
		envOverride(opts, "region", "AWS_REGION")
		envOverride(opts, "access-key-id", "AWS_ACCESS_KEY_ID")
		envOverride(opts, "secret-access-key", "AWS_SECRET_ACCESS_KEY")
		envOverride(opts, "endpoint", "AWS_ENDPOINT_URL")

	case "azure":
		if cfg != nil && cfg.Azure != nil {
			setIfNotNil(opts, "account-name", cfg.Azure.AccountName)
			setIfNotNil(opts, "account-key", cfg.Azure.AccountKey)
			setIfNotNil(opts, "connection-string", cfg.Azure.ConnectionString)
		}
		envOverride(opts, "account-name", "AZURE_STORAGE_ACCOUNT")
		envOverride(opts, "account-key", "AZURE_STORAGE_KEY")
		envOverride(opts, "connection-string", "AZURE_STORAGE_CONNECTION_STRING")

	case "dropbox":
		if cfg != nil && cfg.Dropbox != nil {
			setIfNotNil(opts, "access-token", cfg.Dropbox.AccessToken)
		}
		envOverride(opts, "access-token", "DROPBOX_ACCESS_TOKEN")

	case "gdrive":
		if cfg != nil && cfg.GDrive != nil {
			setIfNotNil(opts, "service-account-json", cfg.GDrive.ServiceAccountJSON)
			setIfNotNil(opts, "credentials-file", cfg.GDrive.CredentialsFile)
		}
		envOverride(opts, "credentials-file", "GOOGLE_APPLICATION_CREDENTIALS")

	case "ftp":
		if cfg != nil && cfg.FTP != nil {
			setIfNotNil(opts, "host", cfg.FTP.Host)
			setIfNotNil(opts, "username", cfg.FTP.Username)
			setIfNotNil(opts, "password", cfg.FTP.Password)
			if cfg.FTP.Port != nil {
				opts["port"] = fmt.Sprintf("%d", *cfg.FTP.Port)
			}
			if cfg.FTP.TLS != nil && *cfg.FTP.TLS {
				opts["tls"] = "true"
			}
		}
		envOverride(opts, "username", "FTP_USERNAME")
		envOverride(opts, "password", "FTP_PASSWORD")
	}

	return opts
}

func setIfNotNil(opts map[string]string, key string, val *string) {
	if val != nil {
		opts[key] = *val
	}
}

func envOverride(opts map[string]string, key, envVar string) {
	if v := os.Getenv(envVar); v != "" {
		opts[key] = v
	}
}

// JoinPrefix combines a base prefix and a sub-prefix with a "/" separator.
// Returns the non-empty part if the other is empty.
func JoinPrefix(base, sub string) string {
	if sub == "" {
		return base
	}
	if base == "" {
		return sub
	}
	return base + "/" + sub
}
