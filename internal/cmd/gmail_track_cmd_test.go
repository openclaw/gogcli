package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/tracking"
)

var errUnexpectedTrackingSecretStoreOpen = errors.New("unexpected tracking secret store open")

func setupTrackingEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")
}

func trackingConfigStoreForTest(t *testing.T) *tracking.ConfigStore {
	t.Helper()
	layout, err := config.ResolveSystemLayoutFor("", config.PathKindConfig, config.PathKindState)
	if err != nil {
		t.Fatalf("resolve tracking layout: %v", err)
	}
	legacyConfigBase := ""
	if !layout.ExplicitState {
		legacyConfigBase, err = config.ResolveUserConfigBase()
		if err != nil {
			t.Fatalf("resolve user config base: %v", err)
		}
	}
	store, err := tracking.NewConfigStore(layout, legacyConfigBase, nil)
	if err != nil {
		t.Fatalf("new tracking config store: %v", err)
	}
	return store
}

func saveTrackingConfigForTest(t *testing.T, cfg *tracking.Config) {
	t.Helper()
	if err := trackingConfigStoreForTest(t).Save("a@b.com", cfg); err != nil {
		t.Fatalf("save tracking config: %v", err)
	}
}

func TestTrackingConfigStoreUsesRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	runtimeConfigDir := filepath.Join(root, "runtime-config")
	runtimeStateDir := filepath.Join(root, "runtime-state")
	ambientConfigDir := filepath.Join(root, "ambient-config")
	ambientStateDir := filepath.Join(root, "ambient-state")
	t.Setenv("GOG_CONFIG_DIR", ambientConfigDir)
	t.Setenv("GOG_STATE_DIR", ambientStateDir)

	ctx := withTestRuntime(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(runtime *app.Runtime) {
		runtime.Layout = config.Layout{
			ConfigDir:      runtimeConfigDir,
			StateDir:       runtimeStateDir,
			ExplicitConfig: true,
			ExplicitState:  true,
		}
	})
	store, err := newTrackingConfigStore(ctx, nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Save("a@b.com", &tracking.Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: "track",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	expected := filepath.Join(runtimeStateDir, "tracking.json")
	if store.Path() != expected {
		t.Fatalf("path = %q, want %q", store.Path(), expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("runtime state file: %v", err)
	}
	for _, path := range []string{ambientConfigDir, ambientStateDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("ambient path unexpectedly touched: %s (%v)", path, err)
		}
	}
}

func TestGmailTrackCommandsUseRuntimeSecretStore(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("GOG_HOME", ambientHome)
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "ambient-password")

	runtimeRoot := t.TempDir()
	secretStore := newMemSecretsStore()
	runtime := &app.Runtime{
		Layout: config.Layout{
			ConfigDir:      filepath.Join(runtimeRoot, "config"),
			StateDir:       filepath.Join(runtimeRoot, "state"),
			ExplicitConfig: true,
			ExplicitState:  true,
		},
		Auth: app.AuthOperations{
			OpenSecretStore: func() (secrets.SecretStore, error) {
				return secretStore, nil
			},
		},
	}
	account := "runtime@example.com"

	setupResult := executeWithTestRuntime(t, []string{
		"--account", account,
		"--no-input",
		"--json",
		"gmail", "track", "setup",
		"--worker-url", "https://example.com",
		"--tracking-key", "track-v1",
		"--admin-key", "admin",
	}, runtime)
	if setupResult.err != nil {
		t.Fatalf("setup: %v", setupResult.err)
	}

	statusResult := executeWithTestRuntime(t, []string{
		"--account", account,
		"--json",
		"gmail", "track", "status",
	}, runtime)
	if statusResult.err != nil {
		t.Fatalf("status: %v", statusResult.err)
	}
	if !strings.Contains(statusResult.stdout, `"configured": true`) {
		t.Fatalf("status output = %q", statusResult.stdout)
	}

	rotateResult := executeWithTestRuntime(t, []string{
		"--account", account,
		"--no-input",
		"--json",
		"gmail", "track", "key", "rotate",
		"--no-deploy",
	}, runtime)
	if rotateResult.err != nil {
		t.Fatalf("rotate: %v", rotateResult.err)
	}

	for _, key := range []string{
		"tracking/runtime@example.com/tracking_key_v1",
		"tracking/runtime@example.com/tracking_key_v2",
		"tracking/runtime@example.com/tracking_key",
		"tracking/runtime@example.com/admin_key",
	} {
		if _, ok := secretStore.secrets[key]; !ok {
			t.Fatalf("runtime secret %q not written: %#v", key, secretStore.secrets)
		}
	}
	if _, err := os.Stat(filepath.Join(ambientHome, "data", "keyring")); !os.IsNotExist(err) {
		t.Fatalf("ambient keyring touched: %v", err)
	}
}

