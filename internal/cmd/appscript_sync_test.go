package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAppScriptFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestReadAppScriptDir_MapsExtensionsToTypes(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{
		"appsscript.json": `{"timeZone":"Etc/GMT"}`,
		"Code.gs":         "function doGet() {}",
		"Helper.js":       "function helper() {}",
		"Index.html":      "<p>hi</p>",
		"README.md":       "ignored",
	})

	files, err := readAppScriptDir(dir)
	if err != nil {
		t.Fatalf("readAppScriptDir: %v", err)
	}

	got := map[string]string{}
	for _, file := range files {
		got[file.Name] = file.Type
	}

	want := map[string]string{
		"appsscript": "JSON",
		"Code":       "SERVER_JS",
		"Helper":     "SERVER_JS",
		"Index":      "HTML",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected files: %#v", got)
	}
	for name, fileType := range want {
		if got[name] != fileType {
			t.Fatalf("%s: got type %q, want %q", name, got[name], fileType)
		}
	}
}

// os.ReadDir sorts entries, so the pushed order is stable without an extra sort.
func TestReadAppScriptDir_OrdersFilesByName(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{
		"appsscript.json": "{}",
		"Zebra.gs":        "",
		"Alpha.gs":        "",
	})

	files, err := readAppScriptDir(dir)
	if err != nil {
		t.Fatalf("readAppScriptDir: %v", err)
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Name)
	}
	if strings.Join(names, ",") != "Alpha,Zebra,appsscript" {
		t.Fatalf("unexpected order: %v", names)
	}
}

func TestReadAppScriptDir_SkipsNonManifestJSON(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{
		"appsscript.json":   "{}",
		"Code.gs":           "",
		"package.json":      `{"name":"local"}`,
		"package-lock.json": `{"lockfileVersion":3}`,
	})

	files, err := readAppScriptDir(dir)
	if err != nil {
		t.Fatalf("readAppScriptDir: %v", err)
	}

	for _, file := range files {
		if file.Type == "JSON" && file.Name != appScriptManifest {
			t.Fatalf("unexpected JSON file pushed: %q", file.Name)
		}
	}
	if len(files) != 2 {
		t.Fatalf("expected manifest + Code, got %d files", len(files))
	}
}

func TestReadAppScriptDir_RequiresManifest(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{"Code.gs": "function doGet() {}"})

	if _, err := readAppScriptDir(dir); err == nil ||
		!strings.Contains(err.Error(), "must contain appsscript.json") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReadAppScriptDir_RejectsEmptyDir(t *testing.T) {
	if _, err := readAppScriptDir(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "no .gs/.js/.html/appsscript.json files found") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestExecute_AppScriptPush_SendsDirectoryContents(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{
		"appsscript.json": `{"timeZone":"Etc/GMT"}`,
		"Code.gs":         "function doGet() {}",
	})

	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "projects/script123/content") {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scriptId": "script123",
			"files": []map[string]any{
				{"name": "appsscript", "type": "JSON"},
				{"name": "Code", "type": "SERVER_JS"},
			},
		})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "push", "script123", dir,
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	sent, _ := received["files"].([]any)
	if len(sent) != 2 {
		t.Fatalf("expected 2 files in request, got %#v", received["files"])
	}
	if !strings.Contains(result.stdout, "pushed\ttrue") ||
		!strings.Contains(result.stdout, "file\tCode\tSERVER_JS") {
		t.Fatalf("unexpected out=%q", result.stdout)
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
	if perm := info.Mode().Perm(); perm != 0o600 {
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

func TestExecute_AppScriptPush_DryRunSkipsService(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{
		"appsscript.json": "{}",
		"Code.gs":         "",
	})

	result := executeWithAppScriptTestServiceFactory(t, []string{
		"--dry-run", "--account", "a@b.com",
		"appscript", "push", "script123", dir,
	}, unexpectedAppScriptTestService(t, "dry-run should not create appscript service"))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "Dry run: would appscript.push") ||
		!strings.Contains(result.stdout, `"Code.gs"`) {
		t.Fatalf("unexpected dry-run out=%q", result.stdout)
	}
}
