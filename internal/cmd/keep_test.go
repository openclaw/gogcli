package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keepapi "google.golang.org/api/keep/v1"

	"github.com/steipete/gogcli/internal/config"
)

func writeKeepSA(t *testing.T) string {
	t.Helper()

	saPath := defaultLayoutForTest(t, config.PathKindData).KeepServiceAccountPath("a@b.com")
	if mkdirErr := os.MkdirAll(filepath.Dir(saPath), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(saPath, []byte("{}"), 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	return saPath
}

func TestGetKeepService_NoServiceAccountConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	called := false
	ctx := withKeepTestServiceFactory(context.Background(), func(context.Context, string, string) (*keepapi.Service, error) {
		called = true
		return &keepapi.Service{}, nil
	})

	_, err := getKeepService(ctx, &RootFlags{Account: "a@b.com"}, &KeepCmd{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
	if called {
		t.Fatalf("expected no service account usage")
	}
}

func TestGetKeepService_UsesStoredServiceAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	saPath := writeKeepSA(t)

	var gotPath, gotImpersonate string
	ctx := withKeepTestServiceFactory(context.Background(), func(ctx context.Context, path, impersonate string) (*keepapi.Service, error) {
		gotPath = path
		gotImpersonate = impersonate
		return &keepapi.Service{}, nil
	})

	svc, err := getKeepService(ctx, &RootFlags{Account: account}, &KeepCmd{})
	if err != nil {
		t.Fatalf("getKeepService: %v", err)
	}
	if svc == nil {
		t.Fatalf("expected service")
	}
	if gotPath != saPath {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotImpersonate != account {
		t.Fatalf("unexpected impersonate: %q", gotImpersonate)
	}
}

func TestKeepInvalidMaxFailsBeforeService(t *testing.T) {
	testCases := [][]string{
		{"--account", "a@b.com", "keep", "list", "--max", "0"},
		{"--account", "a@b.com", "keep", "list", "--max=-1"},
		{"--account", "a@b.com", "keep", "search", "foo", "--max", "0"},
		{"--account", "a@b.com", "keep", "search", "foo", "--max=-1"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[2:], "_"), func(t *testing.T) {
			result := executeWithKeepTestServiceFactory(
				t,
				args,
				unexpectedKeepTestService(t, "expected max validation to fail before creating Keep service"),
			)
			if result.err == nil || ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}

func TestKeepList_Plain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes":
			_, _ = io.WriteString(w, `{"notes":[{"name":"notes/abc","title":"","updateTime":"2026-01-01T00:00:00Z","body":{"text":{"text":"hello\nworld (longer than fifty chars, so it truncates)"}}}],"nextPageToken":"p2"}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"keep", "list", "--plain", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "# More results: use --all/--all-pages to fetch every page, or --page p2 for the next page") {
		t.Fatalf("expected next page hint, got: %q", result.stderr)
	}
	if !strings.Contains(result.stdout, "notes/abc") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "hello world") {
		t.Fatalf("expected snippet, got: %q", result.stdout)
	}
}

func TestKeepList_NoNotes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes":
			_, _ = io.WriteString(w, `{"notes":[],"nextPageToken":""}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"keep", "list", "--plain", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "No notes") {
		t.Fatalf("expected no-notes message, got: %q", result.stderr)
	}
}

func TestKeepList_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes":
			_, _ = io.WriteString(w, `{"notes":[{"name":"notes/abc","title":"T","updateTime":"2026-01-01T00:00:00Z"}],"nextPageToken":"p2"}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"--json", "keep", "list", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var payload struct {
		Notes         []any  `json:"notes"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if len(payload.Notes) != 1 || payload.NextPageToken != "p2" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestKeepGet_Plain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes/abc":
			_, _ = io.WriteString(w, `{"name":"notes/abc","title":"T","createTime":"2026-01-01T00:00:00Z","updateTime":"2026-01-02T00:00:00Z","trashed":false,"body":{"text":{"text":"body"}},"attachments":[{"name":"notes/abc/attachments/att1","mimeType":["text/plain"]}]}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"keep", "get", "abc", "--plain", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "name\tnotes/abc") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "attachments\t1") {
		t.Fatalf("expected attachments, got: %q", result.stdout)
	}
}

func TestKeepGet_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes/abc":
			_, _ = io.WriteString(w, `{"name":"notes/abc","title":"T","createTime":"2026-01-01T00:00:00Z","updateTime":"2026-01-02T00:00:00Z","trashed":false}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"--json", "keep", "get", "abc", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var payload struct {
		Note map[string]any `json:"note"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if payload.Note["name"] != "notes/abc" {
		t.Fatalf("unexpected note: %#v", payload.Note)
	}
}

func TestKeepSearch_Paging(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes":
			if r.URL.Query().Get("pageToken") == "" {
				_, _ = io.WriteString(w, `{"notes":[{"name":"notes/n1","title":"No match","updateTime":"2026-01-01T00:00:00Z","body":{"text":{"text":"zzz"}}}],"nextPageToken":"p2"}`)
				return
			}
			_, _ = io.WriteString(w, `{"notes":[{"name":"notes/n2","title":"","updateTime":"2026-01-01T00:00:00Z","body":{"text":{"text":"hello there"}}}],"nextPageToken":""}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"keep", "search", "hello", "--plain", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "Found 1 notes matching") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
	if !strings.Contains(result.stdout, "notes/n2") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
}

func TestKeepSearch_EmptyQuery(t *testing.T) {
	err := (&KeepSearchCmd{Query: " "}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestKeepSearch_NoMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes":
			_, _ = io.WriteString(w, `{"notes":[{"name":"notes/n1","title":"No match","updateTime":"2026-01-01T00:00:00Z","body":{"text":{"text":"zzz"}}}],"nextPageToken":""}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{"keep", "search", "hello", "--plain", "--account", account}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "No notes matching") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestKeepAttachment_Download(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes/abc/attachments/att1":
			if r.URL.Query().Get("alt") != "media" {
				http.Error(w, "expected alt=media", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, "payload")
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)

	cwd, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("Getwd: %v", getwdErr)
	}
	tmp := t.TempDir()
	if chdirErr := os.Chdir(tmp); chdirErr != nil {
		t.Fatalf("Chdir: %v", chdirErr)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	result := executeWithKeepTestService(t, []string{
		"keep", "attachment", "notes/abc/attachments/att1", "--plain",
		"--account", account, "--mime-type", "text/plain", "--out", "out.bin",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "path\tout.bin") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	b, err := os.ReadFile(filepath.Join(tmp, "out.bin"))
	if err != nil {
		t.Fatalf("read out.bin: %v", err)
	}
	if string(b) != "payload" {
		t.Fatalf("unexpected payload: %q", string(b))
	}
}

func TestKeepAttachment_InvalidName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	err := (&KeepAttachmentCmd{AttachmentName: "nope"}).Run(context.Background(), &RootFlags{Account: account}, &KeepCmd{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestKeepAttachment_DefaultOutAndMkdir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/notes/abc/attachments/att1":
			_, _ = io.WriteString(w, "payload")
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)

	cwd, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("Getwd: %v", getwdErr)
	}
	tmp := t.TempDir()
	if chdirErr := os.Chdir(tmp); chdirErr != nil {
		t.Fatalf("Chdir: %v", chdirErr)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	result := executeWithKeepTestService(t, []string{
		"keep", "attachment", "notes/abc/attachments/att1", "--plain",
		"--account", account, "--mime-type", "text/plain", "--out", "dir/out.bin",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "path\tdir/out.bin") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	if _, err := os.Stat(filepath.Join(tmp, "dir", "out.bin")); err != nil {
		t.Fatalf("expected output file: %v", err)
	}

	result = executeWithKeepTestService(t, []string{
		"keep", "attachment", "notes/abc/attachments/att1", "--plain",
		"--account", account, "--mime-type", "text/plain",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "path\tatt1") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	if _, err := os.Stat(filepath.Join(tmp, "att1")); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
}

func TestGetKeepService_ServiceAccountOverride(t *testing.T) {
	_, err := getKeepService(context.Background(), nil, &KeepCmd{ServiceAccount: "sa.json"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestGetKeepService_ServiceAccountOverride_CallsBuilder(t *testing.T) {
	var gotPath, gotImpersonate string
	ctx := withKeepTestServiceFactory(context.Background(), func(_ context.Context, path, impersonate string) (*keepapi.Service, error) {
		gotPath = path
		gotImpersonate = impersonate
		return &keepapi.Service{}, nil
	})

	_, err := getKeepService(ctx, nil, &KeepCmd{ServiceAccount: "sa.json", Impersonate: "a@b.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "sa.json" || gotImpersonate != "a@b.com" {
		t.Fatalf("unexpected args: path=%q impersonate=%q", gotPath, gotImpersonate)
	}
}

func TestGetKeepService_UsesLegacyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	legacyPath := defaultLayoutForTest(t, config.PathKindConfig).KeepServiceAccountLegacyPath(account)
	if mkdirErr := os.MkdirAll(filepath.Dir(legacyPath), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(legacyPath, []byte("{}"), 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	var gotPath string
	ctx := withKeepTestServiceFactory(context.Background(), func(_ context.Context, path, _ string) (*keepapi.Service, error) {
		gotPath = path
		return &keepapi.Service{}, nil
	})

	_, err := getKeepService(ctx, &RootFlags{Account: account}, &KeepCmd{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != legacyPath {
		t.Fatalf("unexpected path: %q", gotPath)
	}
}

// ---- keep create ----

func TestKeepCreate_TextPlain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			gotBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"name":"notes/new1","title":"My note","createTime":"2026-01-01T00:00:00Z","updateTime":"2026-01-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{
		"keep", "create", "--title", "My note", "--text", "Hello world", "--plain", "--account", account,
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !strings.Contains(result.stdout, "notes/new1") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	if !strings.Contains(string(gotBody), "Hello world") {
		t.Fatalf("expected body text in request, got: %q", string(gotBody))
	}
}

func TestKeepCreate_ListItems(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			gotBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"name":"notes/new2","title":"Checklist","createTime":"2026-01-01T00:00:00Z","updateTime":"2026-01-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{
		"keep", "create", "--title", "Checklist", "--item", "Milk", "--item", "Eggs", "--plain", "--account", account,
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !strings.Contains(result.stdout, "notes/new2") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	body := string(gotBody)
	if !strings.Contains(body, "Milk") || !strings.Contains(body, "Eggs") {
		t.Fatalf("expected list items in request, got: %q", body)
	}
}

func TestKeepCreate_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			_, _ = io.WriteString(w, `{"name":"notes/new3","title":"T","createTime":"2026-01-01T00:00:00Z","updateTime":"2026-01-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{
		"--json", "keep", "create", "--text", "body", "--account", account,
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var payload struct {
		Note map[string]any `json:"note"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if payload.Note["name"] != "notes/new3" {
		t.Fatalf("unexpected note: %#v", payload.Note)
	}
}

func TestKeepCreate_MissingBody(t *testing.T) {
	err := (&KeepCreateCmd{}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error for missing body")
	}
}

func TestKeepCreate_TextAndItemMutuallyExclusive(t *testing.T) {
	err := (&KeepCreateCmd{Text: "hi", Item: []string{"x"}}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error for conflicting flags")
	}
}

func TestKeepCreate_RejectsEmptyItem(t *testing.T) {
	err := (&KeepCreateCmd{Item: []string{"  "}}).Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error for empty item")
	}
}

// ---- keep delete ----

func TestKeepDelete_Plain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/abc" {
			deleted = true
			_, _ = io.WriteString(w, `{}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{
		"keep", "delete", "abc", "--plain", "--account", account, "--force",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !deleted {
		t.Fatalf("expected DELETE request to be made")
	}
	if !strings.Contains(result.stdout, "deleted") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
}

func TestKeepDelete_WithNotesPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/xyz" {
			deleted = true
			_, _ = io.WriteString(w, `{}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{
		"keep", "delete", "notes/xyz", "--plain", "--account", account, "--force",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !deleted {
		t.Fatalf("expected DELETE request for notes/xyz")
	}
}

func TestKeepDelete_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc := newKeepTestServiceFromServer(t, srv)
	result := executeWithKeepTestService(t, []string{
		"--json", "keep", "delete", "abc", "--account", account, "--force",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if payload["deleted"] != true {
		t.Fatalf("expected deleted=true, got: %#v", payload)
	}
}
