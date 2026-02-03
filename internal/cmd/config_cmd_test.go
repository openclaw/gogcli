package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
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

func TestConfigGetCmd_TextOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.File{DefaultTimezone: "America/New_York"}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "get", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "America/New_York") {
		t.Fatalf("expected output to contain timezone, got %q", out)
	}
}

func TestConfigGetCmd_EmptyValueShowsHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config-home"))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "get", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "not set") {
		t.Fatalf("expected hint for empty value, got %q", out)
	}
}

func TestConfigGetCmd_InvalidKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := Execute([]string{"config", "get", "invalid_key"})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Fatalf("expected unknown config key error, got %v", err)
	}
}

func TestConfigSetCmd_ValidTimezone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "set", "timezone", "UTC"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Set timezone = UTC") {
		t.Fatalf("expected confirmation message, got %q", out)
	}

	// Verify value was persisted
	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.DefaultTimezone != "UTC" {
		t.Fatalf("expected timezone UTC, got %q", cfg.DefaultTimezone)
	}
}

func TestConfigSetCmd_JSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "set", "timezone", "Europe/London"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Key   string `json:"key"`
		Value string `json:"value"`
		Saved bool   `json:"saved"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if result.Key != "timezone" || result.Value != "Europe/London" || !result.Saved {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestConfigSetCmd_InvalidTimezone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := Execute([]string{"config", "set", "timezone", "Invalid/Timezone"})
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if !strings.Contains(err.Error(), "invalid timezone") {
		t.Fatalf("expected invalid timezone error, got %v", err)
	}
}

func TestConfigSetCmd_InvalidKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := Execute([]string{"config", "set", "invalid_key", "value"})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Fatalf("expected unknown config key error, got %v", err)
	}
}

func TestConfigUnsetCmd_RemovesValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.File{DefaultTimezone: "UTC"}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "unset", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Unset timezone") {
		t.Fatalf("expected confirmation message, got %q", out)
	}

	// Verify value was removed
	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.DefaultTimezone != "" {
		t.Fatalf("expected empty timezone, got %q", cfg.DefaultTimezone)
	}
}

func TestConfigUnsetCmd_JSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.File{DefaultTimezone: "UTC"}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "unset", "timezone"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Key     string `json:"key"`
		Removed bool   `json:"removed"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if result.Key != "timezone" || !result.Removed {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestConfigUnsetCmd_InvalidKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := Execute([]string{"config", "unset", "invalid_key"})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Fatalf("expected unknown config key error, got %v", err)
	}
}

func TestConfigKeysCmd_TextOutput(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "keys"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "timezone") {
		t.Fatalf("expected timezone key in output, got %q", out)
	}
	if !strings.Contains(out, "keyring_backend") {
		t.Fatalf("expected keyring_backend key in output, got %q", out)
	}
}

func TestConfigKeysCmd_JSONOutput(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "keys"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	hasTimezone := false
	hasKeyring := false
	for _, key := range result.Keys {
		if key == "timezone" {
			hasTimezone = true
		}
		if key == "keyring_backend" {
			hasKeyring = true
		}
	}
	if !hasTimezone || !hasKeyring {
		t.Fatalf("expected timezone and keyring_backend keys, got %v", result.Keys)
	}
}

func TestConfigListCmd_TextOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.File{
		DefaultTimezone: "Asia/Tokyo",
		KeyringBackend:  "file",
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Asia/Tokyo") {
		t.Fatalf("expected timezone value in output, got %q", out)
	}
	if !strings.Contains(out, "file") {
		t.Fatalf("expected keyring_backend value in output, got %q", out)
	}
	if !strings.Contains(out, "Config file:") {
		t.Fatalf("expected config file path in output, got %q", out)
	}
}

func TestConfigListCmd_ShowsNotSetHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config-home"))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "not set") {
		t.Fatalf("expected 'not set' hint in output, got %q", out)
	}
}

func TestConfigPathCmd_TextOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "path"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "config.json") {
		t.Fatalf("expected config path to contain config.json, got %q", out)
	}
}

func TestConfigPathCmd_JSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "config", "path"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !strings.Contains(result.Path, "config.json") {
		t.Fatalf("expected path to contain config.json, got %q", result.Path)
	}
}

func TestConfigSetCmd_KeyringBackend(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"config", "set", "keyring_backend", "file"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Set keyring_backend = file") {
		t.Fatalf("expected confirmation message, got %q", out)
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.KeyringBackend != "file" {
		t.Fatalf("expected keyring_backend file, got %q", cfg.KeyringBackend)
	}
}

func TestFormatConfigValue_WithValue(t *testing.T) {
	result := formatConfigValue("test-value", func() string { return "hint" })
	if result != "test-value" {
		t.Fatalf("expected test-value, got %q", result)
	}
}

func TestFormatConfigValue_EmptyWithHint(t *testing.T) {
	result := formatConfigValue("", func() string { return "(custom hint)" })
	if result != "(custom hint)" {
		t.Fatalf("expected (custom hint), got %q", result)
	}
}

func TestFormatConfigValue_EmptyNilHint(t *testing.T) {
	result := formatConfigValue("", nil)
	if result != "(not set)" {
		t.Fatalf("expected (not set), got %q", result)
	}
}
