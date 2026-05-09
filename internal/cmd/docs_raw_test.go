package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// fullDocResponse returns a richer Document payload than DocsInfoCmd would
// ever request (DocsInfoCmd restricts via Fields). This proves `raw` drops
// that restriction and exposes the full API tree.
func fullDocResponse(id string) map[string]any {
	return map[string]any{
		"documentId": id,
		"title":      "Full Doc",
		"revisionId": "rev1",
		"body": map[string]any{
			"content": []any{
				map[string]any{
					"startIndex": 1,
					"endIndex":   10,
					"paragraph": map[string]any{
						"elements": []any{
							map[string]any{
								"textRun": map[string]any{
									"content": "hello world\n",
								},
							},
						},
					},
				},
			},
		},
		"namedStyles": map[string]any{
			"styles": []any{map[string]any{"namedStyleType": "NORMAL_TEXT"}},
		},
	}
}

// newDocsRawTestServer builds a test Docs server; if status != 0 it returns
// that status instead of a successful response.
func newDocsRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/documents/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockDocsService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := newDocsService
	t.Cleanup(func() { newDocsService = orig })

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
}

func rawTestContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func TestDocsRaw_HappyPath(t *testing.T) {
	srv := newDocsRawTestServer(t, 0, fullDocResponse("doc1"))
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out)
	}
	// Bare struct: top-level keys must be Document fields, not a wrapper.
	if got["documentId"] != "doc1" {
		t.Fatalf("expected documentId=doc1, got: %v", got["documentId"])
	}
	// The whole point of raw: body.content must be present (info -j drops it).
	body, ok := got["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body object in output, got: %v", got["body"])
	}
	if _, ok := body["content"]; !ok {
		t.Fatalf("expected body.content in raw output")
	}
}

func TestDocsRaw_APIError(t *testing.T) {
	srv := newDocsRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		err := runKong(t, cmd, []string{"doc1"}, ctx, flags)
		if err == nil {
			t.Fatalf("expected error on 500, got nil")
		}
	})
}

func TestDocsRaw_NotFound(t *testing.T) {
	srv := newDocsRawTestServer(t, http.StatusNotFound, nil)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		err := runKong(t, cmd, []string{"doc1"}, ctx, flags)
		if err == nil {
			t.Fatalf("expected error on 404")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected 'not found' in error, got: %v", err)
		}
	})
}

func TestDocsRaw_EmptyDocID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	err := (&DocsRawCmd{}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error on empty docId")
	}
}
