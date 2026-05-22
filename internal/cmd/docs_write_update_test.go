package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsWriteUpdate_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

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

func TestDocsWriteUpdate_Pageless(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--pageless"}, ctx, flags); err != nil {
		t.Fatalf("write pageless: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests after write, got %d", len(batchRequests))
	}
	if got := batchRequests[1]; len(got) != 1 || got[0].UpdateDocumentStyle == nil {
		t.Fatalf("unexpected pageless write request: %#v", got)
	}
	if got := batchRequests[1][0].UpdateDocumentStyle; got.Fields != "documentFormat" || got.DocumentStyle.DocumentFormat.DocumentMode != "PAGELESS" {
		t.Fatalf("unexpected pageless write style request: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "!", "--pageless"}, ctx, flags); err != nil {
		t.Fatalf("update pageless: %v", err)
	}
	if len(batchRequests) != 4 {
		t.Fatalf("expected 4 batch requests after update, got %d", len(batchRequests))
	}
	if got := batchRequests[3]; len(got) != 1 || got[0].UpdateDocumentStyle == nil {
		t.Fatalf("unexpected pageless update request: %#v", got)
	}
	if got := batchRequests[3][0].UpdateDocumentStyle; got.Fields != "documentFormat" || got.DocumentStyle.DocumentFormat.DocumentMode != "PAGELESS" {
		t.Fatalf("unexpected pageless update style request: %#v", got)
	}
}

func TestDocsWrite_PageSizeAndMargins(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"body": map[string]any{
					"content": []any{map[string]any{"startIndex": 1, "endIndex": 2}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	args := []string{"doc1", "--text", "hello", "--page-width=8.5in", "--margin-left=0.5in", "--margin-right=0.5in"}
	if err := runKong(t, &DocsWriteCmd{}, args, ctx, flags); err != nil {
		t.Fatalf("write margins: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected write and style batch requests, got %d", len(batchRequests))
	}
	upd := batchRequests[1][0].UpdateDocumentStyle
	if upd == nil {
		t.Fatalf("expected style update, got %#v", batchRequests[1])
	}
	if upd.Fields != "pageSize.width,marginLeft,marginRight" {
		t.Fatalf("fields = %q", upd.Fields)
	}
	if upd.DocumentStyle.PageSize.Width.Magnitude != 612 {
		t.Fatalf("page width = %#v", upd.DocumentStyle.PageSize.Width)
	}
	if upd.DocumentStyle.MarginLeft.Magnitude != 36 || upd.DocumentStyle.MarginRight.Magnitude != 36 {
		t.Fatalf("margins = left %#v right %#v", upd.DocumentStyle.MarginLeft, upd.DocumentStyle.MarginRight)
	}
}

func TestDocsWrite_InvalidLayoutValueFailsBeforeMutation(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })
	newDocsService = func(context.Context, string) (*docs.Service, error) {
		t.Fatal("invalid layout value should fail before creating Docs service")
		return nil, errors.New("unexpected Docs service creation")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--page-width=bogus"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "invalid --page-width") {
		t.Fatalf("expected invalid page-width error, got %v", err)
	}

	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--page-width=NaN"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "invalid --page-width") {
		t.Fatalf("expected invalid page-width NaN error, got %v", err)
	}
}
