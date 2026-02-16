package tracking

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	payload := &PixelPayload{
		Recipient:   "test@example.com",
		SubjectHash: "abc123",
		SentAt:      time.Now().Unix(),
	}

	encrypted, err := Encrypt(payload, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted.Recipient != payload.Recipient {
		t.Errorf("Recipient mismatch: got %q, want %q", decrypted.Recipient, payload.Recipient)
	}

	if decrypted.SubjectHash != payload.SubjectHash {
		t.Errorf("SubjectHash mismatch: got %q, want %q", decrypted.SubjectHash, payload.SubjectHash)
	}

	if decrypted.SentAt != payload.SentAt {
		t.Errorf("SentAt mismatch: got %d, want %d", decrypted.SentAt, payload.SentAt)
	}
}

func TestEncryptProducesURLSafeOutput(t *testing.T) {
	key, _ := GenerateKey()
	payload := &PixelPayload{
		Recipient:   "test@example.com",
		SubjectHash: "abc123",
		SentAt:      time.Now().Unix(),
	}

	encrypted, err := Encrypt(payload, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// URL-safe base64 should not contain +, /, or =
	for _, c := range encrypted {
		if c == '+' || c == '/' || c == '=' {
			t.Errorf("Output contains non-URL-safe character: %c", c)
		}
	}
}

func TestEncryptWithVersionAndDecryptWithVersions(t *testing.T) {
	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	payload := &PixelPayload{
		Recipient:   "test@example.com",
		SubjectHash: "abc123",
		SentAt:      time.Now().Unix(),
	}

	encrypted, err := EncryptWithVersion(payload, key1, "1")
	if err != nil {
		t.Fatalf("EncryptWithVersion failed: %v", err)
	}

	decrypted, err := DecryptWithVersions(encrypted, map[string]string{
		"2": key2,
		"1": key1,
	})
	if err != nil {
		t.Fatalf("DecryptWithVersions failed: %v", err)
	}

	if decrypted.Recipient != payload.Recipient {
		t.Errorf("Recipient mismatch: got %q, want %q", decrypted.Recipient, payload.Recipient)
	}
}

func TestDecryptLegacyBlobWithVersions(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	payload := &PixelPayload{
		Recipient:   "test@example.com",
		SubjectHash: "abc123",
		SentAt:      time.Now().Unix(),
	}

	versioned, err := EncryptWithVersion(payload, key, "1")
	if err != nil {
		t.Fatalf("EncryptWithVersion failed: %v", err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(versioned)
	if err != nil {
		t.Fatalf("decode versioned blob: %v", err)
	}

	if len(raw) == 0 {
		t.Fatalf("unexpected empty blob")
	}

	legacy := base64.RawURLEncoding.EncodeToString(raw[1:])

	decrypted, err := DecryptWithVersions(legacy, map[string]string{
		"1": key,
	})
	if err != nil {
		t.Fatalf("DecryptWithVersions legacy: %v", err)
	}

	if decrypted.SubjectHash != payload.SubjectHash {
		t.Errorf("SubjectHash mismatch: got %q, want %q", decrypted.SubjectHash, payload.SubjectHash)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()

	payload := &PixelPayload{
		Recipient:   "test@example.com",
		SubjectHash: "abc123",
		SentAt:      time.Now().Unix(),
	}

	encrypted, _ := Encrypt(payload, key1)

	_, err := Decrypt(encrypted, key2)
	if err == nil {
		t.Error("Expected error when decrypting with wrong key")
	}
}
