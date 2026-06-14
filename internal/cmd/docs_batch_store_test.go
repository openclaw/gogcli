package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/docsbatch"
)

func TestBatchEndAtomicSubmitsExactPayloadAndDeletesState(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, 1)

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
	ctx = withDocsBatchHTTPTest(t, ctx, server)

	if err := (&BatchEndCmd{BatchID: state.BatchID}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("end: %v", err)
	}
	if len(received.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(received.Requests))
	}
	if received.WriteControl == nil || received.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %#v", received.WriteControl)
	}
	if _, err := store.Get(state.BatchID); err == nil {
		t.Fatal("completed batch still exists")
	}
}

func TestBatchEndAutoSplitChainsRevision(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, docsBatchUpdateRequestCap+1)

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
	ctx = withDocsBatchHTTPTest(t, ctx, server)

	if err := (&BatchEndCmd{BatchID: state.BatchID, AutoSplit: true}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("end split: %v", err)
	}
	if fmt.Sprint(requestCounts) != "[500 1]" {
		t.Fatalf("request counts = %v", requestCounts)
	}
	if fmt.Sprint(revisions) != "[rev1 rev2]" {
		t.Fatalf("revisions = %v", revisions)
	}
	if _, err := store.Get(state.BatchID); err == nil {
		t.Fatal("completed split batch still exists")
	}
}

func TestBatchEndAutoSplitPersistsProgressBeforeMissingRevisionError(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, docsBatchUpdateRequestCap+1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"documentId":"doc1"}`)
	}))
	defer server.Close()
	ctx = withDocsBatchHTTPTest(t, ctx, server)

	err := (&BatchEndCmd{BatchID: state.BatchID, AutoSplit: true}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "omitted the revision") {
		t.Fatalf("error = %v", err)
	}

	loaded, err := store.Get(state.BatchID)
	if err != nil {
		t.Fatalf("get retained state: %v", err)
	}
	if len(loaded.Requests) != 1 || !compactJSONContains(t, loaded.Requests[0].Request, `"text":"request-500"`) {
		t.Fatalf("retained requests = %#v", loaded.Requests)
	}
}

func TestBatchEndContinueOnErrorRetainsFailedRequests(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, 2)

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
	ctx = withDocsBatchHTTPTest(t, ctx, server)

	if err := (&BatchEndCmd{BatchID: state.BatchID, ContinueOnError: true}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("end continue: %v", err)
	}
	loaded, err := store.Get(state.BatchID)
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

func TestBatchEndContinueOnErrorPersistsProgressBeforeMissingRevisionError(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, 2)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"invalid request","status":"INVALID_ARGUMENT"}}`)

			return
		}
		_, _ = fmt.Fprint(w, `{"documentId":"doc1"}`)
	}))
	defer server.Close()
	ctx = withDocsBatchHTTPTest(t, ctx, server)

	err := (&BatchEndCmd{BatchID: state.BatchID, ContinueOnError: true}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "omitted the revision") {
		t.Fatalf("error = %v", err)
	}

	loaded, err := store.Get(state.BatchID)
	if err != nil {
		t.Fatalf("get retained state: %v", err)
	}
	if len(loaded.Requests) != 1 || !compactJSONContains(t, loaded.Requests[0].Request, `"text":"request-1"`) {
		t.Fatalf("retained requests = %#v", loaded.Requests)
	}
}

