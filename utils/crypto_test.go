package utils

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	secret := "test-encryption-secret"

	for _, plaintext := range []string{"", "hello", "sk_live_abc123", strings.Repeat("x", 10000)} {
		encrypted, err := EncryptString(secret, plaintext)
		if err != nil {
			t.Fatalf("EncryptString(%q) error: %v", plaintext, err)
		}
		if encrypted == plaintext && plaintext != "" {
			t.Errorf("ciphertext equals plaintext for %q", plaintext)
		}

		decrypted, err := DecryptString(secret, encrypted)
		if err != nil {
			t.Fatalf("DecryptString error: %v", err)
		}
		if decrypted != plaintext {
			t.Errorf("round trip mismatch: got %q, want %q", decrypted, plaintext)
		}
	}
}

func TestDecryptWithWrongSecretFails(t *testing.T) {
	encrypted, err := EncryptString("secret-a", "sensitive")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptString("secret-b", encrypted); err == nil {
		t.Error("expected decryption with wrong secret to fail")
	}
}

func TestDecryptGarbageFails(t *testing.T) {
	if _, err := DecryptString("secret", "not-valid-base64!!!"); err == nil {
		t.Error("expected invalid base64 to fail")
	}
	if _, err := DecryptString("secret", "dG9vc2hvcnQ="); err == nil {
		t.Error("expected too-short ciphertext to fail")
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	a, _ := EncryptString("secret", "same-input")
	b, _ := EncryptString("secret", "same-input")
	if a == b {
		t.Error("two encryptions of the same plaintext must differ (random nonce)")
	}
}
