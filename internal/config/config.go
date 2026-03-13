package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config holds optional settings loaded from a YAML config file.
// Pointer fields distinguish "not set" (nil) from "set to zero/false/empty".
type Config struct {
	DataShards   *int           `yaml:"data-shards,omitempty"`
	ParityShards *int           `yaml:"parity-shards,omitempty"`
	Output       *string        `yaml:"output,omitempty"`
	NoEncrypt    *bool          `yaml:"no-encrypt,omitempty"`
	Workers      *int           `yaml:"workers,omitempty"`
	FailFast     *bool          `yaml:"fail-fast,omitempty"`
	NoManifest   *bool          `yaml:"no-manifest,omitempty"`
	Backends     *BackendConfig `yaml:"backends,omitempty"`
}

// BackendConfig holds configuration for cloud storage backends.
type BackendConfig struct {
	S3      *S3Config      `yaml:"s3,omitempty"`
	Azure   *AzureConfig   `yaml:"azure,omitempty"`
	Dropbox *DropboxConfig `yaml:"dropbox,omitempty"`
	GDrive  *GDriveConfig  `yaml:"gdrive,omitempty"`
	FTP     *FTPConfig     `yaml:"ftp,omitempty"`
}

// S3Config configures the S3 backend.
type S3Config struct {
	Region          *string `yaml:"region,omitempty"`
	AccessKeyID     *string `yaml:"access-key-id,omitempty"`
	SecretAccessKey *string `yaml:"secret-access-key,omitempty"`
	Endpoint        *string `yaml:"endpoint,omitempty"`
	ForcePathStyle  *bool   `yaml:"force-path-style,omitempty"`
}

// AzureConfig configures the Azure Blob backend.
type AzureConfig struct {
	AccountName      *string `yaml:"account-name,omitempty"`
	AccountKey       *string `yaml:"account-key,omitempty"`
	ConnectionString *string `yaml:"connection-string,omitempty"`
}

// DropboxConfig configures the Dropbox backend.
type DropboxConfig struct {
	AccessToken *string `yaml:"access-token,omitempty"`
}

// GDriveConfig configures the Google Drive backend.
type GDriveConfig struct {
	ServiceAccountJSON *string `yaml:"service-account-json,omitempty"`
	CredentialsFile    *string `yaml:"credentials-file,omitempty"`
}

// FTPConfig configures the FTP/FTPS backend.
type FTPConfig struct {
	Host     *string `yaml:"host,omitempty"`
	Port     *int    `yaml:"port,omitempty"`
	Username *string `yaml:"username,omitempty"`
	Password *string `yaml:"password,omitempty"`
	TLS      *bool   `yaml:"tls,omitempty"`
}

// searchPaths returns the ordered list of config file paths to check.
func searchPaths() []string {
	paths := []string{".hrcxrc"}

	home, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths,
			filepath.Join(home, ".config", "horcrux", "config.yaml"),
			filepath.Join(home, ".hrcxrc"),
		)
	}

	return paths
}

// FindConfigFile returns the path to the first config file found in
// the search order: ./.hrcxrc -> ~/.config/horcrux/config.yaml -> ~/.hrcxrc.
// Returns "" if no config file is found.
func FindConfigFile() string {
	for _, p := range searchPaths() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Load reads and parses a YAML config file at the given path.
// Unknown keys are rejected to catch typos early.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var cfg Config
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate checks that config values are within acceptable ranges.
func (c *Config) Validate() error {
	if c.DataShards != nil && (*c.DataShards < 1 || *c.DataShards > 254) {
		return fmt.Errorf("data-shards must be between 1 and 254, got %d", *c.DataShards)
	}
	if c.ParityShards != nil && (*c.ParityShards < 1 || *c.ParityShards > 254) {
		return fmt.Errorf("parity-shards must be between 1 and 254, got %d", *c.ParityShards)
	}

	// Check combined total if both are set
	if c.DataShards != nil && c.ParityShards != nil {
		if *c.DataShards+*c.ParityShards > 255 {
			return fmt.Errorf("total shards (data + parity) must be <= 255, got %d", *c.DataShards+*c.ParityShards)
		}
	}

	if c.Workers != nil && *c.Workers < 1 {
		return fmt.Errorf("workers must be >= 1, got %d", *c.Workers)
	}

	if c.Backends != nil {
		if err := c.Backends.Validate(); err != nil {
			return fmt.Errorf("backends: %w", err)
		}
	}

	return nil
}

// Validate checks that backend configuration values are reasonable.
func (b *BackendConfig) Validate() error {
	if b.FTP != nil && b.FTP.Port != nil {
		if *b.FTP.Port < 1 || *b.FTP.Port > 65535 {
			return fmt.Errorf("ftp port must be between 1 and 65535, got %d", *b.FTP.Port)
		}
	}
	return nil
}

// DefaultConfig returns a Config with all fields populated with defaults.
func DefaultConfig() *Config {
	workers := runtime.NumCPU()
	return &Config{
		DataShards:   ptr(5),
		ParityShards: ptr(3),
		Output:       ptr("."),
		NoEncrypt:    ptr(false),
		Workers:      &workers,
		FailFast:     ptr(false),
		NoManifest:   ptr(false),
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// DefaultConfigPath returns the default config file path (~/.config/horcrux/config.yaml).
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "horcrux", "config.yaml"), nil
}

// Marshal serializes the config to YAML bytes.
func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}
