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
	DataShards   *int    `yaml:"data-shards,omitempty"`
	ParityShards *int    `yaml:"parity-shards,omitempty"`
	Output       *string `yaml:"output,omitempty"`
	NoEncrypt    *bool   `yaml:"no-encrypt,omitempty"`
	Workers      *int    `yaml:"workers,omitempty"`
	FailFast     *bool   `yaml:"fail-fast,omitempty"`
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

	return nil
}

// DefaultConfig returns a Config with all fields populated with defaults.
func DefaultConfig() *Config {
	dataShards := 5
	parityShards := 3
	output := "."
	noEncrypt := false
	workers := runtime.NumCPU()
	failFast := false

	return &Config{
		DataShards:   &dataShards,
		ParityShards: &parityShards,
		Output:       &output,
		NoEncrypt:    &noEncrypt,
		Workers:      &workers,
		FailFast:     &failFast,
	}
}

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
