package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

func tabExportDocResponse() map[string]any {
	return map[string]any{
		"documentId": "doc1",
		"title":      "Multi-Tab Doc",
		"tabs": []any{
			map[string]any{
				"tabProperties": map[string]any{"tabId": "t.abc", "title": "First Tab", "index": 0},
			},
			map[string]any{
				"tabProperties": map[string]any{"tabId": "t.def", "title": "Second Tab", "index": 1},
			},
		},
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func stubTabExportDeps(t *testing.T, exportHandler http.Handler) {
	t.Helper()

	origDocs := newDocsService
	origHTTP := newDocsHTTPClient
	t.Cleanup(func() {
		newDocsService = origDocs
		newDocsHTTPClient = origHTTP
	})

	docsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tabExportDocResponse())
	}))
	t.Cleanup(docsSrv.Close)

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(docsSrv.Client()),
		option.WithEndpoint(docsSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(_ context.Context, _ string) (*docs.Service, error) {
		return docSvc, nil
	}

	newDocsHTTPClient = func(_ context.Context, _ string) (*http.Client, error) {
		return &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				rec := httptest.NewRecorder()
				exportHandler.ServeHTTP(rec, req)
				return rec.Result(), nil
			}),
		}, nil
	}
}

func TestTabExportFormatParam(t *testing.T) {
	tests := []struct {
		format  string
		want    string
		wantErr bool
	}{
		{"pdf", "pdf", false},
		{"PDF", "pdf", false},
		{"docx", "docx", false},
		{"txt", "txt", false},
		{"md", "markdown", false},
		{"html", "html", false},
		{"csv", "", true},
		{"xlsx", "", true},
	}
	for _, tt := range tests {
		got, err := tabExportFormatParam(tt.format)
		if tt.wantErr {
			if err == nil {
				t.Errorf("tabExportFormatParam(%q): expected error", tt.format)
			}
			continue
		}
		if err != nil {
			t.Errorf("tabExportFormatParam(%q): %v", tt.format, err)
		} else if got != tt.want {
			t.Errorf("tabExportFormatParam(%q) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestGoogleExportRedirectPolicy(t *testing.T) {
	ctx := context.Background()
	origReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://docs.google.com/export", nil)

	tests := []struct {
		name    string
		target  string
		wantErr string
	}{
		{"same host", "https://docs.google.com/other", ""},
		{"googleusercontent", "https://doc-04-0k-docstext.googleusercontent.com/export/abc", ""},
		{"googleapis", "https://storage.googleapis.com/bucket/file", ""},
		{"google sign-in", "https://accounts.google.com/v3/signin/identifier", "Google sign-in host"},
		{"non-google host", "https://evil.example.com/steal", "non-Google host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, tt.target, nil)
			err := googleExportRedirectPolicy(req, []*http.Request{origReq})
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected success, got: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}

	via := make([]*http.Request, 10)
	for i := range via {
		via[i], _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://docs.google.com/r", nil)
	}
	nextReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://docs.google.com/r", nil)
	err := googleExportRedirectPolicy(nextReq, via)
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("expected too many redirects error, got: %v", err)
	}
}

func TestDocsTabExportURL(t *testing.T) {
	got := docsTabExportURL("DOC123", "pdf", "t.abc")
	want := "https://docs.google.com/document/d/DOC123/export?format=pdf&tab=t.abc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeFilenameComponent(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"t.abc", "t.abc"},
		{"My Budget Sheet", "My_Budget_Sheet"},
		{"tab/with\\bad:chars", "tab_with_bad_chars"},
	}
	for _, tt := range tests {
		if got := sanitizeFilenameComponent(tt.in); got != tt.want {
			t.Errorf("sanitizeFilenameComponent(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveTabID(t *testing.T) {
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tabExportDocResponse())
	}))
	defer cleanup()

	tests := []struct {
		query   string
		wantID  string
		wantErr string
	}{
		{"Second Tab", "t.def", ""},
		{"t.abc", "t.abc", ""},
		{"Nonexistent", "", "tab not found"},
	}
	for _, tt := range tests {
		tabID, err := resolveTabID(context.Background(), docSvc, "doc1", tt.query)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("resolveTabID(%q): got err=%v, want containing %q", tt.query, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveTabID(%q): %v", tt.query, err)
		} else if tabID != tt.wantID {
			t.Errorf("resolveTabID(%q) = %q, want %q", tt.query, tabID, tt.wantID)
		}
	}
}

func TestRunDocsTabExport(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		tabQuery string
		respCT   string
		respBody string
		wantBody string
		wantErr  string
	}{
		{
			name:     "pdf success",
			format:   "pdf",
			tabQuery: "First Tab",
			respCT:   "application/pdf",
			respBody: "exported PDF content",
			wantBody: "exported PDF content",
		},
		{
			name:     "markdown format",
			format:   "md",
			tabQuery: "t.abc",
			respCT:   "text/markdown",
			respBody: "# Markdown",
			wantBody: "# Markdown",
		},
		{
			name:     "unsupported format",
			format:   "csv",
			tabQuery: "t.abc",
			wantErr:  "does not support format",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == "" {
				stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", tt.respCT)
					_, _ = w.Write([]byte(tt.respBody))
				}))
			}

			outPath := filepath.Join(t.TempDir(), "output."+tt.format)
			ctx := newDocsCmdContext(t)
			flags := &RootFlags{Account: "test@example.com"}

			err := runDocsTabExport(ctx, flags, tabExportParams{
				DocID:    "doc1",
				OutFlag:  outPath,
				Format:   tt.format,
				TabQuery: tt.tabQuery,
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("runDocsTabExport: %v", err)
			}
			if tt.wantBody != "" {
				data, readErr := os.ReadFile(outPath)
				if readErr != nil {
					t.Fatalf("read output: %v", readErr)
				}
				if string(data) != tt.wantBody {
					t.Errorf("output = %q, want %q", string(data), tt.wantBody)
				}
			}
		})
	}
}

