package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal([]byte("refresh-token"), "youtube:ch1:refresh")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("refresh-token")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	got, err := b.Open(sealed, "youtube:ch1:refresh")
	if err != nil || string(got) != "refresh-token" {
		t.Fatalf("open: %q %v", got, err)
	}
	again, _ := b.Seal([]byte("refresh-token"), "youtube:ch1:refresh")
	if bytes.Equal(sealed, again) {
		t.Fatal("nonces must differ between seals")
	}
}

func TestOpenFailures(t *testing.T) {
	key, _ := GenerateKey()
	b, _ := NewBox(key)
	sealed, _ := b.Seal([]byte("secret"), "ctx-a")

	if _, err := b.Open(sealed, "ctx-b"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("wrong context: %v", err)
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	if _, err := b.Open(tampered, "ctx-a"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("tampered: %v", err)
	}
	if _, err := b.Open([]byte("short"), "ctx-a"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("short: %v", err)
	}
	otherKey, _ := GenerateKey()
	other, _ := NewBox(otherKey)
	if _, err := other.Open(sealed, "ctx-a"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("wrong key: %v", err)
	}
}

func TestNewBoxValidation(t *testing.T) {
	if _, err := NewBox("not base64!"); err == nil {
		t.Error("expected error for invalid base64")
	}
	if _, err := NewBox(base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Error("expected error for short key")
	}
}
