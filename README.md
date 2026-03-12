# Horcrux (`hrcx`)

A cross-platform CLI tool that splits files into encrypted, erasure-coded shards and reconstructs the original file from a subset of those shards.

Split a file into N data + K parity shards. Lose up to K shards and still recover the original file.

## Features

- **Erasure coding** via Reed-Solomon (powered by [klauspost/reedsolomon](https://github.com/klauspost/reedsolomon))
- **AES-256-CTR encryption** with Argon2id key derivation
- **Fast wrong-password detection** via HMAC verification tag (no need to process the entire file)
- **Streaming pipeline** with constant memory usage (~15 MB regardless of file size)
- **Corruption tolerance** -- corrupt shards are automatically detected and excluded during reconstruction
- **Config file support** -- set defaults via `.hrcxrc` or `~/.config/horcrux/config.yaml`
- **Single binary**, no dependencies at runtime

## Installation

### From source

```bash
go install github.com/marmos91/horcrux@latest
```

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

This produces N+K shard files named `<filename>.<index>.hrcx`:

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
```

Merge tolerates up to K missing or corrupt shards. If any data shards are missing, they are automatically reconstructed from the available parity shards.

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
├── Encrypted:         yes
└── Header checksum:   OK
```

### Configuration file

Horcrux supports an optional YAML config file to set default values for CLI flags. Settings in the config file are overridden by explicit CLI flags.

**Search order** (first found wins):
1. `./.hrcxrc` (current directory)
2. `~/.config/horcrux/config.yaml`
3. `~/.hrcxrc`

**Precedence** (highest to lowest): CLI flags > config file > built-in defaults.

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
no-encrypt: false
workers: 8
fail-fast: true
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
  -> Verify password (fast fail via HMAC tag)
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
| Password check | HMAC-SHA256(key, sentinel)[:8] stored in header |
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
  Password verification tag (8B) | reserved
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
| Wrong password | Fail fast via verification tag |
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

# Stress tests (large files, skip with -short)
make test-stress

# Benchmarks
make bench

# Lint
make lint

# Cross-compile (darwin/linux/windows)
make cross-compile
```

## License

MIT
