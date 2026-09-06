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

// A symlink is not a directory, so IsDir does not stop it: without an explicit
// check, push would read the link target and upload a file from outside dir.
func TestReadAppScriptDir_RejectsSymlinkedSources(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.gs")
	if err := os.WriteFile(outside, []byte("SECRET"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{"appsscript.json": "{}"})
	if err := os.Symlink(outside, filepath.Join(dir, "Code.gs")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	files, err := readAppScriptDir(dir)
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, file := range files {
		if strings.Contains(file.Source, "SECRET") {
			t.Fatal("symlink target was read into the push payload")
		}
	}
}

func TestReadAppScriptDir_RejectsSymlinkedDirectoryEntry(t *testing.T) {
	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{"appsscript.json": "{}"})
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "Linked.html")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := readAppScriptDir(dir); err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("unexpected err: %v", err)
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

// appScriptContentServer serves GetContent for the given remote files and
// records the UpdateContent payload that follows. The real API is one path with
// two methods, so one handler mirrors it.
type appScriptContentServer struct {
	*httptest.Server

	sent map[string]any
}

func newAppScriptContentServer(t *testing.T, remote ...string) *appScriptContentServer {
	t.Helper()

	files := make([]map[string]any, 0, len(remote))
	for _, name := range remote {
		base, fileType := splitAppScriptTestName(t, name)
		files = append(files, map[string]any{"name": base, "type": fileType, "source": "remote " + name})
	}

	server := &appScriptContentServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "projects/script123/content") {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"scriptId": "script123", "files": files})
			return
		}

		var sent map[string]any
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			// t.Fatalf cannot stop the test from the server goroutine.
			t.Errorf("decode update request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		server.sent = sent
		_ = json.NewEncoder(w).Encode(map[string]any{"scriptId": "script123", "files": sent["files"]})
	}))
	t.Cleanup(server.Close)

	return server
}

// pushedNames returns the editor-visible filenames of the recorded payload.
func (s *appScriptContentServer) pushedNames(t *testing.T) []string {
	t.Helper()

	raw, _ := s.sent["files"].([]any)
	names := make([]string, 0, len(raw))

	for _, item := range raw {
		file, _ := item.(map[string]any)
		name, _ := file["name"].(string)
		fileType, _ := file["type"].(string)
		names = append(names, name+appScriptExtByType[fileType])
	}

	return names
}

func splitAppScriptTestName(t *testing.T, name string) (base, fileType string) {
	t.Helper()

	ext := filepath.Ext(name)
	fileType, ok := appScriptTypeByExt[ext]
	if !ok {
		t.Fatalf("unsupported fixture name %q", name)
	}

	return strings.TrimSuffix(name, ext), fileType
}

// appScriptPushDir is the local side of every push test: a manifest plus one
// source file. No assertion depends on their contents.
func appScriptPushDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeAppScriptFixture(t, dir, map[string]string{
		"appsscript.json": `{"timeZone":"Etc/GMT"}`,
		"Code.gs":         "function doGet() {}",
	})

	return dir
}

func TestExecute_AppScriptPush_SendsDirectoryContents(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Code.gs")

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t),
	}, newAppScriptTestService(t, srv.Server))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	assertSameStrings(t, srv.pushedNames(t), []string{"appsscript.json", "Code.gs"})
	if !strings.Contains(result.stdout, "pushed\ttrue") ||
		!strings.Contains(result.stdout, "file\tCode\tSERVER_JS") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