func TestRunDocsTabExport_HTMLRedirectGuard(t *testing.T) {
	stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "Text/HTML; charset=utf-8")
		_, _ = w.Write([]byte("<html>Sign in</html>"))
	}))

	ctx := newDocsCmdContext(t)
	err := runDocsTabExport(ctx, &RootFlags{Account: "test@example.com"}, tabExportParams{
		DocID:    "doc1",
		OutFlag:  filepath.Join(t.TempDir(), "out.pdf"),
		Format:   "pdf",
		TabQuery: "First Tab",
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected text/html") {
		t.Fatalf("expected HTML redirect error, got: %v", err)
	}
}

func TestRunDocsTabExport_OutStdout(t *testing.T) {
	stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("tab text\n"))
	}))
	t.Chdir(t.TempDir())

	ctx := newDocsCmdContext(t)
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := runDocsTabExport(ctx, &RootFlags{Account: "test@example.com"}, tabExportParams{
				DocID:    "doc1",
				OutFlag:  "-",
				Format:   "txt",
				TabQuery: "First Tab",
			})
			if err != nil {
				t.Fatalf("runDocsTabExport: %v", err)
			}
		})
	})
	if stdout != "tab text\n" {
		t.Fatalf("stdout=%q, want raw export bytes", stdout)
	}
	if _, statErr := os.Stat("-"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file named -, stat=%v", statErr)
	}
}

func TestRunDocsTabExport_OutStdoutJSONRejected(t *testing.T) {
	called := false
	stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("tab text\n"))
	}))

	ctx := newDocsJSONContext(t)
	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := runDocsTabExport(ctx, &RootFlags{Account: "test@example.com"}, tabExportParams{
				DocID:    "doc1",
				OutFlag:  "-",
				Format:   "txt",
				TabQuery: "First Tab",
			})
			if err == nil || !strings.Contains(err.Error(), "can't combine --json with --out -") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})
	if stdout != "" {
		t.Fatalf("stdout=%q, want empty", stdout)
	}
	if called {
		t.Fatal("export request should not be called")
	}
}