func TestBatchEndContinueOnErrorRequiresRevisionForEarlierFailure(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, 2)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls <= 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"invalid request","status":"INVALID_ARGUMENT"}}`)

			return
		}
		_, _ = fmt.Fprint(w, `{"documentId":"doc1"}`)
	}))
	defer server.Close()
	ctx = withDocsBatchHTTPTest(t, ctx, server)

	err := (&BatchEndCmd{BatchID: state.BatchID, ContinueOnError: true}).Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "omitted the revision") {
		t.Fatalf("error = %v", err)
	}

	loaded, err := store.Get(state.BatchID)
	if err != nil {
		t.Fatalf("get retained state: %v", err)
	}
	if len(loaded.Requests) != 1 || !compactJSONContains(t, loaded.Requests[0].Request, `"text":"request-0"`) {
		t.Fatalf("retained requests = %#v", loaded.Requests)
	}
}

func TestBatchEndDryRunKeepsStateWithoutHTTP(t *testing.T) {
	store, state, ctx := prepareDocsBatchEndTest(t, 1)
	ctx = withDocsTestHTTPClientFactory(ctx, func(context.Context, string) (*http.Client, error) {
		t.Fatal("dry run created HTTP client")
		return nil, errors.New("unexpected HTTP client request")
	})
	statePath, pathErr := store.Path(state.BatchID)
	if pathErr != nil {
		t.Fatalf("state path: %v", pathErr)
	}
	before, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state before dry run: %v", readErr)
	}
	lockPath := filepath.Join(filepath.Dir(statePath), ".lock")
	if removeErr := os.Remove(lockPath); removeErr != nil {
		t.Fatalf("remove setup lock: %v", removeErr)
	}

	if runErr := (&BatchEndCmd{BatchID: state.BatchID}).Run(ctx, &RootFlags{DryRun: true}); runErr != nil {
		t.Fatalf("dry-run end: %v", runErr)
	}
	if _, getErr := store.Get(state.BatchID); getErr != nil {
		t.Fatalf("dry run removed state: %v", getErr)
	}
	after, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state after dry run: %v", readErr)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("dry run changed batch state")
	}
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("dry run created lock: %v", statErr)
	}
}

func TestBatchJSONUsesRuntimeOutput(t *testing.T) {
	var output bytes.Buffer
	ctx := withDocsBatchStateDir(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), t.TempDir())
	if err := (&BatchListCmd{}).Run(ctx); err != nil {
		t.Fatalf("list: %v", err)
	}

	var result struct {
		Batches []docsbatch.Summary `json:"batches"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", output.String(), err)
	}
	if len(result.Batches) != 0 {
		t.Fatalf("batches = %#v, want empty", result.Batches)
	}
}

func TestBatchListUsesRuntimeStateDirWithoutCreatingIt(t *testing.T) {
	runtimeStateDir := t.TempDir()
	ambientStateDir := filepath.Join(t.TempDir(), "ambient")
	t.Setenv("GOG_STATE_DIR", ambientStateDir)

	ctx := withDocsBatchStateDir(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), runtimeStateDir)
	if err := (&BatchListCmd{}).Run(ctx); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeStateDir, "batches")); !os.IsNotExist(err) {
		t.Fatalf("runtime batch directory unexpectedly created: %v", err)
	}
	if _, err := os.Stat(ambientStateDir); !os.IsNotExist(err) {
		t.Fatalf("ambient state directory unexpectedly touched: %v", err)
	}
}

func TestBatchListAndShowReadWithoutLock(t *testing.T) {
	ctx := batchTestContext(t)
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	state, err := store.Create(docsbatch.State{
		Service:    docsbatch.ServiceDocs,
		DocumentID: "doc1",
		Account:    "a@b.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	statePath, pathErr := store.Path(state.BatchID)
	if pathErr != nil {
		t.Fatalf("state path: %v", pathErr)
	}
	before, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state before commands: %v", readErr)
	}
	lockPath := filepath.Join(filepath.Dir(statePath), ".lock")
	if removeErr := os.Remove(lockPath); removeErr != nil {
		t.Fatalf("remove setup lock: %v", removeErr)
	}

	if listErr := (&BatchListCmd{}).Run(ctx); listErr != nil {
		t.Fatalf("list: %v", listErr)
	}
	if showErr := (&BatchShowCmd{BatchID: state.BatchID}).Run(ctx); showErr != nil {
		t.Fatalf("show: %v", showErr)
	}
	after, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state after commands: %v", readErr)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("read command changed batch state")
	}
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("read command created lock: %v", statErr)
	}
}

func TestBatchCommandsValidateBeforeCreatingState(t *testing.T) {
	stateDir := t.TempDir()
	ctx := withDocsBatchStateDir(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), stateDir)

	for name, run := range map[string]func() error{
		"abort": func() error {
			return (&BatchAbortCmd{BatchID: "placeholder"}).Run(ctx, &RootFlags{DryRun: true})
		},
		"show": func() error {
			return (&BatchShowCmd{BatchID: "placeholder"}).Run(ctx)
		},
		"end dry-run": func() error {
			return (&BatchEndCmd{BatchID: "placeholder"}).Run(ctx, &RootFlags{DryRun: true})
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "invalid batch ID: placeholder") {
				t.Fatalf("error = %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(stateDir, "batches")); !os.IsNotExist(statErr) {
				t.Fatalf("batch directory unexpectedly created: %v", statErr)
			}
		})
	}
}

