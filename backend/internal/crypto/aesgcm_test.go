package crypto

import (
	"errors"
	"testing"
)

func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey()
	ct, err := Encrypt(key, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if ct[:3] != "v1:" {
		t.Fatalf("missing v1: prefix: %q", ct)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "s3cret" {
		t.Fatalf("got %q", pt)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	k1, k2 := testKey(), make([]byte, 32)
	k2[0] = 1
	ct, _ := Encrypt(k1, "s3cret")
	if _, err := Decrypt(k2, ct); err == nil {
		t.Fatal("expected error with wrong key")
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	key := testKey()
	_, err := Decrypt(key, "legacy-plaintext")
	if !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("expected ErrNotEncrypted, got %v", err)
	}
}
