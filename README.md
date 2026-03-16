# Horcrux (`hrcx`)

[![CI](https://github.com/marmos91/horcrux/actions/workflows/ci.yml/badge.svg)](https://github.com/marmos91/horcrux/actions/workflows/ci.yml)
[![Release](https://github.com/marmos91/horcrux/actions/workflows/release.yml/badge.svg)](https://github.com/marmos91/horcrux/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/marmos91/horcrux)](https://goreportcard.com/report/github.com/marmos91/horcrux)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A cross-platform CLI tool that splits files into encrypted, erasure-coded shards and reconstructs the original from a subset of those shards. With N data shards and K parity shards, you can lose up to K shards and still recover.

## Features

- **Erasure coding** via Reed-Solomon (powered by [klauspost/reedsolomon](https://github.com/klauspost/reedsolomon))
- **AES-256-CTR encryption** with Argon2id key derivation
- **Key file support** -- use a key file alone or combined with a password for two-factor encryption
- **Fast credential verification** via HMAC tag (detects wrong password or key file without processing the entire file)
- **Cloud backend integration** -- distribute shards to S3, Azure Blob, Google Drive, Dropbox, or FTP
- **Weighted distribution** -- proportional shard allocation across backends with `*N` syntax
- **Interactive wizard** -- guided split configuration via `-i` flag
- **Manifest distribution** -- auto-distribute and auto-discover manifests on backends
- **QR code export/import** -- export small shards as QR codes for paper backup
- **Shard verification** -- check integrity and recoverability without decryption
- **Streaming pipeline** with constant memory usage (~15 MB regardless of file size)
- **Corruption tolerance** -- corrupt shards are automatically detected and excluded during reconstruction
- **Batch processing** -- split or merge entire directories recursively
- **Config file support** -- set defaults via `.hrcxrc` or `~/.config/horcrux/config.yaml`
- **Single binary**, no dependencies at runtime

## Installation

### From source

```bash
go install github.com/marmos91/horcrux@latest
```

### Pre-built binaries

Download from the [Releases](https://github.com/marmos91/horcrux/releases) page. Binaries are available for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64).

### Build from repo

```bash
git clone https://github.com/marmos91/horcrux.git
cd horcrux
make build
```

The binary is named `hrcx`.

## Usage

### Split a file

```bash
# Encrypted (prompted for password)
hrcx split secret.pdf

# With explicit password
hrcx split -p "my-password" secret.pdf

# Custom shard counts (4 data + 2 parity)
hrcx split -n 4 -k 2 -o ./shards/ secret.pdf

# Without encryption
hrcx split --no-encrypt large-video.mp4

# Verbose output
hrcx split -v -p "my-password" secret.pdf
```

### Interactive mode

Use the `-i` flag for a guided wizard that walks you through split configuration:

```bash
hrcx split -i secret.pdf
```

The wizard prompts for shard counts, encryption, output directory, backend distribution, and weights -- ideal when you're not sure which flags to use.

Shards are named `<filename>.<index>.hrcx`. With the default 5 data + 3 parity shards:

```
secret.pdf.000.hrcx
secret.pdf.001.hrcx
secret.pdf.002.hrcx
...
secret.pdf.007.hrcx
```

### Merge (reconstruct) a file

```bash
# From a directory of shards (prompted for password if encrypted)
hrcx merge ./shards/

# With explicit password and output path
hrcx merge -p "my-password" -o recovered.pdf ./shards/

# Validate against manifest before merging
hrcx merge --manifest ./shards/manifest.json ./shards/
```

Merge tolerates up to K missing or corrupt shards. If any data shards are missing, they are automatically reconstructed from the available parity shards.

### Key file encryption

Use a key file alone or combined with a password for two-factor encryption.

```bash
# Generate a key file
hrcx keygen -o secret.key

# Split with key file only (no password)
hrcx split --key-file secret.key document.pdf

# Split with key file + password (two-factor)
hrcx split --key-file secret.key -p "my-password" document.pdf

# Merge with key file
hrcx merge --key-file secret.key ./shards/

# Merge with key file + password
hrcx merge --key-file secret.key -p "my-password" ./shards/
```

The key file can be any file (1 byte to 1 MB). Its SHA-256 hash is used as key material. When combined with a password, both are required to decrypt.

### QR code export/import

Export small shards as QR codes for paper backup. Each QR code supports up to ~2.2 KB of shard data, so use high data-shard counts to keep individual shards small.

```bash
# Export shards as QR codes (PNG)
hrcx export-qr ./shards/

# Export as SVG
hrcx export-qr -f svg -o ./qrcodes/ ./shards/

# Import shards from QR code images
hrcx import-qr ./qrcodes/

# Import a single QR image
hrcx import-qr photo.png
```

### Cloud backend distribution

Distribute shards to remote storage backends during split, or collect them before merge.

```bash
# Split and distribute to S3
hrcx split --distribute s3://my-bucket/shards secret.pdf

# Split and distribute to multiple backends
hrcx split --distribute s3://bucket/shards,azure://container/shards secret.pdf

# Weighted distribution (3 shards to S3, 2 to Azure)
hrcx split --distribute "s3://bucket/shards*3,azure://container/shards*2" secret.pdf

# Distribute the manifest alongside shards
hrcx split --distribute s3://bucket/shards --distribute-manifest secret.pdf

# Collect from backends and merge
hrcx merge --collect s3://my-bucket/shards

# Collect using a manifest (downloads only needed shards)
hrcx merge --collect s3://my-bucket/shards --manifest manifest.json

# Auto-discover manifest on backends (no --manifest needed)
hrcx merge --collect s3://bucket/shards,azure://container/shards
```

When `--distribute-manifest` is used, the manifest is uploaded to every backend. During `--collect`, if a manifest is found on any backend it is automatically used for guided collection and output verification -- no `--manifest` flag required.

**Supported backends:**

| Backend | URI scheme | Authentication |
|---------|-----------|---------------|
| AWS S3 | `s3://bucket/prefix` | AWS credentials (env vars or config) |
| Azure Blob | `azure://container/prefix` | Account key or connection string |
| Google Drive | `gdrive:///folder/path` | Service account JSON or credentials file |
| Dropbox | `dropbox:///folder/path` | Access token |
| FTP/FTPS | `ftp://host/path` | Username/password |
| QR code | `qr:///path` | Filesystem access |
| Local | `file:///absolute/path` | Filesystem access |

Backend credentials can be configured via environment variables or the config file (see [Configuration](#configuration-file)).

### Verify shards

```bash
# Check integrity and recoverability without decryption
hrcx verify ./shards/

# Verbose output (per-shard details)
hrcx verify -v ./shards/
```

### Inspect shard metadata

```bash
# Single shard
hrcx inspect shards/secret.pdf.003.hrcx

# All shards in a directory
hrcx inspect ./shards/
```

Output:

```
Shard: secret.pdf.003.hrcx
├── Format version:    1
├── Shard index:       3 / 8 (data shard)
├── Data shards:       5
├── Parity shards:     3
├── Original filename: secret.pdf
├── Original filesize: 15.0 MB
├── Encrypted:         yes (password)
└── Header checksum:   OK
```

### Dry-run mode

Preview what would happen without writing any files.

```bash
hrcx split --dry-run secret.pdf
hrcx merge --dry-run ./shards/
```

### Batch processing

Split or merge entire directories recursively.

```bash
# Split all files in a directory
hrcx split ./documents/

# Merge all shard sets in a directory
hrcx merge ./shards/

# Control parallelism and error handling
hrcx split -w 4 --fail-fast ./documents/
```

### Manifest files

A manifest JSON file is generated alongside shards during split. It records file hashes for integrity verification.

```bash
# Annotate shard locations in a manifest
hrcx manifest annotate manifest.json 0 "USB drive A"
hrcx manifest annotate manifest.json 1 "office safe"

# Distribute the manifest to all backends during split
hrcx split --distribute s3://bucket/shards --distribute-manifest secret.pdf

# Auto-discovery: collect finds the manifest on backends automatically
hrcx merge --collect s3://bucket/shards
```

When `--distribute-manifest` is used, the manifest is uploaded to every distribution backend. During `--collect`, the manifest is automatically discovered on backends and used for guided collection with SHA-256 output verification.

### Configuration file

Horcrux supports an optional YAML config file to set default values for CLI flags.

**Precedence** (highest to lowest): CLI flags > config file > built-in defaults.

**Config search order** (first found wins):
1. `./.hrcxrc` (current directory)
2. `~/.config/horcrux/config.yaml`
3. `~/.hrcxrc`

```bash
# Create a default config file at ~/.config/horcrux/config.yaml
hrcx config init

# Overwrite an existing config file
hrcx config init --force

# Display the active configuration and its source
hrcx config show
```

Example config file (`.hrcxrc` or `config.yaml`):

```yaml
data-shards: 10
parity-shards: 4
output: "./shards"
key-file: "~/.config/horcrux/default.key"
no-encrypt: false
workers: 8
fail-fast: true
no-manifest: false

backends:
  s3:
    region: "us-east-1"
    # access-key-id and secret-access-key can also use AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars
  azure:
    account-name: "myaccount"
    # account-key can also use AZURE_STORAGE_KEY env var
  dropbox:
    # access-token can also use DROPBOX_ACCESS_TOKEN env var
  gdrive:
    credentials-file: "/path/to/service-account.json"
  ftp:
    host: "ftp.example.com"
    port: 21
    tls: true
```

All fields are optional -- only the settings you include will override the defaults.

## How it works

### Split pipeline

```
Input file
  -> AES-256-CTR encrypt (if enabled)
  -> Reed-Solomon split into N data shards
  -> Reed-Solomon encode K parity shards
  -> Write shard files (header + payload + checksum trailer)
```

### Merge pipeline

```
Discover .hrcx files in directory
  -> Read & validate headers
  -> Verify credentials (fast fail via HMAC tag)
  -> Verify payload checksums (exclude corrupt shards)
  -> Reconstruct missing shards (if needed, requires >= N available)
  -> Reed-Solomon join data shards
  -> AES-256-CTR decrypt (if encrypted)
  -> Write output file
```

### Crypto design

| Component | Algorithm |
|---|---|
| Encryption | AES-256-CTR (stream cipher, no padding) |
| Key derivation | Argon2id (time=3, memory=64 MB, parallelism=4) |
| Key file | SHA-256 hash of file contents (combined with password via HMAC-SHA256) |
| Credential check | HMAC-SHA256(key, sentinel)[:8] stored in header |
| Payload integrity | SHA-256 checksum in trailer |
| Header integrity | SHA-256 checksum in header |

Encryption happens **before** erasure coding, so each shard contains encrypted data. The salt and IV are generated once per split and stored in every shard header.

### Shard file format

Each `.hrcx` file has a 256-byte fixed header, variable-length payload, and 32-byte trailer:

```
HEADER (256 bytes)
  Magic "HRCX" | version | shard index | N | K
  Original file size | shard payload size
  Encryption flags | KDF salt (32B) | AES-CTR IV (16B)
  Argon2 params | original filename (128B)
  Credential verification tag (8B) | reserved
  Header checksum (SHA-256)

PAYLOAD (variable)
  Encrypted (or plain) erasure-coded shard data

TRAILER (32 bytes)
  Payload checksum (SHA-256)
```

## Error handling

| Scenario | Behavior |
|---|---|
| Corrupt header (bad magic/checksum) | Warn, exclude shard, continue if >= N remain |
| Corrupt payload (checksum mismatch) | Warn, treat as missing, reconstruct if possible |
| Missing > K shards | Fail with clear error message |
| Wrong password or key file | Fail fast via verification tag |
| Inconsistent headers across shards | Warn on mismatched shards |
| Empty file | Valid: produces shards with empty payload |

## Development

```bash
# Run all tests
make test

# Unit tests only
make test-unit

# E2E tests only
make test-e2e

# Stress tests (large files; individual tests skip under `go test -short`)
make test-stress

# Benchmarks
make bench

# Lint
make lint

# Cross-compile (darwin/linux/windows)
make cross-compile
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes with tests for new features
4. Ensure quality checks pass: `make test && make fmt && make lint`
5. Commit with a descriptive message and open a pull request

## License

MIT
