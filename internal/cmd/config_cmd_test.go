package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
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

func TestConfigCmd_ConcurrentMutationsPreserveAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(context.Context, *RootFlags) error
	}{
		{
			name: "set",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ConfigSetCmd{Key: "timezone", Value: "America/Los_Angeles"}).Run(ctx, flags)
			},
		},
		{
			name: "unset",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ConfigUnsetCmd{Key: "timezone"}).Run(ctx, flags)
			},
		},
		{
			name: "keyring",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&AuthKeyringCmd{Backend: "file"}).Run(ctx, flags)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			layout := config.Layout{ConfigDir: t.TempDir()}
			store := config.NewConfigStore(layout)
			if err := store.Write(config.File{DefaultTimezone: "UTC"}); err != nil {
				t.Fatalf("write config: %v", err)
			}

			const aliasCount = 24
			start := make(chan struct{})
			errs := make(chan error, aliasCount*2)
			var wg sync.WaitGroup
			wg.Add(aliasCount * 2)

			for i := range aliasCount {
				ctx := withTestRuntime(newCmdOutputContext(t, io.Discard, io.Discard), func(runtime *app.Runtime) {
					runtime.Config = config.NewConfigStore(layout)
					runtime.IO = app.IO{Out: io.Discard, Err: io.Discard}
				})
				go func() {
					defer wg.Done()
					<-start

					errs <- tt.run(ctx, &RootFlags{})
				}()

				go func() {
					defer wg.Done()
					<-start

					alias := fmt.Sprintf("proof-%02d", i)
					errs <- config.NewConfigStore(layout).SetAccountAlias(alias, "clawdbot@gmail.com")
				}()
			}

			close(start)
			wg.Wait()
			close(errs)

			for err := range errs {
				if err != nil {
					t.Fatalf("concurrent update: %v", err)
				}
			}

			cfg, err := store.Read()
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			if len(cfg.AccountAliases) != aliasCount {
				t.Fatalf("retained %d of %d concurrent account aliases: %#v", len(cfg.AccountAliases), aliasCount, cfg.AccountAliases)
			}
		})
	}
}

func TestConfigCmd_DryRunLeavesConfigAndLockUntouched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(context.Context, *RootFlags) error
	}{
		{
			name: "set",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ConfigSetCmd{Key: "timezone", Value: "UTC"}).Run(ctx, flags)
			},
		},
		{
			name: "unset",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&ConfigUnsetCmd{Key: "timezone"}).Run(ctx, flags)
			},
		},
		{
			name: "keyring",
			run: func(ctx context.Context, flags *RootFlags) error {
				return (&AuthKeyringCmd{Backend: "file"}).Run(ctx, flags)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			configDir := filepath.Join(t.TempDir(), "config")
			ctx := withTestRuntime(newCmdOutputContext(t, io.Discard, io.Discard), func(runtime *app.Runtime) {
				runtime.Config = config.NewConfigStore(config.Layout{ConfigDir: configDir})
				runtime.IO = app.IO{Out: io.Discard, Err: io.Discard}
			})
			if err := tt.run(ctx, &RootFlags{DryRun: true}); err == nil || ExitCode(err) != 0 {
				t.Fatalf("dry run: %v", err)
			}
			if _, err := os.Stat(configDir); !os.IsNotExist(err) {
				t.Fatalf("dry run touched configuration or lock directory: %v", err)
			}
		})
	}
}

func TestConfigSetRedactsSensitiveValues(t *testing.T) {
	t.Parallel()

	for _, key := range []config.Key{config.KeyYoutubeAPIKey, config.KeyPlacesAPIKey} {
		for _, tc := range []struct {
			name   string
			json   bool
			dryRun bool
		}{
			{name: "JSON preview", json: true, dryRun: true},
			{name: "plain preview", dryRun: true},
			{name: "JSON saved", json: true},
			{name: "plain saved"},
		} {
			t.Run(key.String()+" "+tc.name, func(t *testing.T) {
				t.Parallel()

				configDir := filepath.Join(t.TempDir(), "config")
				store := config.NewConfigStore(config.Layout{ConfigDir: configDir})
				runtime := &app.Runtime{Config: store}
				value := "synthetic-" + key.String() + "-value"
				args := []string{}
				if tc.json {
					args = append(args, "--json")
				} else {
					args = append(args, "--plain")
				}
				if tc.dryRun {
					args = append(args, "--dry-run")
				}
				args = append(args, "config", "set", key.String(), value)
				result := executeWithTestRuntime(t, args, runtime)
				if result.err != nil {
					t.Fatalf("config set: %v\n%s", result.err, result.stderr)
				}
				if strings.Contains(result.stdout, value) || strings.Contains(result.stderr, value) ||
					!strings.Contains(result.stdout, "[REDACTED]") {
					t.Fatalf("sensitive value escaped output: stdout=%q stderr=%q", result.stdout, result.stderr)
				}
				if tc.dryRun {
					if _, err := os.Stat(configDir); !os.IsNotExist(err) {
						t.Fatalf("dry-run created configuration state: %v", err)
					}
					return
				}
				cfg, err := store.Read()
				if err != nil {
					t.Fatalf("read stored config: %v", err)
				}
				stored := cfg.YoutubeAPIKey
				if key == config.KeyPlacesAPIKey {
					stored = cfg.PlacesAPIKey
				}
				if stored != value {
					t.Fatalf("stored value = %q, want the unchanged original", stored)
				}
			})
		}
	}
}

func TestRemoveDomainMappings_NoMatchLeavesConfigAbsent(t *testing.T) {
	t.Parallel()

	configDir := filepath.Join(t.TempDir(), "config")
	ctx := withTestRuntime(newCmdOutputContext(t, io.Discard, io.Discard), func(runtime *app.Runtime) {
		runtime.Config = config.NewConfigStore(config.Layout{ConfigDir: configDir})
	})
	removed, err := removeDomainMappings(ctx, "work")
	if err != nil {
		t.Fatalf("remove domain mappings: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed unexpected domains: %v", removed)
	}
	if _, err := os.Stat(filepath.Join(configDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("unchanged domain cleanup created config file: %v", err)
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
