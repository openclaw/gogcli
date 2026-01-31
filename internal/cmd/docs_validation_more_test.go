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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsInfo_ValidationAndText(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsInfoCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}

	origNew := newDocsService
	t.Cleanup(func() { newDocsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/documents/doc1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"title":      "Doc",
				"revisionId": "r1",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return svc, nil }

	var outBuf strings.Builder
	u2, uiErr := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx2 := ui.WithUI(context.Background(), u2)

	if err := (&DocsInfoCmd{DocID: "doc1"}).Run(ctx2, flags); err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(outBuf.String(), "revision") {
		t.Fatalf("unexpected output: %q", outBuf.String())
	}
}

func TestDocsCreateCat_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsCreateCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing title error")
	}
	if err := (&DocsCatCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}
}

func TestDocsWriteUpdate_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&DocsWriteCmd{}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}
	if err := (&DocsWriteCmd{DocID: "doc1"}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing text error")
	}
	if err := (&DocsUpdateCmd{}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing docId error")
	}
	if err := (&DocsUpdateCmd{DocID: "doc1"}).Run(ctx, nil, flags); err == nil {
		t.Fatalf("expected missing text error")
	}
}

func TestDocsCat_JSON_EmptyDoc(t *testing.T) {
	origNew := newDocsService
	t.Cleanup(func() { newDocsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/documents/doc1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body":       map[string]any{"content": []map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return svc, nil }

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		if err := (&DocsCatCmd{DocID: "doc1"}).Run(ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if !strings.Contains(out, "\"text\"") {
		t.Fatalf("unexpected json: %q", out)
	}
}
