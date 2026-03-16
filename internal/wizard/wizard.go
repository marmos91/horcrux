// Package wizard provides an interactive CLI wizard for configuring split operations.
package wizard

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/marmos91/horcrux/internal/display"
	"golang.org/x/term"
)

// SplitConfig holds the configuration produced by the interactive wizard.
type SplitConfig struct {
	InputFile          string
	OutputDir          string
	DataShards         int
	ParityShards       int
	Encrypt            bool
	Password           string
	KeyFile            string
	DistributeURIs     []string // includes *N weights
	DistributeManifest bool
	KeepLocal          bool
}

// availableBackends lists the backends the wizard can configure.
var availableBackends = []struct {
	Name        string
	Scheme      string
	Description string
}{
	{"Local filesystem", "file", "Store shards on a local or mounted path"},
	{"Amazon S3", "s3", "Store shards in an S3 bucket"},
	{"Azure Blob Storage", "azure", "Store shards in an Azure container"},
	{"QR Code images", "qr", "Encode shards as QR code images"},
	{"Dropbox", "dropbox", "Store shards in Dropbox"},
	{"Google Drive", "gdrive", "Store shards in Google Drive"},
	{"FTP server", "ftp", "Store shards on an FTP server"},
}

// Prompter handles interactive prompting via a reader/writer pair.
type Prompter struct {
	scanner *bufio.Scanner
	out     io.Writer
	inFd    int // file descriptor for password reading, -1 if not a terminal
}

// NewPrompter creates a prompter. If in supports Fd() (e.g. os.Stdin),
// passwords will be read securely via terminal raw mode.
func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	p := &Prompter{
		scanner: bufio.NewScanner(in),
		out:     out,
		inFd:    -1,
	}
	if f, ok := in.(interface{ Fd() uintptr }); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			p.inFd = fd
		}
	}
	return p
}

func (p *Prompter) promptString(prompt, defaultVal string) string {
	if defaultVal != "" {
		_, _ = fmt.Fprintf(p.out, "%s [%s]: ", prompt, defaultVal)
	} else {
		_, _ = fmt.Fprintf(p.out, "%s: ", prompt)
	}
	if p.scanner.Scan() {
		text := strings.TrimSpace(p.scanner.Text())
		if text != "" {
			return text
		}
	}
	return defaultVal
}

