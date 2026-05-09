package tracking

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGeneratePixelURL(t *testing.T) {
	key, _ := GenerateKey()
	cfg := &Config{
		Enabled:     true,
		WorkerURL:   "https://test.workers.dev",
		TrackingKey: key,
	}

	pixelURL, blob, err := GeneratePixelURL(cfg, "test@example.com", "Hello World")
	if err != nil {
		t.Fatalf("GeneratePixelURL failed: %v", err)
	}

	if !strings.HasPrefix(pixelURL, "https://test.workers.dev/p/") {
		t.Errorf("Unexpected URL prefix: %s", pixelURL)
	}

	if !strings.HasSuffix(pixelURL, ".gif") {
		t.Errorf("URL should end with .gif: %s", pixelURL)
	}

	if blob == "" {
		t.Error("Blob should not be empty")
	}
}

func TestGeneratePixelURLUsesCurrentKeyVersion(t *testing.T) {
	key, _ := GenerateKey()
	cfg := &Config{
		Enabled:                   true,
		WorkerURL:                 "https://test.workers.dev",
		TrackingKey:               key,
		TrackingCurrentKeyVersion: 2,
	}

	_, blob, err := GeneratePixelURL(cfg, "test@example.com", "Hello World")
	if err != nil {
		t.Fatalf("GeneratePixelURL failed: %v", err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}

	if got := int(raw[0]); got != 2 {
		t.Fatalf("version prefix = %d, want 2", got)
	}
}

func TestGeneratePixelURLNotConfigured(t *testing.T) {
	cfg := &Config{Enabled: false}

	_, _, err := GeneratePixelURL(cfg, "test@example.com", "Hello")
	if err == nil {
		t.Error("Expected error for unconfigured tracking")
	}
}

func TestGeneratePixelHTML(t *testing.T) {
	html := GeneratePixelHTML("https://test.workers.dev/p/abc123.gif")

	if !strings.Contains(html, `src="https://test.workers.dev/p/abc123.gif"`) {
		t.Errorf("HTML missing src: %s", html)
	}

	if !strings.Contains(html, `width="1"`) {
		t.Errorf("HTML missing width: %s", html)
	}

	if !strings.Contains(html, `style="display:none`) {
		t.Errorf("HTML missing display:none: %s", html)
	}
}

func TestHashSubjectConsistent(t *testing.T) {
	h1 := hashSubject("Hello World")
	h2 := hashSubject("Hello World")
	h3 := hashSubject("Different Subject")

	if h1 != h2 {
		t.Error("Same subject should produce same hash")
	}

	if h1 == h3 {
		t.Error("Different subjects should produce different hashes")
	}

	if len(h1) != 6 {
		t.Errorf("Hash should be 6 chars, got %d", len(h1))
	}
}
