package wizard

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunSplitWizardDefaults(t *testing.T) {
	// Simulate user accepting all defaults (just pressing enter)
	// Flow: output dir -> data shards -> parity shards -> encrypt Y ->
	// password -> confirm password -> key file N -> distribute N -> proceed Y
	input := strings.Join([]string{
		".",        // output dir
		"5",        // data shards
		"3",        // parity shards
		"y",        // encrypt
		"testpass", // password
		"testpass", // confirm
		"n",        // key file
		"n",        // distribute
		"y",        // proceed
	}, "\n") + "\n"

	var out bytes.Buffer
	cfg, err := RunSplitWizard(strings.NewReader(input), &out, "test.pdf", 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.InputFile != "test.pdf" {
		t.Errorf("InputFile = %q, want test.pdf", cfg.InputFile)
	}
	if cfg.OutputDir != "." {
		t.Errorf("OutputDir = %q, want .", cfg.OutputDir)
	}
	if cfg.DataShards != 5 {
		t.Errorf("DataShards = %d, want 5", cfg.DataShards)
	}
	if cfg.ParityShards != 3 {
		t.Errorf("ParityShards = %d, want 3", cfg.ParityShards)
	}
	if !cfg.Encrypt {
		t.Error("expected Encrypt = true")
	}
	if cfg.Password != "testpass" {
		t.Errorf("Password = %q, want testpass", cfg.Password)
	}
	if len(cfg.DistributeURIs) != 0 {
		t.Errorf("expected no DistributeURIs, got %v", cfg.DistributeURIs)
	}
}

func TestRunSplitWizardNoEncrypt(t *testing.T) {
	input := strings.Join([]string{
		"./output", // output dir
		"3",        // data shards
		"2",        // parity shards
		"n",        // don't encrypt
		"n",        // don't distribute
		"y",        // proceed
	}, "\n") + "\n"

	var out bytes.Buffer
	cfg, err := RunSplitWizard(strings.NewReader(input), &out, "secret.bin", 2048)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Encrypt {
		t.Error("expected Encrypt = false")
	}
	if cfg.DataShards != 3 {
		t.Errorf("DataShards = %d, want 3", cfg.DataShards)
	}
	if cfg.ParityShards != 2 {
		t.Errorf("ParityShards = %d, want 2", cfg.ParityShards)
	}
}

func TestRunSplitWizardAbort(t *testing.T) {
	input := strings.Join([]string{
		".", // output dir
		"5", // data shards
		"3", // parity shards
		"n", // don't encrypt
		"n", // don't distribute
		"n", // don't proceed
	}, "\n") + "\n"

	var out bytes.Buffer
	_, err := RunSplitWizard(strings.NewReader(input), &out, "test.pdf", 1024)
	if err == nil {
		t.Fatal("expected abort error, got nil")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("expected abort error, got: %v", err)
	}
}

func TestRunSplitWizardWithDistribution(t *testing.T) {
	input := strings.Join([]string{
		".",           // output dir
		"5",           // data shards
		"3",           // parity shards
		"n",           // don't encrypt
		"y",           // distribute
		"1",           // select backend 1 (Local filesystem)
		"/tmp/shards", // path
		"",            // even distribution (no weighted prompt since only 1 backend)
		"y",           // distribute manifest
		"y",           // keep local
		"y",           // proceed
	}, "\n") + "\n"

	var out bytes.Buffer
	cfg, err := RunSplitWizard(strings.NewReader(input), &out, "test.pdf", 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.DistributeURIs) != 1 {
		t.Fatalf("expected 1 distribute URI, got %d", len(cfg.DistributeURIs))
	}
	if !strings.Contains(cfg.DistributeURIs[0], "file://") {
		t.Errorf("expected file:// URI, got %s", cfg.DistributeURIs[0])
	}
	if !cfg.DistributeManifest {
		t.Error("expected DistributeManifest = true")
	}
	if !cfg.KeepLocal {
		t.Error("expected KeepLocal = true")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
