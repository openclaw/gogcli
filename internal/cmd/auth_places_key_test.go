package cmd

import (
	"encoding/json"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestAuthPlacesKeyConfigFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOOGLE_PLACES_API_KEY", "")

	setOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "places-key", "set", "--key", "places-123", "--store", "config"}); err != nil {
				t.Fatalf("Execute set: %v", err)
			}
		})
	})

	var setResp struct {
		Saved bool `json:"saved"`
	}
	if err := json.Unmarshal([]byte(setOut), &setResp); err != nil {
		t.Fatalf("set json parse: %v\nout=%q", err, setOut)
	}
	if !setResp.Saved {
		t.Fatalf("expected saved true")
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.PlacesAPIKey != "places-123" {
		t.Fatalf("expected places api key stored, got %q", cfg.PlacesAPIKey)
	}

	statusOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{"--json", "auth", "places-key", "status"}); execErr != nil {
				t.Fatalf("Execute status: %v", execErr)
			}
		})
	})

	var status struct {
		Configured bool   `json:"configured"`
		Source     string `json:"source"`
	}
	if unmarshalErr := json.Unmarshal([]byte(statusOut), &status); unmarshalErr != nil {
		t.Fatalf("status json parse: %v\nout=%q", unmarshalErr, statusOut)
	}
	if !status.Configured {
		t.Fatalf("expected configured true")
	}
	if status.Source != "config" {
		t.Fatalf("expected source config, got %q", status.Source)
	}

	clearOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if execErr := Execute([]string{"--json", "auth", "places-key", "clear", "--store", "config"}); execErr != nil {
				t.Fatalf("Execute clear: %v", execErr)
			}
		})
	})

	var clearResp struct {
		Cleared bool `json:"cleared"`
	}
	if unmarshalErr := json.Unmarshal([]byte(clearOut), &clearResp); unmarshalErr != nil {
		t.Fatalf("clear json parse: %v\nout=%q", unmarshalErr, clearOut)
	}
	if !clearResp.Cleared {
		t.Fatalf("expected cleared true")
	}

	cfg, err = config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.PlacesAPIKey != "" {
		t.Fatalf("expected places api key cleared, got %q", cfg.PlacesAPIKey)
	}
}
