package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newDocsServiceForTest(t *testing.T, h http.HandlerFunc) (*docs.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	return docSvc, func() {} // retained for call-site compat; cleanup is via t.Cleanup
}

func newDocsCmdContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func newDocsCmdOutputContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &out, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u), &out
}

func newDocsJSONContext(t *testing.T) context.Context {
	t.Helper()
	return outfmt.WithMode(newDocsCmdContext(t), outfmt.Mode{JSON: true})
}

func docBodyWithText(text string) map[string]any {
	return map[string]any{
		"documentId": "doc1",
		"body": map[string]any{
			"content": []any{
				map[string]any{
					"startIndex":   0,
					"endIndex":     1,
					"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
				},
				map[string]any{
					"startIndex": 1,
					"endIndex":   1 + len(text),
					"paragraph": map[string]any{
						"elements": []any{
							map[string]any{
								"startIndex": 1,
								"endIndex":   1 + len(text),
								"textRun":    map[string]any{"content": text},
							},
						},
					},
				},
			},
		},
	}
}