func TestGmailTrackInlineSecretsDoNotOpenRuntimeSecretStore(t *testing.T) {
	root := t.TempDir()
	layout := config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		StateDir:       filepath.Join(root, "state"),
		ExplicitConfig: true,
		ExplicitState:  true,
	}
	store, err := tracking.NewConfigStore(layout, "", nil)
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	account := "inline@example.com"
	if saveErr := store.Save(account, &tracking.Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: "inline-track",
		AdminKey:    "inline-admin",
	}); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	opened := false
	runtime := &app.Runtime{
		Layout: layout,
		Auth: app.AuthOperations{
			OpenSecretStore: func() (secrets.SecretStore, error) {
				opened = true
				return nil, errUnexpectedTrackingSecretStoreOpen
			},
		},
	}
	statusResult := executeWithTestRuntime(t, []string{
		"--account", account,
		"--json",
		"gmail", "track", "status",
	}, runtime)
	if statusResult.err != nil {
		t.Fatalf("status: %v", statusResult.err)
	}

	ctx := withTestRuntime(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(testRuntime *app.Runtime) {
		testRuntime.Layout = layout
		testRuntime.Auth = runtime.Auth
	})
	sendCmd := GmailSendCmd{BodyHTML: "<p>tracked</p>"}
	cfg, err := sendCmd.resolveTrackingConfig(ctx, account, []string{"to@example.com"}, nil, nil, sendCmd.BodyHTML)
	if err != nil {
		t.Fatalf("resolveTrackingConfig: %v", err)
	}
	if cfg.TrackingKey != "inline-track" {
		t.Fatalf("tracking config = %#v", cfg)
	}
	if opened {
		t.Fatalf("inline config opened runtime secret store")
	}
}

func TestGmailTrackSetupAndStatus(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "Next steps") {
			t.Fatalf("expected next steps in stderr: %q", errOut)
		}
	})
	if !strings.Contains(out, "configured\ttrue") {
		t.Fatalf("unexpected setup output: %q", out)
	}
	if !strings.Contains(out, "tracking_key_version\t1") {
		t.Fatalf("missing setup key version: %q", out)
	}

	statusOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(statusOut, "configured\ttrue") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
	if !strings.Contains(statusOut, "tracking_key_version\t1") {
		t.Fatalf("missing status key version: %q", statusOut)
	}
}

