package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecute_AppScriptPull_WritesFilesWithPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "projects/script123/content") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scriptId": "script123",
			"files": []map[string]any{
				{"name": "Code", "type": "SERVER_JS", "source": "function doGet() {}"},
				{"name": "appsscript", "type": "JSON", "source": "{}"},
			},
		})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "pull", "script123", dir,
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	body, err := os.ReadFile(filepath.Join(dir, "Code.gs"))
	if err != nil {
		t.Fatalf("read pulled file: %v", err)
	}
	if string(body) != "function doGet() {}" {
		t.Fatalf("unexpected body: %q", body)
	}

	info, err := os.Stat(filepath.Join(dir, "appsscript.json"))
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	// Windows has no POSIX permission bits, so Go reports 0666 for any
	// writable file there.
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Fatalf("expected 0600 manifest, got %o", perm)
	}
}

// A server-supplied name must not be able to escape the target directory.
func TestExecute_AppScriptPull_ContainsTraversalNames(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "out")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "projects/script123/content") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scriptId": "script123",
			"files": []map[string]any{
				{"name": "../escaped", "type": "SERVER_JS", "source": "nope"},
			},
		})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "pull", "script123", dir,
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if _, err := os.Stat(filepath.Join(base, "escaped.gs")); !os.IsNotExist(err) {
		t.Fatalf("file escaped the target directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.gs")); err != nil {
		t.Fatalf("expected sanitized name inside dir: %v", err)
	}
}
