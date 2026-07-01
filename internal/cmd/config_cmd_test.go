package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
)

func TestConfigCmd_JSONParity(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	cfg := config.File{
		KeyringBackend:  "file",
		DefaultTimezone: "UTC",
	}
	if err := store.Write(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}
	runtime := &app.Runtime{Config: store}

	listResult := executeWithTestRuntime(t, []string{"--json", "config", "list"}, runtime)
	if listResult.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", listResult.err, listResult.stderr)
	}

	var list struct {
		Timezone       string `json:"timezone"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(listResult.stdout), &list); err != nil {
		t.Fatalf("list json parse: %v\nout=%q", err, listResult.stdout)
	}

	getResult := executeWithTestRuntime(t, []string{"--json", "config", "get", "timezone"}, runtime)
	if getResult.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", getResult.err, getResult.stderr)
	}

	var get struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(getResult.stdout), &get); err != nil {
		t.Fatalf("get json parse: %v\nout=%q", err, getResult.stdout)
	}
	if get.Key != "timezone" {
		t.Fatalf("expected key timezone, got %q", get.Key)
	}
	if get.Value != list.Timezone {
		t.Fatalf("expected timezone %q, got %q", list.Timezone, get.Value)
	}
}

func TestConfigCmd_JSONEmptyValues(t *testing.T) {
	t.Parallel()

	runtime := &app.Runtime{Config: config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})}
	listResult := executeWithTestRuntime(t, []string{"--json", "config", "list"}, runtime)
	if listResult.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", listResult.err, listResult.stderr)
	}

	var list struct {
		Timezone       string `json:"timezone"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(listResult.stdout), &list); err != nil {
		t.Fatalf("list json parse: %v\nout=%q", err, listResult.stdout)
	}
	if list.Timezone != "" {
		t.Fatalf("expected empty timezone, got %q", list.Timezone)
	}
	if list.KeyringBackend != "" {
		t.Fatalf("expected empty keyring_backend, got %q", list.KeyringBackend)
	}

	getResult := executeWithTestRuntime(t, []string{"--json", "config", "get", "timezone"}, runtime)
	if getResult.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", getResult.err, getResult.stderr)
	}

	var get struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(getResult.stdout), &get); err != nil {
		t.Fatalf("get json parse: %v\nout=%q", err, getResult.stdout)
	}
	if get.Value != "" {
		t.Fatalf("expected empty value, got %q", get.Value)
	}
}

func TestConfigCmd_InvalidInputIsUsageError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "get unknown key",
			args: []string{"config", "get", "nope"},
			want: "unknown config key",
		},
		{
			name: "set unknown key",
			args: []string{"config", "set", "nope", "value"},
			want: "unknown config key",
		},
		{
			name: "unset unknown key",
			args: []string{"config", "unset", "nope"},
			want: "unknown config key",
		},
		{
			name: "set invalid value",
			args: []string{"config", "set", "gmail_no_send", "maybe"},
			want: "invalid bool",
		},
		{
			name: "set invalid onepassword timeout",
			args: []string{"config", "set", "onepassword_timeout", "0s"},
			want: "invalid onepassword_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := &app.Runtime{Config: config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})}
			result := executeWithTestRuntime(t, tt.args, runtime)
			err := result.err
			if err == nil {
				t.Fatal("expected error")
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q in error, got %v", tt.want, err)
			}
		})
	}
}

func TestConfigNoSendRoundTrip(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	runtime := &app.Runtime{Config: store}
	setResult := executeWithTestRuntime(t, []string{"config", "no-send", "set", "User@Example.com"}, runtime)
	if setResult.err != nil {
		t.Fatalf("set: %v", setResult.err)
	}
	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if !cfg.NoSendAccounts["user@example.com"] {
		t.Fatalf("expected normalized no-send account, got %#v", cfg.NoSendAccounts)
	}

	result := executeWithTestRuntime(t, []string{"config", "no-send", "list"}, runtime)
	if result.err != nil {
		t.Fatalf("list: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "user@example.com") {
		t.Fatalf("expected listed account, got %q", result.stdout)
	}

	removeResult := executeWithTestRuntime(t, []string{"config", "no-send", "remove", "user@example.com"}, runtime)
	if removeResult.err != nil {
		t.Fatalf("remove: %v", removeResult.err)
	}
	cfg, err = store.Read()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if len(cfg.NoSendAccounts) != 0 {
		t.Fatalf("expected no no-send accounts, got %#v", cfg.NoSendAccounts)
	}
}
