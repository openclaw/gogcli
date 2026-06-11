package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsBatchStoreLifecyclePreservesRequestJSON(t *testing.T) {
	dir := t.TempDir()
	store := newDocsBatchStoreAt(dir)
	state, err := store.create(docsBatchState{
		Service:    docsBatchService,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	request := &docs.Request{UpdateTextStyle: &docs.UpdateTextStyleRequest{
		Range:  &docs.Range{StartIndex: 1, EndIndex: 2},
		Fields: "bold",
		TextStyle: &docs.TextStyle{
			Bold:            false,
			ForceSendFields: []string{"Bold"},
		},
	}}
	total, err := store.appendRequests(state.BatchID, "docs.format", "doc1", "user@example.com", "default", "rev1", []*docs.Request{request}, false)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}

	loaded, err := store.get(state.BatchID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !compactJSONContains(t, loaded.Requests[0].Request, `"bold":false`) {
		t.Fatalf("request JSON lost explicit false: %s", loaded.Requests[0].Request)
	}
	if loaded.RequiredRevisionID != "rev1" {
		t.Fatalf("revision = %q, want rev1", loaded.RequiredRevisionID)
	}

	listed, err := store.list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].Requests != 1 {
		t.Fatalf("list = %#v", listed)
	}

	deleted, err := store.delete(state.BatchID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(deleted.Requests) != 1 {
		t.Fatalf("deleted requests = %d, want 1", len(deleted.Requests))
	}
}

func TestDocsBatchStoreRejectsIdentityRevisionAndNonemptyReplace(t *testing.T) {
	store := newDocsBatchStoreAt(t.TempDir())
	state, err := store.create(docsBatchState{
		Service:    docsBatchService,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	request := []*docs.Request{{InsertText: &docs.InsertTextRequest{
		Location: &docs.Location{Index: 1},
		Text:     "x",
	}}}

	if _, err := store.appendRequests(state.BatchID, "docs.insert", "other", "user@example.com", "default", "rev1", request, false); err == nil {
		t.Fatal("expected document mismatch")
	}
	if _, err := store.appendRequests(state.BatchID, "docs.insert", "doc1", "user@example.com", "default", "rev1", request, false); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if _, err := store.appendRequests(state.BatchID, "docs.insert", "doc1", "user@example.com", "default", "rev2", request, false); err == nil {
		t.Fatal("expected revision mismatch")
	}
	if _, err := store.appendRequests(state.BatchID, "docs.write", "doc1", "user@example.com", "default", "rev1", request, true); err == nil {
		t.Fatal("expected nonempty batch rejection")
	}
}

func TestBatchEndAtomicSubmitsExactPayloadAndDeletesState(t *testing.T) {
	store, state := prepareDocsBatchEndTest(t, 1)

	var received docsBatchWireBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents/doc1:batchUpdate" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = fmt.Fprint(w, `{"documentId":"doc1","writeControl":{"requiredRevisionId":"rev2"}}`)
	}))
	defer server.Close()
	restoreDocsBatchHTTPTest(t, server)

	if err := (&BatchEndCmd{BatchID: state.BatchID}).Run(batchTestContext(t), &RootFlags{}); err != nil {
		t.Fatalf("end: %v", err)
	}
	if len(received.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(received.Requests))
	}
	if received.WriteControl == nil || received.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %#v", received.WriteControl)
	}
	if _, err := store.get(state.BatchID); err == nil {
		t.Fatal("completed batch still exists")
	}
}

func TestBatchEndAutoSplitChainsRevision(t *testing.T) {
	store, state := prepareDocsBatchEndTest(t, docsBatchUpdateRequestCap+1)

	var requestCounts []int
	var revisions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body docsBatchWireBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		requestCounts = append(requestCounts, len(body.Requests))
		revision := ""
		if body.WriteControl != nil {
			revision = body.WriteControl.RequiredRevisionId
		}
		revisions = append(revisions, revision)
		next := "rev2"
		if len(requestCounts) == 2 {
			next = "rev3"
		}
		_, _ = fmt.Fprintf(w, `{"documentId":"doc1","writeControl":{"requiredRevisionId":%q}}`, next)
	}))
	defer server.Close()
	restoreDocsBatchHTTPTest(t, server)

	if err := (&BatchEndCmd{BatchID: state.BatchID, AutoSplit: true}).Run(batchTestContext(t), &RootFlags{}); err != nil {
		t.Fatalf("end split: %v", err)
	}
	if fmt.Sprint(requestCounts) != "[500 1]" {
		t.Fatalf("request counts = %v", requestCounts)
	}
	if fmt.Sprint(revisions) != "[rev1 rev2]" {
		t.Fatalf("revisions = %v", revisions)
	}
	if _, err := store.get(state.BatchID); err == nil {
		t.Fatal("completed split batch still exists")
	}
}

