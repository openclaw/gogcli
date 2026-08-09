package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
)

func TestAuthAliasSetListUnset_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := newCmdJSONOutputContext(t, os.Stdout, os.Stderr)

	// set
	_ = captureStdout(t, func() {
		if err := runKong(t, &AuthAliasSetCmd{}, []string{"work", "alias@example.com"}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("set: %v", err)
		}
	})

	// list
	out := captureStdout(t, func() {
		if err := runKong(t, &AuthAliasListCmd{}, []string{}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	var listResp struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal([]byte(out), &listResp); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if listResp.Aliases["work"] != "alias@example.com" {
		t.Fatalf("unexpected aliases: %#v", listResp.Aliases)
	}

	// unset
	_ = captureStdout(t, func() {
		if err := runKong(t, &AuthAliasUnsetCmd{}, []string{"work"}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("unset: %v", err)
		}
	})
}

func TestExecuteAuthAliasCRUDUsesRuntimeConfigStore(t *testing.T) {
	assertRuntimeAliasCRUD(
		t, "auth", "work", "ambient@example.com", "runtime@example.com",
		func(store *config.ConfigStore, key, value string) error { return store.SetAccountAlias(key, value) },
		func(store *config.ConfigStore, key string) (string, bool, error) {
			return store.ResolveAccountAlias(key)
		},
	)
}

func assertRuntimeAliasCRUD(
	t *testing.T,
	namespace, key, ambientValue, runtimeValue string,
	set func(*config.ConfigStore, string, string) error,
	resolve func(*config.ConfigStore, string) (string, bool, error),
) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ambientStore := defaultConfigStoreForTest(t)
	if err := set(ambientStore, key, ambientValue); err != nil {
		t.Fatalf("set ambient alias: %v", err)
	}
	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	runtime := &app.Runtime{Config: runtimeStore}

	setResult := executeWithTestRuntime(t, []string{
		"--json", namespace, "alias", "set", key, runtimeValue,
	}, runtime)
	if setResult.err != nil {
		t.Fatalf("set: %v", setResult.err)
	}
	if value, ok, err := resolve(runtimeStore, key); err != nil || !ok || value != runtimeValue {
		t.Fatalf("runtime alias = %q, ok=%v err=%v", value, ok, err)
	}
	if value, ok, err := resolve(ambientStore, key); err != nil || !ok || value != ambientValue {
		t.Fatalf("ambient alias = %q, ok=%v err=%v", value, ok, err)
	}

	listResult := executeWithTestRuntime(t, []string{"--json", namespace, "alias", "list"}, runtime)
	if listResult.err != nil {
		t.Fatalf("list: %v", listResult.err)
	}
	var listed struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal([]byte(listResult.stdout), &listed); err != nil {
		t.Fatalf("list JSON: %v", err)
	}
	if listed.Aliases[key] != runtimeValue {
		t.Fatalf("listed aliases = %#v", listed.Aliases)
	}

	unsetResult := executeWithTestRuntime(t, []string{"--json", namespace, "alias", "unset", key}, runtime)
	if unsetResult.err != nil {
		t.Fatalf("unset: %v", unsetResult.err)
	}
	if _, ok, err := resolve(runtimeStore, key); err != nil || ok {
		t.Fatalf("runtime alias after unset: ok=%v err=%v", ok, err)
	}
}
