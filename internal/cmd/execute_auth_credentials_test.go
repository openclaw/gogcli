package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/degree-analytics/ratatosk/internal/config"
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
