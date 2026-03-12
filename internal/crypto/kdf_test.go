package crypto

import (
	"testing"
)

func TestDeriveKeyDeterministic(t *testing.T) {
	salt := [32]byte{1, 2, 3, 4, 5}
	params := KDFParams{Time: 1, Memory: 64 * 1024, Parallelism: 1}

	key1 := DeriveKey("password", salt, params)
	key2 := DeriveKey("password", salt, params)

	if len(key1) != KeyLen {
		t.Fatalf("key length: got %d, want %d", len(key1), KeyLen)
	}

	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("same password+salt should produce same key")
		}
	}
}

func TestDeriveKeyDifferentSalts(t *testing.T) {
	salt1 := [32]byte{1}
	salt2 := [32]byte{2}
	params := KDFParams{Time: 1, Memory: 64 * 1024, Parallelism: 1}

	key1 := DeriveKey("password", salt1, params)
	key2 := DeriveKey("password", salt2, params)

	same := true
	for i := range key1 {
		if key1[i] != key2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different salts should produce different keys")
	}
}

func TestDeriveKeyDifferentPasswords(t *testing.T) {
	salt := [32]byte{1, 2, 3}
	params := KDFParams{Time: 1, Memory: 64 * 1024, Parallelism: 1}

	key1 := DeriveKey("password1", salt, params)
	key2 := DeriveKey("password2", salt, params)

	same := true
	for i := range key1 {
		if key1[i] != key2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different passwords should produce different keys")
	}
}

func TestPasswordTagMatch(t *testing.T) {
	salt := [32]byte{1, 2, 3}
	params := KDFParams{Time: 1, Memory: 64 * 1024, Parallelism: 1}
	key := DeriveKey("correct-password", salt, params)

	tag := PasswordTag(key)
	if !VerifyPasswordTag(key, tag) {
		t.Fatal("password tag should match for correct key")
	}
}

func TestPasswordTagMismatch(t *testing.T) {
	salt := [32]byte{1, 2, 3}
	params := KDFParams{Time: 1, Memory: 64 * 1024, Parallelism: 1}

	key1 := DeriveKey("correct-password", salt, params)
	key2 := DeriveKey("wrong-password", salt, params)

	tag := PasswordTag(key1)
	if VerifyPasswordTag(key2, tag) {
		t.Fatal("password tag should not match for wrong key")
	}
}

func TestGenerateSalt(t *testing.T) {
	s1, err := GenerateSalt()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := GenerateSalt()
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s2 {
		t.Fatal("two random salts should differ")
	}
}

func TestGenerateIV(t *testing.T) {
	iv1, err := GenerateIV()
	if err != nil {
		t.Fatal(err)
	}
	iv2, err := GenerateIV()
	if err != nil {
		t.Fatal(err)
	}
	if iv1 == iv2 {
		t.Fatal("two random IVs should differ")
	}
}