func TestBatchEndContinueOnErrorRetainsFailedRequests(t *testing.T) {
	store, state := prepareDocsBatchEndTest(t, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body docsBatchWireBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if len(body.Requests) == 2 || bytes.Contains(body.Requests[0], []byte(`"text":"request-1"`)) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"invalid request","status":"INVALID_ARGUMENT"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"documentId":"doc1","writeControl":{"requiredRevisionId":"rev2"}}`)
	}))
	defer server.Close()
	restoreDocsBatchHTTPTest(t, server)

	if err := (&BatchEndCmd{BatchID: state.BatchID, ContinueOnError: true}).Run(batchTestContext(t), &RootFlags{}); err != nil {
		t.Fatalf("end continue: %v", err)
	}
	loaded, err := store.get(state.BatchID)
	if err != nil {
		t.Fatalf("get retained state: %v", err)
	}
	if len(loaded.Requests) != 1 || !compactJSONContains(t, loaded.Requests[0].Request, `"text":"request-1"`) {
		t.Fatalf("retained requests = %#v", loaded.Requests)
	}
	if loaded.RequiredRevisionID != "rev2" {
		t.Fatalf("revision = %q, want rev2", loaded.RequiredRevisionID)
	}
}

func TestBatchEndDryRunKeepsStateWithoutHTTP(t *testing.T) {
	store, state := prepareDocsBatchEndTest(t, 1)
	oldClient := newDocsBatchHTTPClient
	newDocsBatchHTTPClient = func(context.Context, string) (*http.Client, error) {
		t.Fatal("dry run created HTTP client")
		return nil, errors.New("unexpected HTTP client request")
	}
	t.Cleanup(func() { newDocsBatchHTTPClient = oldClient })

	if err := (&BatchEndCmd{BatchID: state.BatchID}).Run(batchTestContext(t), &RootFlags{DryRun: true}); err != nil {
		t.Fatalf("dry-run end: %v", err)
	}
	if _, err := store.get(state.BatchID); err != nil {
		t.Fatalf("dry run removed state: %v", err)
	}
}

func TestBatchLocalMutatorsHonorDryRun(t *testing.T) {
	restoreHome, err := config.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatalf("set home: %v", err)
	}
	t.Cleanup(restoreHome)

	ctx := batchTestContext(t)
	dryRun := &RootFlags{DryRun: true}
	beginErr := (&BatchBeginCmd{Service: docsBatchService, DocID: "doc1"}).Run(ctx, dryRun)
	if !isSuccessfulDryRunExit(beginErr) {
		t.Fatalf("begin dry run: %v", beginErr)
	}

	store, err := newDocsBatchStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	batches, err := store.list()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("begin dry run created batches: %#v", batches)
	}

	state, err := store.create(docsBatchState{
		Service:    docsBatchService,
		DocumentID: "doc1",
		Account:    "a@b.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	abortErr := (&BatchAbortCmd{BatchID: state.BatchID}).Run(ctx, dryRun)
	if !isSuccessfulDryRunExit(abortErr) {
		t.Fatalf("abort dry run: %v", abortErr)
	}
	if _, err := store.get(state.BatchID); err != nil {
		t.Fatalf("abort dry run removed batch: %v", err)
	}

	pruneErr := (&BatchPruneCmd{OlderThan: time.Nanosecond}).Run(ctx, dryRun)
	if !isSuccessfulDryRunExit(pruneErr) {
		t.Fatalf("prune dry run: %v", pruneErr)
	}
	if _, err := store.get(state.BatchID); err != nil {
		t.Fatalf("prune dry run removed batch: %v", err)
	}
}

func TestDocsInsertPageBreakBatchQueuesWithoutSubmitting(t *testing.T) {
	restoreHome, err := config.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatalf("set home: %v", err)
	}
	t.Cleanup(restoreHome)

	store, err := newDocsBatchStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	state, err := store.create(docsBatchState{
		Service:    docsBatchService,
		DocumentID: "doc1",
		Account:    "a@b.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	originalDocs := newDocsService
	t.Cleanup(func() { newDocsService = originalDocs })
	postCalls := 0
	docService, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = fmt.Fprint(w, `{"documentId":"doc1","revisionId":"rev1"}`)
		case http.MethodPost:
			postCalls++
			http.Error(w, "unexpected submit", http.StatusInternalServerError)
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docService, nil }

	command := &DocsInsertPageBreakCmd{}
	if runErr := runKong(t, command, []string{"doc1", "--index", "7", "--batch", state.BatchID}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); runErr != nil {
		t.Fatalf("queue page break: %v", runErr)
	}
	if postCalls != 0 {
		t.Fatalf("POST calls = %d, want 0", postCalls)
	}
	loaded, err := store.get(state.BatchID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(loaded.Requests) != 1 || !compactJSONContains(t, loaded.Requests[0].Request, `"insertPageBreak"`) {
		t.Fatalf("queued requests = %#v", loaded.Requests)
	}
	if loaded.RequiredRevisionID != "rev1" {
		t.Fatalf("revision = %q, want rev1", loaded.RequiredRevisionID)
	}
}

func TestDocsWriteBatchRejectsMultiPhaseModes(t *testing.T) {
	command := &DocsWriteCmd{}
	err := runKong(t, command, []string{"doc1", "--text", "# title", "--markdown", "--replace", "--batch", "not-used"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "--batch supports plain text") {
		t.Fatalf("error = %v", err)
	}
}

func prepareDocsBatchEndTest(t *testing.T, requestCount int) (*docsBatchStore, *docsBatchState) {
	t.Helper()
	restoreHome, err := config.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatalf("set home: %v", err)
	}
	t.Cleanup(restoreHome)

	store, err := newDocsBatchStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	state, err := store.create(docsBatchState{
		Service:    docsBatchService,
		DocumentID: "doc1",
		Account:    "user@example.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	requests := make([]*docs.Request, 0, requestCount)
	for index := range requestCount {
		requests = append(requests, &docs.Request{InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: int64(index + 1)},
			Text:     fmt.Sprintf("request-%d", index),
		}})
	}
	if _, err := store.appendRequests(state.BatchID, "docs.insert", "doc1", "user@example.com", "default", "rev1", requests, false); err != nil {
		t.Fatalf("append: %v", err)
	}

	return store, state
}

func restoreDocsBatchHTTPTest(t *testing.T, server *httptest.Server) {
	t.Helper()
	oldBaseURL := docsBatchBaseURL
	oldClient := newDocsBatchHTTPClient
	docsBatchBaseURL = server.URL
	newDocsBatchHTTPClient = func(context.Context, string) (*http.Client, error) {
		return server.Client(), nil
	}
	t.Cleanup(func() {
		docsBatchBaseURL = oldBaseURL
		newDocsBatchHTTPClient = oldClient
	})
}

func batchTestContext(t *testing.T) context.Context {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	testUI, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("new UI: %v", err)
	}

	return outfmt.WithMode(ui.WithUI(context.Background(), testUI), outfmt.Mode{})
}

func TestDocsBatchPruneRemovesOnlyStaleState(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := newDocsBatchStoreAt(dir)
	oldNow := docsBatchNow
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	docsBatchNow = func() time.Time { return now.Add(-4 * time.Hour) }
	stale, err := store.create(docsBatchState{Service: docsBatchService, DocumentID: "old", Account: "a", Client: "default"})
	if err != nil {
		t.Fatal(err)
	}
	docsBatchNow = func() time.Time { return now }
	current, err := store.create(docsBatchState{Service: docsBatchService, DocumentID: "new", Account: "a", Client: "default"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { docsBatchNow = oldNow })

	removed, err := store.prune(3 * time.Hour)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(removed) != 1 || removed[0].BatchID != stale.BatchID {
		t.Fatalf("removed = %#v", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, current.BatchID+".json")); err != nil {
		t.Fatalf("current batch missing: %v", err)
	}
}

func TestValidateDocsBatchIDRejectsTraversal(t *testing.T) {
	for _, value := range []string{"../batch", "not-a-uuid", strings.ToUpper("018f47b5-7b5e-7cc0-9a78-4a5bb1886251")} {
		if err := validateDocsBatchID(value); err == nil {
			t.Fatalf("validateDocsBatchID(%q) succeeded", value)
		}
	}
}

func compactJSONContains(t *testing.T, data []byte, value string) bool {
	t.Helper()
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatalf("compact JSON: %v", err)
	}

	return strings.Contains(compact.String(), value)
}

func isSuccessfulDryRunExit(err error) bool {
	var exitErr *ExitError
	return errors.As(err, &exitErr) && exitErr.Code == 0
}
