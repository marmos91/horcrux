package pipeline

import (
	"testing"
)

func TestParseWeightedURIs(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []WeightedURI
		wantErr bool
	}{
		{
			name:  "no weights",
			input: []string{"s3://bucket", "file:///tmp/shards"},
			want: []WeightedURI{
				{URI: "s3://bucket", Weight: 1},
				{URI: "file:///tmp/shards", Weight: 1},
			},
		},
		{
			name:  "with weights",
			input: []string{"s3://bucket*3", "azure://container*2", "qr:///backup*1"},
			want: []WeightedURI{
				{URI: "s3://bucket", Weight: 3},
				{URI: "azure://container", Weight: 2},
				{URI: "qr:///backup", Weight: 1},
			},
		},
		{
			name:  "mixed",
			input: []string{"s3://bucket*3", "file:///tmp"},
			want: []WeightedURI{
				{URI: "s3://bucket", Weight: 3},
				{URI: "file:///tmp", Weight: 1},
			},
		},
		{
			name:    "zero weight",
			input:   []string{"s3://bucket*0"},
			wantErr: true,
		},
		{
			name:    "negative weight",
			input:   []string{"s3://bucket*-1"},
			wantErr: true,
		},
		{
			name:  "asterisk in path not a weight",
			input: []string{"s3://bucket/pre*fix"},
			want: []WeightedURI{
				{URI: "s3://bucket/pre*fix", Weight: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWeightedURIs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d URIs, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].URI != tt.want[i].URI {
					t.Errorf("URI[%d] = %q, want %q", i, got[i].URI, tt.want[i].URI)
				}
				if got[i].Weight != tt.want[i].Weight {
					t.Errorf("Weight[%d] = %d, want %d", i, got[i].Weight, tt.want[i].Weight)
				}
			}
		})
	}
}

func TestDeriveOriginalName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"secret.pdf.000.hrcx", "secret.pdf"},
		{"secret.pdf.003.hrcx", "secret.pdf"},
		{"file.txt.123.hrcx", "file.txt"},
		{"noext.hrcx", ""},           // no .NNN suffix
		{"secret.pdf.abc.hrcx", ""},  // NNN is not numeric
		{"secret.pdf.0000.hrcx", ""}, // 4-digit, not 3
		{"secret.pdf.000.txt", ""},   // not .hrcx
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := deriveOriginalName(tt.input)
			if got != tt.want {
				t.Errorf("deriveOriginalName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOpenWeightedBackendsExpansion(t *testing.T) {
	// Test that weights expand the backends slice correctly.
	// We can't actually open backends without real URIs, so test ParseWeightedURIs
	// and verify expansion logic.
	uris := []string{"s3://a*3", "s3://b*2"}
	weighted, err := ParseWeightedURIs(uris)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate expansion
	total := 0
	for _, w := range weighted {
		total += w.Weight
	}
	if total != 5 {
		t.Errorf("total weight = %d, want 5", total)
	}
}
