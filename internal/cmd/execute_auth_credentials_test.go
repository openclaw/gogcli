package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestExecute_AuthCredentials_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	in := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(in, []byte(`{"installed":{"client_id":"id","client_secret":"sec"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "credentials", in}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Saved bool   `json:"saved"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Saved || parsed.Path == "" {
		t.Fatalf("unexpected: %#v", parsed)
	}
	outPath, err := config.ClientCredentialsPath()
	if err != nil {
		t.Fatalf("ClientCredentialsPath: %v", err)
	}
	if parsed.Path != outPath {
		t.Fatalf("expected %q, got %q", outPath, parsed.Path)
	}
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		t.Fatalf("stat: %v size=%d", err, st.Size())
	}
}

func TestExecute_AuthCredentials_Stdin_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			withStdin(t, `{"installed":{"client_id":"id","client_secret":"sec"}}`, func() {
				if err := Execute([]string{"--json", "auth", "credentials", "-"}); err != nil {
					t.Fatalf("Execute: %v", err)
				}
			})
		})
	})

	var parsed struct {
		Saved bool   `json:"saved"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Saved || parsed.Path == "" {
		t.Fatalf("unexpected: %#v", parsed)
	}
}

func TestExecute_AuthCredentialsList_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	files := []string{"credentials.json", "credentials-work.json"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"installed":{"client_id":"id","client_secret":"sec"}}`), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	cfg := config.File{
		ClientDomains: map[string]string{
			"example.com": "work",
			"missing.com": "missing",
		},
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "credentials", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Clients []struct {
			Client  string   `json:"client"`
			Path    string   `json:"path"`
			Default bool     `json:"default"`
			Domains []string `json:"domains"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Clients) != 3 {
		t.Fatalf("expected 3 clients, got %d", len(parsed.Clients))
	}
	seen := make(map[string]bool)
	for _, c := range parsed.Clients {
		switch c.Client {
		case "default":
			if !c.Default || c.Path == "" {
				t.Fatalf("default entry unexpected: %#v", c)
			}
		case "work":
			if c.Path == "" || len(c.Domains) != 1 || c.Domains[0] != "example.com" {
				t.Fatalf("work entry unexpected: %#v", c)
			}
		case "missing":
			if c.Path != "" || len(c.Domains) != 1 || c.Domains[0] != "missing.com" {
				t.Fatalf("missing entry unexpected: %#v", c)
			}
		default:
			t.Fatalf("unexpected client: %s", c.Client)
		}
		seen[c.Client] = true
	}
	if !seen["default"] || !seen["work"] || !seen["missing"] {
		t.Fatalf("missing expected entries: %#v", seen)
	}
}

func TestExecute_AuthCredentialsRemove_RemovesCredentialTokenAndDomain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	if err := config.WriteClientCredentialsFor("work", config.ClientCredentials{ClientID: "id", ClientSecret: "sec"}); err != nil {
		t.Fatalf("WriteClientCredentialsFor: %v", err)
	}
	if err := store.SetToken("work", "A@B.COM", secrets.Token{RefreshToken: "rt"}); err != nil {
		t.Fatalf("SetToken work: %v", err)
	}
	if err := store.SetToken(config.DefaultClientName, "default@example.com", secrets.Token{RefreshToken: "rt"}); err != nil {
		t.Fatalf("SetToken default: %v", err)
	}
	if err := config.WriteConfig(config.File{ClientDomains: map[string]string{
		"example.com": "work",
		"other.com":   config.DefaultClientName,
	}}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--force", "auth", "credentials", "remove", "work"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Removed        bool     `json:"removed"`
		Client         string   `json:"client"`
		TokensRemoved  []string `json:"tokens_removed"`
		DomainsRemoved []string `json:"domains_removed"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Removed || parsed.Client != "work" {
		t.Fatalf("unexpected remove output: %#v", parsed)
	}
	if len(parsed.TokensRemoved) != 1 || parsed.TokensRemoved[0] != "a@b.com" {
		t.Fatalf("unexpected removed tokens: %#v", parsed.TokensRemoved)
	}
	if len(parsed.DomainsRemoved) != 1 || parsed.DomainsRemoved[0] != "example.com" {
		t.Fatalf("unexpected removed domains: %#v", parsed.DomainsRemoved)
	}
	path, err := config.ClientCredentialsPathFor("work")
	if err != nil {
		t.Fatalf("ClientCredentialsPathFor: %v", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected work credentials removed, stat err=%v", statErr)
	}
	if _, tokenErr := store.GetToken("work", "a@b.com"); !errors.Is(tokenErr, keyring.ErrKeyNotFound) {
		t.Fatalf("expected work token removed, err=%v", tokenErr)
	}
	if _, defaultTokenErr := store.GetToken(config.DefaultClientName, "default@example.com"); defaultTokenErr != nil {
		t.Fatalf("expected default token retained: %v", defaultTokenErr)
	}
	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if _, ok := cfg.ClientDomains["example.com"]; ok {
		t.Fatalf("expected example.com mapping removed: %#v", cfg.ClientDomains)
	}
	if cfg.ClientDomains["other.com"] != config.DefaultClientName {
		t.Fatalf("expected other.com mapping retained: %#v", cfg.ClientDomains)
	}
}

func TestExecute_AuthCredentialsRemoveAll(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	for _, client := range []string{config.DefaultClientName, "work"} {
		if err := config.WriteClientCredentialsFor(client, config.ClientCredentials{ClientID: "id-" + client, ClientSecret: "sec"}); err != nil {
			t.Fatalf("WriteClientCredentialsFor(%s): %v", client, err)
		}
		if err := store.SetToken(client, client+"@example.com", secrets.Token{RefreshToken: "rt"}); err != nil {
			t.Fatalf("SetToken(%s): %v", client, err)
		}
	}
	if err := config.WriteConfig(config.File{ClientDomains: map[string]string{
		"default.example": config.DefaultClientName,
		"work.example":    "work",
	}}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--force", "auth", "credentials", "remove", "all"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Removed        int      `json:"removed"`
		Clients        []string `json:"clients"`
		TokensRemoved  []string `json:"tokens_removed"`
		DomainsRemoved []string `json:"domains_removed"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Removed != 2 || len(parsed.Clients) != 2 || len(parsed.TokensRemoved) != 2 || len(parsed.DomainsRemoved) != 2 {
		t.Fatalf("unexpected remove-all output: %#v", parsed)
	}
	for _, client := range []string{config.DefaultClientName, "work"} {
		path, err := config.ClientCredentialsPathFor(client)
		if err != nil {
			t.Fatalf("ClientCredentialsPathFor(%s): %v", client, err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s credentials removed, stat err=%v", client, err)
		}
		if _, err := store.GetToken(client, client+"@example.com"); !errors.Is(err, keyring.ErrKeyNotFound) {
			t.Fatalf("expected %s token removed, err=%v", client, err)
		}
	}
	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if len(cfg.ClientDomains) != 0 {
		t.Fatalf("expected all domain mappings removed: %#v", cfg.ClientDomains)
	}
}