func TestValidateDocsBatchTargetRejectsInvalidIDBeforeAccountResolution(t *testing.T) {
	stateDir := t.TempDir()
	ctx := withDocsBatchStateDir(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), stateDir)

	err := validateDocsBatchTarget(ctx, &RootFlags{}, "placeholder", "doc1")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "invalid batch ID: placeholder") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(stateDir, "batches")); !os.IsNotExist(statErr) {
		t.Fatalf("batch directory unexpectedly created: %v", statErr)
	}
}

func TestBatchLocalMutatorsHonorDryRun(t *testing.T) {
	ctx := batchTestContext(t)
	dryRun := &RootFlags{DryRun: true}
	beginErr := (&BatchBeginCmd{Service: docsbatch.ServiceDocs, DocID: "doc1"}).Run(ctx, dryRun)
	if !isSuccessfulDryRunExit(beginErr) {
		t.Fatalf("begin dry run: %v", beginErr)
	}

	store, err := newDocsBatchStore(ctx)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	batches, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("begin dry run created batches: %#v", batches)
	}

	state, err := store.Create(docsbatch.State{
		Service:    docsbatch.ServiceDocs,
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
	if _, err := store.Get(state.BatchID); err != nil {
		t.Fatalf("abort dry run removed batch: %v", err)
	}

	pruneErr := (&BatchPruneCmd{OlderThan: time.Nanosecond}).Run(ctx, dryRun)
	if !isSuccessfulDryRunExit(pruneErr) {
		t.Fatalf("prune dry run: %v", pruneErr)
	}
	if _, err := store.Get(state.BatchID); err != nil {
		t.Fatalf("prune dry run removed batch: %v", err)
	}
}

func TestDocsInsertPageBreakBatchQueuesWithoutSubmitting(t *testing.T) {
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

	command := &DocsInsertPageBreakCmd{}
	ctx := withDocsTestService(batchTestContext(t), docService)
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	state, err := store.Create(docsbatch.State{
		Service:    docsbatch.ServiceDocs,
		DocumentID: "doc1",
		Account:    "a@b.com",
		Client:     "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if runErr := runKong(t, command, []string{"doc1", "--index", "7", "--batch", state.BatchID}, ctx, &RootFlags{Account: "a@b.com"}); runErr != nil {
		t.Fatalf("queue page break: %v", runErr)
	}
	if postCalls != 0 {
		t.Fatalf("POST calls = %d, want 0", postCalls)
	}
	loaded, err := store.Get(state.BatchID)
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
	err := runKong(t, command, []string{"doc1", "--text", "# title", "--markdown", "--replace", "--batch", "not-used"}, newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "--batch supports plain text") {
		t.Fatalf("error = %v", err)
	}
}

func prepareDocsBatchEndTest(t *testing.T, requestCount int) (*docsbatch.Repository, *docsbatch.State, context.Context) {
	t.Helper()

	ctx := batchTestContext(t)
	store, err := newDocsBatchStore(ctx)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	state, err := store.Create(docsbatch.State{
		Service:    docsbatch.ServiceDocs,
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
	rawRequests, err := marshalDocsBatchRequests(requests)
	if err != nil {
		t.Fatalf("marshal requests: %v", err)
	}
	if _, err := store.Append(docsbatch.AppendOptions{
		BatchID: state.BatchID,
		Command: "docs.insert",
		Identity: docsbatch.Identity{
			Service:    docsbatch.ServiceDocs,
			DocumentID: "doc1",
			Account:    "user@example.com",
			Client:     "default",
		},
		RevisionID: "rev1",
		Requests:   rawRequests,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	return store, state, ctx
}

func withDocsBatchHTTPTest(t *testing.T, ctx context.Context, server *httptest.Server) context.Context {
	t.Helper()
	oldBaseURL := docsBatchBaseURL
	docsBatchBaseURL = server.URL
	t.Cleanup(func() {
		docsBatchBaseURL = oldBaseURL
	})
	return withDocsTestHTTPClientFactory(ctx, func(context.Context, string) (*http.Client, error) {
		return server.Client(), nil
	})
}

func batchTestContext(t *testing.T) context.Context {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	return withDocsBatchStateDir(newCmdRuntimeOutputContext(t, &stdout, &stderr), t.TempDir())
}

func withDocsBatchStateDir(ctx context.Context, stateDir string) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Layout = config.Layout{
			StateDir:      stateDir,
			ExplicitState: true,
		}
	})
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