func (p *Prompter) promptInt(prompt string, defaultVal int) int {
	s := p.promptString(prompt, strconv.Itoa(defaultVal))
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

func (p *Prompter) promptYesNo(prompt string, defaultYes bool) bool {
	hint := "Y/n"
	if !defaultYes {
		hint = "y/N"
	}
	_, _ = fmt.Fprintf(p.out, "%s [%s]: ", prompt, hint)
	if p.scanner.Scan() {
		text := strings.TrimSpace(strings.ToLower(p.scanner.Text()))
		switch text {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
	}
	return defaultYes
}

func (p *Prompter) promptPassword(prompt string) (string, error) {
	_, _ = fmt.Fprint(p.out, prompt)
	if p.inFd >= 0 {
		pw, err := term.ReadPassword(p.inFd)
		_, _ = fmt.Fprintln(p.out)
		return string(pw), err
	}
	// Fallback for non-terminal (e.g. tests with strings.Reader)
	if p.scanner.Scan() {
		return strings.TrimSpace(p.scanner.Text()), nil
	}
	return "", fmt.Errorf("failed to read password")
}

func (p *Prompter) promptSelect(prompt string, options []string, defaultIdx int) int {
	_, _ = fmt.Fprintln(p.out, prompt)
	for i, opt := range options {
		marker := "  "
		if i == defaultIdx {
			marker = "> "
		}
		_, _ = fmt.Fprintf(p.out, "  %s%d. %s\n", marker, i+1, opt)
	}
	s := p.promptString("Choice", strconv.Itoa(defaultIdx+1))
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > len(options) {
		return defaultIdx
	}
	return n - 1
}

func (p *Prompter) promptMultiSelect(prompt string, options []string) []int {
	_, _ = fmt.Fprintln(p.out, prompt)
	for i, opt := range options {
		_, _ = fmt.Fprintf(p.out, "  %d. %s\n", i+1, opt)
	}
	s := p.promptString("Enter numbers (comma-separated)", "")
	if s == "" {
		return nil
	}

	var selected []int
	seen := make(map[int]bool)
	for _, part := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 || n > len(options) {
			continue
		}
		idx := n - 1
		if !seen[idx] {
			seen[idx] = true
			selected = append(selected, idx)
		}
	}
	return selected
}

// RunSplitWizard guides the user through configuring a split operation.
func RunSplitWizard(in io.Reader, out io.Writer, inputFile string, fileSize int64) (*SplitConfig, error) {
	p := NewPrompter(in, out)
	cfg := &SplitConfig{
		InputFile: inputFile,
		KeepLocal: true,
	}

	_, _ = fmt.Fprintf(out, "\n=== Horcrux Split Wizard ===\n")
	_, _ = fmt.Fprintf(out, "File: %s (%s)\n\n", inputFile, formatSize(fileSize))

	cfg.OutputDir = p.promptString("Output directory", ".")

	_, _ = fmt.Fprintln(out, "\n--- Erasure Coding ---")
	cfg.DataShards = p.promptInt("Data shards", 5)
	if cfg.DataShards < 1 {
		cfg.DataShards = 5
	}
	cfg.ParityShards = p.promptInt("Parity shards", 3)
	if cfg.ParityShards < 1 {
		cfg.ParityShards = 3
	}
	total := cfg.DataShards + cfg.ParityShards
	_, _ = fmt.Fprintf(out, "  Total shards: %d (can tolerate %d lost)\n", total, cfg.ParityShards)

	_, _ = fmt.Fprintln(out, "\n--- Encryption ---")
	cfg.Encrypt = p.promptYesNo("Encrypt shards?", true)
	if cfg.Encrypt {
		pw, err := p.promptPassword("Enter encryption password: ")
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}
		confirm, err := p.promptPassword("Confirm password: ")
		if err != nil {
			return nil, fmt.Errorf("reading password confirmation: %w", err)
		}
		if pw != confirm {
			return nil, fmt.Errorf("passwords do not match")
		}
		cfg.Password = pw

		useKeyFile := p.promptYesNo("Use a key file?", false)
		if useKeyFile {
			cfg.KeyFile = p.promptString("Key file path", "")
		}
	}

	_, _ = fmt.Fprintln(out, "\n--- Distribution ---")
	distribute := p.promptYesNo("Distribute shards to backends?", false)
	if distribute {
		backendNames := make([]string, len(availableBackends))
		for i, b := range availableBackends {
			backendNames[i] = fmt.Sprintf("%s — %s", b.Name, b.Description)
		}

		selected := p.promptMultiSelect("Select backends:", backendNames)
		if len(selected) == 0 {
			_, _ = fmt.Fprintln(out, "  No backends selected, skipping distribution.")
		} else {
			type backendEntry struct {
				uri    string
				weight int
			}
			var entries []backendEntry

			for _, idx := range selected {
				b := availableBackends[idx]
				_, _ = fmt.Fprintf(out, "\n  Configuring %s (%s://):\n", b.Name, b.Scheme)

				var uri string
				switch b.Scheme {
				case "file", "qr":
					path := p.promptString("    Path", "")
					if path == "" {
						continue
					}
					uri = b.Scheme + "://" + path
				case "s3":
					bucket := p.promptString("    Bucket", "")
					prefix := p.promptString("    Prefix (optional)", "")
					if bucket == "" {
						continue
					}
					uri = "s3://" + bucket
					if prefix != "" {
						uri += "/" + prefix
					}
				case "azure":
					container := p.promptString("    Container", "")
					prefix := p.promptString("    Prefix (optional)", "")
					if container == "" {
						continue
					}
					uri = "azure://" + container
					if prefix != "" {
						uri += "/" + prefix
					}
				default:
					path := p.promptString("    Path/folder", "")
					if path == "" {
						continue
					}
					uri = b.Scheme + ":///" + path
				}
				entries = append(entries, backendEntry{uri: uri, weight: 1})
			}

			if len(entries) > 1 {
				_, _ = fmt.Fprintln(out, "\n--- Distribution Strategy ---")
				strategyOpts := []string{"Even distribution (default)", "Weighted distribution"}
				strategy := p.promptSelect("How should shards be distributed?", strategyOpts, 0)

				if strategy == 1 {
					for i := range entries {
						w := p.promptInt(fmt.Sprintf("  Weight for %s", entries[i].uri), 1)
						if w < 1 {
							w = 1
						}
						entries[i].weight = w
					}
				}
			}

			for _, e := range entries {
				uriWithWeight := e.uri
				if e.weight > 1 {
					uriWithWeight += fmt.Sprintf("*%d", e.weight)
				}
				cfg.DistributeURIs = append(cfg.DistributeURIs, uriWithWeight)
			}

			cfg.DistributeManifest = p.promptYesNo("Distribute manifest to backends?", true)
			cfg.KeepLocal = p.promptYesNo("Keep local copies of shards?", true)
		}
	}

	_, _ = fmt.Fprintln(out, "\n=== Summary ===")
	_, _ = fmt.Fprintf(out, "  Input:        %s\n", cfg.InputFile)
	_, _ = fmt.Fprintf(out, "  Output:       %s\n", cfg.OutputDir)
	_, _ = fmt.Fprintf(out, "  Shards:       %d data + %d parity\n", cfg.DataShards, cfg.ParityShards)
	_, _ = fmt.Fprintf(out, "  Encrypted:    %v\n", cfg.Encrypt)
	if len(cfg.DistributeURIs) > 0 {
		_, _ = fmt.Fprintf(out, "  Distribute:   %s\n", strings.Join(cfg.DistributeURIs, ", "))
		_, _ = fmt.Fprintf(out, "  Dist manifest: %v\n", cfg.DistributeManifest)
		_, _ = fmt.Fprintf(out, "  Keep local:   %v\n", cfg.KeepLocal)
	}

	proceed := p.promptYesNo("\nProceed?", true)
	if !proceed {
		return nil, fmt.Errorf("aborted by user")
	}

	return cfg, nil
}

func formatSize(bytes int64) string {
	return display.FormatSize(uint64(bytes))
}
