//go:build integration

package tracking

import (
	"os"
	"strings"
	"testing"
)

func TestIntegrationEncryptDecryptWithWorker(t *testing.T) {
	account := strings.TrimSpace(os.Getenv("RATA_IT_ACCOUNT"))
	if account == "" {
		account = strings.TrimSpace(os.Getenv("GOG_IT_ACCOUNT"))
	}
	if account == "" {
		t.Skip("set RATA_IT_ACCOUNT to run integration test")
	}

	cfg, err := LoadConfig(account)
	if err != nil || !cfg.IsConfigured() {
		t.Skip("Tracking not configured, skipping integration test")
	}

	// Generate a pixel URL
	pixelURL, blob, err := GeneratePixelURL(cfg, "integration-test@example.com", "Test Subject")
	if err != nil {
		t.Fatalf("GeneratePixelURL failed: %v", err)
	}

	t.Logf("Generated pixel URL: %s", pixelURL)
	t.Logf("Blob: %s", blob)

	// Verify we can decrypt locally
	payload, err := Decrypt(blob, cfg.TrackingKey)
	if err != nil {
		t.Fatalf("Local decrypt failed: %v", err)
	}

	if payload.Recipient != "integration-test@example.com" {
		t.Errorf("Recipient mismatch: %s", payload.Recipient)
	}
}
