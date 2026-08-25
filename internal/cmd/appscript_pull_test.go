package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func appScriptPullHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

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

// A pull into a working directory must not quietly discard local edits.
func TestExecute_AppScriptPull_RefusesToOverwriteByDefault(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "Code.gs")
	if err := os.WriteFile(local, []byte("MY LOCAL EDIT"), 0o600); err != nil {
		t.Fatalf("seed local file: %v", err)
	}

	srv := httptest.NewServer(appScriptPullHandler())
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "pull", "script123", dir,
	}, newAppScriptTestService(t, srv))
	if result.err == nil || !strings.Contains(result.err.Error(), "--overwrite") {
		t.Fatalf("unexpected err: %v", result.err)
	}
	if !strings.Contains(result.err.Error(), "Code.gs") {
		t.Fatalf("error should name the clashing file: %v", result.err)
	}

	body, err := os.ReadFile(local)
	if err != nil {
		t.Fatalf("read local file: %v", err)
	}
	if string(body) != "MY LOCAL EDIT" {
		t.Fatalf("local file was modified: %q", body)
	}
	// The refusal has to happen before anything is written, not part-way.
	if _, err := os.Stat(filepath.Join(dir, "appsscript.json")); !os.IsNotExist(err) {
		t.Fatalf("no file should have been written: %v", err)
	}
}

func TestExecute_AppScriptPull_OverwriteReplacesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "Code.gs")
	if err := os.WriteFile(local, []byte("MY LOCAL EDIT"), 0o600); err != nil {
		t.Fatalf("seed local file: %v", err)
	}

	srv := httptest.NewServer(appScriptPullHandler())
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "pull", "script123", dir, "--overwrite",
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	body, err := os.ReadFile(local)
	if err != nil {
		t.Fatalf("read local file: %v", err)
	}
	if string(body) != "function doGet() {}" {
		t.Fatalf("expected the remote source, got %q", body)
	}
}

// A server-supplied name must not be able to escape the target directory.
func TestExecute_AppScriptPull_OverwriteRejectsSymlinkedOutput(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "project")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create project directory: %v", err)
	}
	protected := filepath.Join(base, "protected.txt")
	if err := os.WriteFile(protected, []byte("DO NOT REPLACE"), 0o600); err != nil {
		t.Fatalf("seed protected file: %v", err)
	}
	if err := os.Symlink(protected, filepath.Join(dir, "Code.gs")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
		t.Fatalf("create output symlink: %v", err)
	}

	srv := httptest.NewServer(appScriptPullHandler())
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "pull", "script123", dir, "--overwrite",
	}, newAppScriptTestService(t, srv))
	if result.err == nil || !strings.Contains(result.err.Error(), "non-regular output file") {
		t.Fatalf("unexpected err: %v", result.err)
	}
	content, err := os.ReadFile(protected)
	if err != nil {
		t.Fatalf("read protected file: %v", err)
	}
	if string(content) != "DO NOT REPLACE" {
		t.Fatalf("symlink target was overwritten: %q", content)
	}
	if _, err := os.Stat(filepath.Join(dir, "appsscript.json")); !os.IsNotExist(err) {
		t.Fatalf("no project file should have been written: %v", err)
	}
}

func TestExecute_AppScriptPull_RejectsSanitizedFilenameCollisions(t *testing.T) {
	cases := []struct {
		name      string
		firstName string
		otherName string
	}{
		{name: "sanitized traversal", firstName: "../Code", otherName: "Code"},
		{name: "case-insensitive filesystem", firstName: "Code", otherName: "code"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, "projects/script123/content") {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"scriptId": "script123",
					"files": []map[string]any{
						{"name": "First", "type": "SERVER_JS", "source": "first"},
						{"name": tc.firstName, "type": "SERVER_JS", "source": "original"},
						{"name": tc.otherName, "type": "SERVER_JS", "source": "collision"},
					},
				})
			}))
			defer srv.Close()

			for _, overwrite := range []bool{false, true} {
				t.Run(fmt.Sprintf("overwrite=%t", overwrite), func(t *testing.T) {
					dir := t.TempDir()
					args := []string{"--account", "a@b.com", "appscript", "pull", "script123", dir}
					if overwrite {
						args = append(args, "--overwrite")
					}
					result := executeWithAppScriptTestService(t, args, newAppScriptTestService(t, srv))
					if result.err == nil || !strings.Contains(result.err.Error(), "multiple Apps Script files resolve") {
						t.Fatalf("unexpected err: %v", result.err)
					}
					for _, name := range []string{"First.gs", "Code.gs", "code.gs"} {
						if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
							t.Fatalf("%s should not have been written: %v", name, err)
						}
					}
				})
			}
		})
	}
}

func TestWriteAppScriptFile_RemainsWithinPinnedRootAfterDirectorySwap(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("overwrite=%t", overwrite), func(t *testing.T) {
			base := t.TempDir()
			original := filepath.Join(base, "project")
			outside := filepath.Join(base, "outside")
			for _, dir := range []string{original, outside} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatalf("create directory %s: %v", dir, err)
				}
			}
			protected := filepath.Join(outside, "Code.gs")
			if err := os.WriteFile(protected, []byte("PROTECTED"), 0o600); err != nil {
				t.Fatalf("seed external file: %v", err)
			}
			if overwrite {
				if err := os.WriteFile(filepath.Join(original, "Code.gs"), []byte("old"), 0o600); err != nil {
					t.Fatalf("seed project file: %v", err)
				}
			}

			root, _, _, err := openDriveSyncRoot(original)
			if err != nil {
				t.Fatalf("pin project directory: %v", err)
			}
			defer root.Close()

			moved := filepath.Join(base, "moved")
			if err := os.Rename(original, moved); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("renaming a pinned directory is unavailable: %v", err)
				}
				t.Fatalf("move pinned directory: %v", err)
			}
			if err := os.Symlink(outside, original); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink creation is unavailable: %v", err)
				}
				t.Fatalf("swap directory for symlink: %v", err)
			}

			if err := writeAppScriptFile(root, "Code.gs", "pulled", overwrite); err != nil {
				t.Fatalf("write to pinned directory: %v", err)
			}
			if body, err := os.ReadFile(filepath.Join(moved, "Code.gs")); err != nil || string(body) != "pulled" {
				t.Fatalf("pinned directory missing pulled file: body=%q err=%v", body, err)
			}
			if body, err := os.ReadFile(protected); err != nil || string(body) != "PROTECTED" {
				t.Fatalf("outside file was modified: body=%q err=%v", body, err)
			}
		})
	}
}

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