// The default must never delete: a remote file the directory does not provide
// has to be carried through the whole-project UpdateContent replacement.
func TestExecute_AppScriptPush_KeepsRemoteOnlyFilesByDefault(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Code.gs", "Sidebar.html", "Legacy.gs")

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t),
	}, newAppScriptTestService(t, srv.Server))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	assertSameStrings(t, srv.pushedNames(t), []string{"appsscript.json", "Code.gs", "Sidebar.html", "Legacy.gs"})
	if strings.Contains(result.stdout, "removed\t") {
		t.Fatalf("default push must not report removals: %q", result.stdout)
	}
	if !strings.Contains(result.stderr, "Kept 2 remote-only file(s)") ||
		!strings.Contains(result.stderr, "Sidebar.html") ||
		!strings.Contains(result.stderr, "--prune") {
		t.Fatalf("expected a kept-files hint on stderr, got %q", result.stderr)
	}
}

func TestExecute_AppScriptPush_PruneRemovesRemoteOnlyFiles(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Code.gs", "Sidebar.html")

	result := executeWithAppScriptTestService(t, []string{
		"--force", "--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t), "--prune",
	}, newAppScriptTestService(t, srv.Server))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	assertSameStrings(t, srv.pushedNames(t), []string{"appsscript.json", "Code.gs"})
	if !strings.Contains(result.stdout, "removed\tSidebar.html") {
		t.Fatalf("expected the removal to be reported on stdout: %q", result.stdout)
	}
	// The exact deletion set has to be named before the call that performs it.
	if !strings.Contains(result.stderr, "1 remote file(s) will be deleted") ||
		!strings.Contains(result.stderr, "Sidebar.html") {
		t.Fatalf("expected the removal set on stderr, got %q", result.stderr)
	}
}

// --prune needs the project read before it can say what it would delete, so its
// dry run reports the resolved deletion set instead of just the local files.
func TestExecute_AppScriptPush_PruneDryRunReportsRemovalSet(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Code.gs", "Sidebar.html")

	result := executeWithAppScriptTestService(t, []string{
		"--dry-run", "--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t), "--prune",
	}, newAppScriptTestService(t, srv.Server))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if !strings.Contains(result.stdout, "Dry run: would appscript.push") ||
		!strings.Contains(result.stdout, `"Sidebar.html"`) ||
		!strings.Contains(result.stdout, `"prune": true`) {
		t.Fatalf("dry run did not report the removal set: %q", result.stdout)
	}
	if srv.sent != nil {
		t.Fatalf("dry run must not write content: %#v", srv.sent)
	}
}

// Both lists are always arrays: a consumer should not have to special-case
// null for whichever mode it did not run in.
func TestExecute_AppScriptPush_JSONAlwaysEmitsBothFileLists(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Code.gs", "Sidebar.html")

	result := executeWithAppScriptTestService(t, []string{
		"--json", "--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t),
	}, newAppScriptTestService(t, srv.Server))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"removed", "kept"} {
		if _, ok := parsed[key].([]any); !ok {
			t.Fatalf("%s should be an array, got %#v", key, parsed[key])
		}
	}
	if got, _ := parsed["kept"].([]any); len(got) != 1 || got[0] != "Sidebar.html" {
		t.Fatalf("unexpected kept: %#v", parsed["kept"])
	}
	if got, _ := parsed["removed"].([]any); len(got) != 0 {
		t.Fatalf("default push must remove nothing: %#v", parsed["removed"])
	}
}

func TestExecute_AppScriptPush_PruneRequiresForceWhenNonInteractive(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Sidebar.html")

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t), "--prune",
	}, newAppScriptTestService(t, srv.Server))
	if result.err == nil || !strings.Contains(result.err.Error(), "without --force") {
		t.Fatalf("unexpected err: %v", result.err)
	}
	if srv.sent != nil {
		t.Fatalf("no content should have been written: %#v", srv.sent)
	}
}

// A prune with nothing to remove must not stop for a confirmation.
func TestExecute_AppScriptPush_PruneWithoutRemovalsSkipsConfirmation(t *testing.T) {
	srv := newAppScriptContentServer(t, "appsscript.json", "Code.gs")

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "push", "script123", appScriptPushDir(t), "--prune",
	}, newAppScriptTestService(t, srv.Server))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if srv.sent == nil {
		t.Fatal("expected the push to go through")
	}
}
