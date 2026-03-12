package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"

	"golang.org/x/crypto/argon2"
)

const (
	DefaultArgonTime        = 3
	DefaultArgonMemory      = 64 * 1024 // 64 MB
	DefaultArgonParallelism = 4
	KeyLen                  = 32 // AES-256

	passwordSentinel = "HORCRUX-PASSWORD-CHECK"
)

// KDFParams holds Argon2id parameters.
type KDFParams struct {
	Time        uint32
	Memory      uint32
	Parallelism uint8
}

// DefaultKDFParams returns the default Argon2id parameters.
func DefaultKDFParams() KDFParams {
	return KDFParams{
		Time:        DefaultArgonTime,
		Memory:      DefaultArgonMemory,
		Parallelism: DefaultArgonParallelism,
	}
}

// GenerateSalt generates a cryptographically random 32-byte salt.
func GenerateSalt() ([32]byte, error) {
	var salt [32]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return salt, err
	}
	return salt, nil
}

// GenerateIV generates a cryptographically random 16-byte IV for AES-CTR.
func GenerateIV() ([16]byte, error) {
	var iv [16]byte
	if _, err := rand.Read(iv[:]); err != nil {
		return iv, err
	}
	return iv, nil
}

// DeriveKey derives a 32-byte key from a password and salt using Argon2id.
func DeriveKey(password string, salt [32]byte, params KDFParams) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt[:],
		params.Time,
		params.Memory,
		params.Parallelism,
		KeyLen,
	)
}

// PasswordTag computes HMAC-SHA256(key, sentinel)[:8] for fast password verification.
func PasswordTag(key []byte) [8]byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(passwordSentinel))
	full := mac.Sum(nil)
	var tag [8]byte
	copy(tag[:], full[:8])
	return tag
}

// VerifyPasswordTag checks if the given tag matches the expected tag for the key.
func VerifyPasswordTag(key []byte, expected [8]byte) bool {
	computed := PasswordTag(key)
	return hmac.Equal(computed[:], expected[:])
}
