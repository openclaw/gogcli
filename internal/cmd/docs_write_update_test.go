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

func TestDocsWriteUpdate_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/documents/"), ":batchUpdate")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": id})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"body": map[string]any{
					"content": []any{
						map[string]any{"startIndex": 1, "endIndex": 12},
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

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello"}, ctx, flags); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	if got := batchRequests[0]; len(got) != 2 || got[0].DeleteContentRange == nil || got[1].InsertText == nil {
		t.Fatalf("unexpected write requests: %#v", got)
	}
	if got := batchRequests[0][0].DeleteContentRange.Range; got.StartIndex != 1 || got.EndIndex != 11 {
		t.Fatalf("unexpected delete range: %#v", got)
	}
	if got := batchRequests[0][1].InsertText; got.Location.Index != 1 || got.Text != "hello" {
		t.Fatalf("unexpected insert: %#v", got)
	}

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "world", "--append"}, ctx, flags); err != nil {
		t.Fatalf("write append: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[1]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected append requests: %#v", got)
	}
	if got := batchRequests[1][0].InsertText; got.Location.Index != 11 || got.Text != "world" {
		t.Fatalf("unexpected append insert: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "!"}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(batchRequests) != 3 {
		t.Fatalf("expected 3 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[2]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected update requests: %#v", got)
	}
	if got := batchRequests[2][0].InsertText; got.Location.Index != 11 || got.Text != "!" {
		t.Fatalf("unexpected update insert: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "?", "--index", "5"}, ctx, flags); err != nil {
		t.Fatalf("update index: %v", err)
	}
	if len(batchRequests) != 4 {
		t.Fatalf("expected 4 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[3]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected update index requests: %#v", got)
	}
	if got := batchRequests[3][0].InsertText; got.Location.Index != 5 || got.Text != "?" {
		t.Fatalf("unexpected update index insert: %#v", got)
	}
}
