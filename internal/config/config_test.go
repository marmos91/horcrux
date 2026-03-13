package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	content := `
data-shards: 10
parity-shards: 5
output: "/tmp/shards"
no-encrypt: true
workers: 4
fail-fast: true
`
	path := writeTempConfig(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertIntPtr(t, "DataShards", cfg.DataShards, 10)
	assertIntPtr(t, "ParityShards", cfg.ParityShards, 5)
	assertStringPtr(t, "Output", cfg.Output, "/tmp/shards")
	assertBoolPtr(t, "NoEncrypt", cfg.NoEncrypt, true)
	assertIntPtr(t, "Workers", cfg.Workers, 4)
	assertBoolPtr(t, "FailFast", cfg.FailFast, true)
}

func TestLoadPartialConfig(t *testing.T) {
	content := `data-shards: 7`

	path := writeTempConfig(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertIntPtr(t, "DataShards", cfg.DataShards, 7)
	if cfg.ParityShards != nil {
		t.Errorf("ParityShards should be nil, got %d", *cfg.ParityShards)
	}
	if cfg.Output != nil {
		t.Errorf("Output should be nil, got %q", *cfg.Output)
	}
	if cfg.NoEncrypt != nil {
		t.Errorf("NoEncrypt should be nil, got %v", *cfg.NoEncrypt)
	}
	if cfg.Workers != nil {
		t.Errorf("Workers should be nil, got %d", *cfg.Workers)
	}
	if cfg.FailFast != nil {
		t.Errorf("FailFast should be nil, got %v", *cfg.FailFast)
	}
}

func TestLoadEmptyConfig(t *testing.T) {
	path := writeTempConfig(t, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DataShards != nil {
		t.Errorf("DataShards should be nil")
	}
}

func TestValidate(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{"valid config", &Config{DataShards: intPtr(5), ParityShards: intPtr(3), Workers: intPtr(4)}, false},
		{"data-shards=0", &Config{DataShards: intPtr(0)}, true},
		{"parity-shards=-1", &Config{ParityShards: intPtr(-1)}, true},
		{"data-shards=255", &Config{DataShards: intPtr(255)}, true},
		{"total shards > 255", &Config{DataShards: intPtr(200), ParityShards: intPtr(100)}, true},
		{"workers=0", &Config{Workers: intPtr(0)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestFindConfigFile(t *testing.T) {
	// Create a temp dir and place .hrcxrc in it
	dir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("cannot chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("cannot restore working directory: %v", err)
		}
	})

	// No config file yet
	if got := FindConfigFile(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Create .hrcxrc in cwd
	if err := os.WriteFile(filepath.Join(dir, ".hrcxrc"), []byte("data-shards: 5\n"), 0o644); err != nil {
		t.Fatalf("cannot write .hrcxrc: %v", err)
	}

	if got := FindConfigFile(); got != ".hrcxrc" {
		t.Errorf("expected .hrcxrc, got %q", got)
	}
}

func TestDefaultConfigMarshalRoundtrip(t *testing.T) {
	cfg := DefaultConfig()

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	path := writeTempConfig(t, string(data))
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	assertIntPtr(t, "DataShards", loaded.DataShards, *cfg.DataShards)
	assertIntPtr(t, "ParityShards", loaded.ParityShards, *cfg.ParityShards)
	assertStringPtr(t, "Output", loaded.Output, *cfg.Output)
	assertBoolPtr(t, "NoEncrypt", loaded.NoEncrypt, *cfg.NoEncrypt)
	assertIntPtr(t, "Workers", loaded.Workers, *cfg.Workers)
	assertBoolPtr(t, "FailFast", loaded.FailFast, *cfg.FailFast)
}

func TestLoadBackendConfig(t *testing.T) {
	content := `
data-shards: 5
backends:
  s3:
    region: us-east-1
  azure:
    account-name: myaccount
  dropbox:
    access-token: my-token
  ftp:
    host: ftp.example.com
    port: 2121
    tls: true
`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Backends == nil {
		t.Fatal("Backends should not be nil")
	}
	if cfg.Backends.S3 == nil || *cfg.Backends.S3.Region != "us-east-1" {
		t.Errorf("S3.Region: expected us-east-1, got %v", cfg.Backends.S3)
	}
	if cfg.Backends.Azure == nil || *cfg.Backends.Azure.AccountName != "myaccount" {
		t.Errorf("Azure.AccountName: expected myaccount, got %v", cfg.Backends.Azure)
	}
	if cfg.Backends.Dropbox == nil || *cfg.Backends.Dropbox.AccessToken != "my-token" {
		t.Errorf("Dropbox.AccessToken: expected my-token, got %v", cfg.Backends.Dropbox)
	}
	if cfg.Backends.FTP == nil {
		t.Fatal("FTP should not be nil")
	}
	if *cfg.Backends.FTP.Host != "ftp.example.com" {
		t.Errorf("FTP.Host: expected ftp.example.com, got %q", *cfg.Backends.FTP.Host)
	}
	if *cfg.Backends.FTP.Port != 2121 {
		t.Errorf("FTP.Port: expected 2121, got %d", *cfg.Backends.FTP.Port)
	}
	if !*cfg.Backends.FTP.TLS {
		t.Error("FTP.TLS: expected true")
	}
}

func TestBackendConfigValidation(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name    string
		cfg     *BackendConfig
		wantErr bool
	}{
		{"nil config", nil, false},
		{"empty config", &BackendConfig{}, false},
		{"valid FTP port", &BackendConfig{FTP: &FTPConfig{Port: intPtr(21)}}, false},
		{"FTP port too low", &BackendConfig{FTP: &FTPConfig{Port: intPtr(0)}}, true},
		{"FTP port too high", &BackendConfig{FTP: &FTPConfig{Port: intPtr(70000)}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Backends: tt.cfg}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadUnknownKeys(t *testing.T) {
	path := writeTempConfig(t, "unknown-key: true\n")
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for unknown config key")
	}
}

func TestLoadInvalidValues(t *testing.T) {
	path := writeTempConfig(t, "data-shards: -5\n")
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid data-shards value")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	p, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(p) != "config.yaml" {
		t.Errorf("expected config.yaml, got %q", filepath.Base(p))
	}
	if filepath.Base(filepath.Dir(p)) != "horcrux" {
		t.Errorf("expected horcrux parent dir, got %q", filepath.Base(filepath.Dir(p)))
	}
}

// --- helpers ---

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write temp config: %v", err)
	}
	return path
}

func assertIntPtr(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: expected %d, got nil", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s: expected %d, got %d", name, want, *got)
	}
}

func assertStringPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: expected %q, got nil", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s: expected %q, got %q", name, want, *got)
	}
}

func assertBoolPtr(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: expected %v, got nil", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s: expected %v, got %v", name, want, *got)
	}
}
