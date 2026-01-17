package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestConfigCmd_JSONParity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.File{
		KeyringBackend:  "file",
		DefaultTimezone: "UTC",
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var list struct {
		Timezone       string `json:"timezone"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("list json parse: %v\nout=%q", err, listOut)
	}

	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "get", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var get struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(getOut), &get); err != nil {
		t.Fatalf("get json parse: %v\nout=%q", err, getOut)
	}
	if get.Key != "timezone" {
		t.Fatalf("expected key timezone, got %q", get.Key)
	}
	if get.Value != list.Timezone {
		t.Fatalf("expected timezone %q, got %q", list.Timezone, get.Value)
	}
}

func TestConfigCmd_JSONEmptyValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config-home"))

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var list struct {
		Timezone       string `json:"timezone"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("list json parse: %v\nout=%q", err, listOut)
	}
	if list.Timezone != "" {
		t.Fatalf("expected empty timezone, got %q", list.Timezone)
	}
	if list.KeyringBackend != "" {
		t.Fatalf("expected empty keyring_backend, got %q", list.KeyringBackend)
	}

	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "get", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var get struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(getOut), &get); err != nil {
		t.Fatalf("get json parse: %v\nout=%q", err, getOut)
	}
	if get.Value != "" {
		t.Fatalf("expected empty value, got %q", get.Value)
	}
}