func TestRunDocsTabExport_HTTPErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr string
	}{
		{"forbidden", http.StatusForbidden, "tab export failed"},
		{"unauthorized", http.StatusUnauthorized, "re-authenticate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("error body"))
			}))

			ctx := newDocsCmdContext(t)
			err := runDocsTabExport(ctx, &RootFlags{Account: "test@example.com"}, tabExportParams{
				DocID:    "doc1",
				OutFlag:  filepath.Join(t.TempDir(), "out.pdf"),
				Format:   "pdf",
				TabQuery: "First Tab",
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestRunDocsTabExport_JSONOutput(t *testing.T) {
	stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("pdf bytes"))
	}))

	outPath := filepath.Join(t.TempDir(), "output.pdf")
	ctx := newDocsJSONContext(t)

	raw := captureStdout(t, func() {
		err := runDocsTabExport(ctx, &RootFlags{Account: "test@example.com"}, tabExportParams{
			DocID:    "doc1",
			OutFlag:  outPath,
			Format:   "pdf",
			TabQuery: "First Tab",
		})
		if err != nil {
			t.Fatalf("runDocsTabExport: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("json decode: %v (raw=%q)", err, raw)
	}
	if _, ok := result["path"]; !ok {
		t.Errorf("JSON output missing 'path' key: %v", result)
	}
}

func TestDocsExportCmd_TabRouting(t *testing.T) {
	stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("tab pdf"))
	}))

	outPath := filepath.Join(t.TempDir(), "out.pdf")
	ctx := newDocsCmdContext(t)

	cmd := &DocsExportCmd{
		DocID:  "doc1",
		Format: "pdf",
		Tab:    "Second Tab",
		Output: OutputPathFlag{Path: outPath},
	}

	if err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"}); err != nil {
		t.Fatalf("DocsExportCmd.Run: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	if string(data) != "tab pdf" {
		t.Errorf("output = %q, want %q", string(data), "tab pdf")
	}
}

func TestDocsExportCmd_TabEmptyDocID(t *testing.T) {
	ctx := newDocsCmdContext(t)
	cmd := &DocsExportCmd{DocID: "", Tab: "some-tab"}
	err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err == nil || !strings.Contains(err.Error(), "empty docId") {
		t.Fatalf("expected empty docId error, got: %v", err)
	}
}

func TestTabExportOutPath(t *testing.T) {
	tests := []struct {
		name     string
		outFlag  string
		docID    string
		tabQuery string
		format   string
		wantBase string
	}{
		{"tab ID in filename", "", "doc123", "t.abc", "pdf", "doc123_t.abc.pdf"},
		{"markdown extension", "", "doc1", "t.xyz", "md", "doc1_t.xyz.md"},
		{"tab title sanitized", "", "doc1", "My Budget Sheet", "pdf", "doc1_My_Budget_Sheet.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			path, err := tabExportOutPath(tt.outFlag, tt.docID, tt.tabQuery, tt.format)
			if err != nil {
				t.Fatalf("tabExportOutPath: %v", err)
			}
			if got := filepath.Base(path); got != tt.wantBase {
				t.Errorf("base = %q, want %q", got, tt.wantBase)
			}
		})
	}
}

func TestTabExportOutPath_ExplicitPath(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "custom.pdf")
	path, err := tabExportOutPath(outPath, "doc1", "t.abc", "pdf")
	if err != nil {
		t.Fatalf("tabExportOutPath: %v", err)
	}
	if path != outPath {
		t.Errorf("got %q, want %q", path, outPath)
	}
}

func TestTabExportOutPath_DirectoryOutput(t *testing.T) {
	tmpDir := t.TempDir()
	path, err := tabExportOutPath(tmpDir, "doc1", "t.abc", "pdf")
	if err != nil {
		t.Fatalf("tabExportOutPath: %v", err)
	}
	if filepath.Dir(path) != tmpDir {
		t.Errorf("expected file in %q, got %q", tmpDir, filepath.Dir(path))
	}
}

func TestDriveDownloadCmd_TabRouting(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"explicit format", "pdf"},
		{"defaults to pdf", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubTabExportDeps(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/pdf")
				_, _ = w.Write([]byte("tab pdf"))
			}))

			ctx := newDocsCmdContext(t)
			cmd := &DriveDownloadCmd{
				FileID: "doc1",
				Tab:    "First Tab",
				Format: tt.format,
				Output: OutputPathFlag{Path: filepath.Join(t.TempDir(), "out.pdf")},
			}

			if err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"}); err != nil {
				t.Fatalf("DriveDownloadCmd.Run: %v", err)
			}
		})
	}
}

func TestDriveDownloadCmd_TabUnsupportedFormat(t *testing.T) {
	ctx := newDocsCmdContext(t)
	cmd := &DriveDownloadCmd{
		FileID: "doc1",
		Tab:    "First Tab",
		Format: "csv",
		Output: OutputPathFlag{Path: filepath.Join(t.TempDir(), "out.csv")},
	}

	err := cmd.Run(ctx, &RootFlags{Account: "test@example.com"})
	if err == nil || !strings.Contains(err.Error(), "--tab limits export formats") {
		t.Fatalf("expected --tab format restriction error, got: %v", err)
	}
}