func TestGmailTrackSetup_InvalidWorkerNameIsUsageError(t *testing.T) {
	setupTrackingEnv(t)

	err := Execute([]string{
		"--account", "a@b.com",
		"--no-input",
		"gmail", "track", "setup",
		"--worker-name", "!!!",
		"--worker-url", "https://example.com",
		"--dry-run",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid worker name") {
		t.Fatalf("expected invalid worker name error, got: %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestGmailTrackSetup_DryRunDoesNotCreateKeyring(t *testing.T) {
	setupTrackingEnv(t)
	home := t.TempDir()
	t.Setenv("GOG_HOME", home)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"--no-input",
				"--json",
				"--dry-run",
				"gmail", "track", "setup",
				"--worker-url", "https://example.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("unexpected dry-run output: %q", out)
	}
	if _, err := os.Stat(filepath.Join(home, "data", "keyring")); err == nil || !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create keyring dir, stat err=%v", err)
	}
}

func TestGmailTrackSetup_DryRunDoesNotReadExistingKeyringSecrets(t *testing.T) {
	setupTrackingEnv(t)

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"--no-input",
				"--json",
				"gmail", "track", "setup",
				"--worker-url", "https://example.com",
			}); err != nil {
				t.Fatalf("setup: %v", err)
			}
		})
	})

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"--no-input",
				"--json",
				"gmail", "track", "key", "rotate",
				"--no-deploy",
			}); err != nil {
				t.Fatalf("rotate: %v", err)
			}
		})
	})

	t.Setenv("GOG_KEYRING_PASSWORD", "")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"--no-input",
				"--json",
				"--dry-run",
				"gmail", "track", "setup",
				"--worker-url", "https://example.org",
			}); err != nil {
				t.Fatalf("dry-run setup: %v", err)
			}
		})
	})
	if strings.Contains(out, "TRACKING_KEY") || strings.Contains(out, "ADMIN_KEY") {
		t.Fatalf("dry-run output should not contain secrets: %q", out)
	}

	var payload struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			WorkerURL           string  `json:"worker_url"`
			TrackingKeyVersion  float64 `json:"tracking_key_version"`
			TrackingKeyVersions []int   `json:"tracking_key_versions"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dry-run json: %v\n%s", err, out)
	}
	if !payload.DryRun || payload.Request.WorkerURL != "https://example.org" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if payload.Request.TrackingKeyVersion != 2 || len(payload.Request.TrackingKeyVersions) != 2 {
		t.Fatalf("dry-run should report existing rotated versions: %#v", payload.Request)
	}

	explicitOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"--no-input",
				"--json",
				"--dry-run",
				"gmail", "track", "setup",
				"--worker-url", "https://example.net",
				"--tracking-key", "replacement",
			}); err != nil {
				t.Fatalf("explicit dry-run setup: %v", err)
			}
		})
	})
	payload = struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			WorkerURL           string  `json:"worker_url"`
			TrackingKeyVersion  float64 `json:"tracking_key_version"`
			TrackingKeyVersions []int   `json:"tracking_key_versions"`
		} `json:"request"`
	}{}
	if err := json.Unmarshal([]byte(explicitOut), &payload); err != nil {
		t.Fatalf("explicit dry-run json: %v\n%s", err, explicitOut)
	}
	if payload.Request.TrackingKeyVersion != 1 ||
		len(payload.Request.TrackingKeyVersions) != 1 ||
		payload.Request.TrackingKeyVersions[0] != 1 {
		t.Fatalf("explicit replacement dry-run should report reset version 1: %#v", payload.Request)
	}
}

func TestGmailTrackJSONOutputs(t *testing.T) {
	setupTrackingEnv(t)

	setupResult := executeWithTestRuntime(t, []string{"--account", "a@b.com", "--no-input", "--json", "gmail", "track", "setup", "--worker-url", "https://example.com"}, nil)
	if setupResult.err != nil {
		t.Fatalf("setup: %v", setupResult.err)
	}
	if strings.Contains(setupResult.stderr, "TRACKING_KEY=") || strings.Contains(setupResult.stderr, "ADMIN_KEY=") {
		t.Fatalf("json setup should not print manual secrets to stderr: %q", setupResult.stderr)
	}
	var setupPayload map[string]any
	if err := json.Unmarshal([]byte(setupResult.stdout), &setupPayload); err != nil {
		t.Fatalf("setup json: %v\n%s", err, setupResult.stdout)
	}
	if setupPayload["configured"] != true || setupPayload["workerUrl"] != "https://example.com" {
		t.Fatalf("unexpected setup json: %#v", setupPayload)
	}
	if setupPayload["trackingKeySet"] != true || setupPayload["adminConfigured"] != true {
		t.Fatalf("setup json should expose secret presence, not values: %#v", setupPayload)
	}

	statusResult := executeWithTestRuntime(t, []string{"--account", "a@b.com", "--json", "gmail", "track", "status"}, nil)
	if statusResult.err != nil {
		t.Fatalf("status: %v", statusResult.err)
	}
	var statusPayload map[string]any
	if err := json.Unmarshal([]byte(statusResult.stdout), &statusPayload); err != nil {
		t.Fatalf("status json: %v\n%s", err, statusResult.stdout)
	}
	if statusPayload["configured"] != true || statusPayload["trackingKeyVersion"].(float64) != 1 {
		t.Fatalf("unexpected status json: %#v", statusPayload)
	}

	rotateResult := executeWithTestRuntime(t, []string{"--account", "a@b.com", "--no-input", "--json", "gmail", "track", "key", "rotate", "--no-deploy"}, nil)
	if rotateResult.err != nil {
		t.Fatalf("rotate: %v", rotateResult.err)
	}
	var rotatePayload map[string]any
	if err := json.Unmarshal([]byte(rotateResult.stdout), &rotatePayload); err != nil {
		t.Fatalf("rotate json: %v\n%s", err, rotateResult.stdout)
	}
	if rotatePayload["trackingKeyRotated"] != true || rotatePayload["trackingKeyVersion"].(float64) != 2 {
		t.Fatalf("unexpected rotate json: %#v", rotatePayload)
	}
}

func TestGmailTrackKeyRotateNoDeploy(t *testing.T) {
	setupTrackingEnv(t)

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("setup: %v", err)
			}
		})
	})

	rotateOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "key", "rotate", "--no-deploy"}); err != nil {
				t.Fatalf("rotate: %v", err)
			}
		})
	})
	if !strings.Contains(rotateOut, "tracking_key_version\t2") {
		t.Fatalf("unexpected rotate output: %q", rotateOut)
	}
	if !strings.Contains(rotateOut, "tracking_key_versions\t1,2") {
		t.Fatalf("unexpected rotate versions: %q", rotateOut)
	}
	if !strings.Contains(rotateOut, "deployed\tfalse") {
		t.Fatalf("unexpected rotate deployed output: %q", rotateOut)
	}

	statusOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("status: %v", err)
			}
		})
	})
	if !strings.Contains(statusOut, "tracking_key_version\t2") {
		t.Fatalf("missing rotated status key version: %q", statusOut)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("rerun setup: %v", err)
			}
		})
	})

	statusOut = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("status after setup rerun: %v", err)
			}
		})
	})
	if !strings.Contains(statusOut, "tracking_key_version\t2") {
		t.Fatalf("setup rerun lost rotated key version: %q", statusOut)
	}
}

func TestGmailTrackStatus_NotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "configured\tfalse") {
		t.Fatalf("unexpected status output: %q", out)
	}
}

func TestGmailTrackOpens(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/q/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tracking_id": "tid",
				"recipient":   "user@example.com",
				"sent_at":     "2025-01-01T00:00:00Z",
				"total_opens": 2,
				"human_opens": 1,
				"first_human_open": map[string]any{
					"at": "2025-01-01T02:00:00Z",
					"location": map[string]any{
						"city":    "SF",
						"region":  "CA",
						"country": "US",
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/opens"):
			if r.Header.Get("Authorization") != "Bearer adminkey" {
				t.Fatalf("unexpected auth: %q", r.Header.Get("Authorization"))
			}
			if r.URL.Query().Get("recipient") != "user@example.com" {
				t.Fatalf("unexpected recipient: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{
					{
						"tracking_id":  "tid",
						"recipient":    "user@example.com",
						"subject_hash": "hash",
						"sent_at":      "2025-01-01T00:00:00Z",
						"opened_at":    "2025-01-01T01:00:00Z",
						"is_bot":       false,
						"location":     map[string]any{"city": "SF", "region": "CA", "country": "US"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	saveTrackingConfigForTest(t, cfg)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "tid"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "tracking_id\ttid") {
		t.Fatalf("unexpected tracking id output: %q", out)
	}
	if !strings.Contains(out, "first_human_open\t2025-01-01T02:00:00Z") {
		t.Fatalf("unexpected first open output: %q", out)
	}
	if !strings.Contains(out, "first_human_open_location\tSF, CA") {
		t.Fatalf("unexpected first open location output: %q", out)
	}

	adminOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com", "--since", "2025-01-01"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(adminOut, "tid\tuser@example.com") {
		t.Fatalf("unexpected admin output: %q", adminOut)
	}

	if _, err := parseTrackingSince("not-a-date"); err == nil {
		t.Fatalf("expected parseTrackingSince error")
	}
}

func TestGmailTrackOpens_JSON(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/q/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tracking_id": "tid",
				"recipient":   "user@example.com",
				"sent_at":     "2025-01-01T00:00:00Z",
				"total_opens": 2,
				"human_opens": 1,
			})
			return
		case strings.Contains(r.URL.Path, "/opens"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{
					{
						"tracking_id":  "tid",
						"recipient":    "user@example.com",
						"subject_hash": "hash",
						"sent_at":      "2025-01-01T00:00:00Z",
						"opened_at":    "2025-01-01T01:00:00Z",
						"is_bot":       false,
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	saveTrackingConfigForTest(t, cfg)

	trackResult := executeWithTestRuntime(t, []string{"--json", "--account", "a@b.com", "gmail", "track", "opens", "tid"}, nil)
	if trackResult.err != nil {
		t.Fatalf("Execute: %v", trackResult.err)
	}
	if !strings.Contains(trackResult.stdout, "\"tracking_id\"") {
		t.Fatalf("unexpected track json output: %q", trackResult.stdout)
	}

	adminResult := executeWithTestRuntime(t, []string{"--json", "--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com"}, nil)
	if adminResult.err != nil {
		t.Fatalf("Execute: %v", adminResult.err)
	}
	if !strings.Contains(adminResult.stdout, "\"opens\"") {
		t.Fatalf("unexpected admin json output: %q", adminResult.stdout)
	}

	if parsed, err := parseTrackingSince("24h"); err != nil || parsed == "" {
		t.Fatalf("unexpected parseTrackingSince duration result: %q err=%v", parsed, err)
	}
	if parsed, err := parseTrackingSince("2025-01-01"); err != nil || parsed == "" {
		t.Fatalf("unexpected parseTrackingSince date result: %q err=%v", parsed, err)
	}
}

func TestGmailTrackOpens_AdminEmpty(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/opens") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	saveTrackingConfigForTest(t, cfg)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "opens\t0") {
		t.Fatalf("unexpected empty admin output: %q", out)
	}
}

func TestGmailTrackOpens_NotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	cfg := &tracking.Config{Enabled: false}
	saveTrackingConfigForTest(t, cfg)

	if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens"}); err == nil {
		t.Fatalf("expected error for unconfigured tracking")
	}
}

func TestParseTrackingSince_FlexibleFormats(t *testing.T) {
	t.Parallel()

	if parsed, err := parseTrackingSince("2026-02-13T10:20"); err != nil || parsed == "" {
		t.Fatalf("unexpected local datetime parse: %q err=%v", parsed, err)
	}

	parsedNano, err := parseTrackingSince("2026-02-13T10:20:30.123456789Z")
	if err != nil {
		t.Fatalf("unexpected RFC3339Nano parse error: %v", err)
	}
	if !strings.Contains(parsedNano, ".123456789Z") {
		t.Fatalf("expected nano precision output, got %q", parsedNano)
	}
}
