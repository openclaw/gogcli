package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsWriteUpdate_FileInput(t *testing.T) {
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
	tmpDir := t.TempDir()

	tmpFile := filepath.Join(tmpDir, "test-input.txt")
	if err := os.WriteFile(tmpFile, []byte("file content"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", tmpFile}, ctx, flags); err != nil {
		t.Fatalf("write with file: %v", err)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	if got := batchRequests[0]; len(got) != 2 || got[0].DeleteContentRange == nil || got[1].InsertText == nil {
		t.Fatalf("unexpected write requests: %#v", got)
	}
	if got := batchRequests[0][1].InsertText; got.Location.Index != 1 || got.Text != "file content" {
		t.Fatalf("unexpected insert from file: got Text=%q, want %q", got.Text, "file content")
	}

	updateFile := filepath.Join(tmpDir, "update-input.txt")
	if err := os.WriteFile(updateFile, []byte("updated text"), 0o600); err != nil {
		t.Fatalf("write update temp file: %v", err)
	}
	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--file", updateFile}, ctx, flags); err != nil {
		t.Fatalf("update with file: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[1]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected update requests: %#v", got)
	}
	if got := batchRequests[1][0].InsertText; got.Location.Index != 11 || got.Text != "updated text" {
		t.Fatalf("unexpected update insert from file: got Text=%q at index %d, want %q at index 11",
			got.Text, got.Location.Index, "updated text")
	}

	appendFile := filepath.Join(tmpDir, "append-input.txt")
	if err := os.WriteFile(appendFile, []byte("appended"), 0o600); err != nil {
		t.Fatalf("write append temp file: %v", err)
	}
	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", appendFile, "--append"}, ctx, flags); err != nil {
		t.Fatalf("write append with file: %v", err)
	}
	if len(batchRequests) != 3 {
		t.Fatalf("expected 3 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[2]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected append requests: %#v", got)
	}
	if got := batchRequests[2][0].InsertText; got.Location.Index != 11 || got.Text != "appended" {
		t.Fatalf("unexpected append insert from file: got Text=%q at index %d, want %q at index 11",
			got.Text, got.Location.Index, "appended")
	}

	indexFile := filepath.Join(tmpDir, "index-input.txt")
	if err := os.WriteFile(indexFile, []byte("at index 5"), 0o600); err != nil {
		t.Fatalf("write index temp file: %v", err)
	}
	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--file", indexFile, "--index", "5"}, ctx, flags); err != nil {
		t.Fatalf("update with file and index: %v", err)
	}
	if len(batchRequests) != 4 {
		t.Fatalf("expected 4 batch requests, got %d", len(batchRequests))
	}
	if got := batchRequests[3]; len(got) != 1 || got[0].InsertText == nil {
		t.Fatalf("unexpected update index requests: %#v", got)
	}
	if got := batchRequests[3][0].InsertText; got.Location.Index != 5 || got.Text != "at index 5" {
		t.Fatalf("unexpected update index insert from file: got Text=%q at index %d, want %q at index 5",
			got.Text, got.Location.Index, "at index 5")
	}
}

func TestDocsWriteUpdate_FileInputErrors(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", "/nonexistent/path/file.txt"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got: %v", err)
	}

	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if writeErr := os.WriteFile(emptyFile, []byte(""), 0o600); writeErr != nil {
		t.Fatalf("write empty temp file: %v", writeErr)
	}
	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--file", emptyFile}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
	if !strings.Contains(err.Error(), "empty text") {
		t.Fatalf("expected 'empty text' error, got: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if writeErr := os.WriteFile(testFile, []byte("content"), 0o600); writeErr != nil {
		t.Fatalf("write test temp file: %v", writeErr)
	}
	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--file", testFile}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for both --text and --file, got nil")
	}
	if !strings.Contains(err.Error(), "use only one of --text or --file") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}

	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--markdown"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--markdown requires --replace or --append") {
		t.Fatalf("expected markdown mode error, got: %v", err)
	}

	err = runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--append", "--replace"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--append cannot be combined with --replace") {
		t.Fatalf("expected append replace error, got: %v", err)
	}
}
