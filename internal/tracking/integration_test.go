//go:build integration

package tracking

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/termutil"
)

func TestIntegrationEncryptDecryptWithWorker(t *testing.T) {
	account := strings.TrimSpace(os.Getenv("GOG_IT_ACCOUNT"))
	if account == "" {
		t.Skip("set GOG_IT_ACCOUNT to run integration test")
	}

	resolver := config.NewSystemResolver("")
	layout, err := resolver.Resolve(config.PathKindConfig, config.PathKindData, config.PathKindState)
	if err != nil {
		t.Skipf("Tracking layout unavailable: %v", err)
	}
	legacyConfigBase := ""
	if !layout.ExplicitState {
		legacyConfigBase, err = resolver.UserConfigBase()
		if err != nil {
			t.Skipf("Legacy tracking path unavailable: %v", err)
		}
	}
	configStore := config.NewConfigStore(layout)
	secretRepository, err := secrets.Open(secrets.OpenOptionsFromLookup(
		layout,
		configStore,
		os.LookupEnv,
		runtime.GOOS,
		termutil.IsTerminal(os.Stdin),
	))
	if err != nil {
		t.Skipf("Tracking secrets unavailable: %v", err)
	}
	secretStore, err := NewSecretStore(secretRepository)
	if err != nil {
		t.Skipf("Tracking secret store unavailable: %v", err)
	}
	store, err := NewConfigStore(layout, legacyConfigBase, secretStore)
	if err != nil {
		t.Skipf("Tracking store unavailable: %v", err)
	}
	cfg, err := store.Load(account)
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
