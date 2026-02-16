package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func newDocsServiceForTest(t *testing.T, h http.HandlerFunc) (*docs.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("NewDocsService: %v", err)
	}
	return docSvc, srv.Close
}

func newDocsCmdContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func TestDocsWriteReplace_EmptyBody_NoPanic(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsWriteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "hello", "--replace"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs write --replace: %v", err)
	}

	if len(got.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got.Requests))
	}
	if got.Requests[0].DeleteContentRange != nil {
		t.Fatal("unexpected delete request for empty doc body")
	}
	if got.Requests[0].InsertText == nil || got.Requests[0].InsertText.Text != "hello" {
		t.Fatalf("unexpected insert request: %#v", got.Requests[0])
	}
}

func TestDocsCatAllTabs_PropagatesStdoutError(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocResponse("doc1"))
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close()
	_ = w.Close()
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsCatCmd{}
	err = runKong(t, cmd, []string{"doc1", "--all-tabs"}, newDocsCmdContext(t), flags)
	if err == nil {
		t.Fatal("expected stdout write error")
	}
}

func TestDocsInsert_SendsExpectedRequest(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsInsertCmd{}
	if err := runKong(t, cmd, []string{"doc1", "hello", "--index", "5"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs insert: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].InsertText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	if got.Requests[0].InsertText.Text != "hello" || got.Requests[0].InsertText.Location == nil || got.Requests[0].InsertText.Location.Index != 5 {
		t.Fatalf("unexpected insert payload: %#v", got.Requests[0].InsertText)
	}
}

func TestDocsDelete_SendsExpectedRequest(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsDeleteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--start", "2", "--end", "7"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs delete: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].DeleteContentRange == nil || got.Requests[0].DeleteContentRange.Range == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	rng := got.Requests[0].DeleteContentRange.Range
	if rng.StartIndex != 2 || rng.EndIndex != 7 {
		t.Fatalf("unexpected delete range: %#v", rng)
	}
}

func TestDocsFindReplace_SendsExpectedRequest(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 3}}},
			})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "foo", "bar", "--match-case"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].ReplaceAllText == nil || got.Requests[0].ReplaceAllText.ContainsText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	r := got.Requests[0].ReplaceAllText
	if r.ContainsText.Text != "foo" || !r.ContainsText.MatchCase || r.ReplaceText != "bar" {
		t.Fatalf("unexpected replace payload: %#v", r)
	}
}
