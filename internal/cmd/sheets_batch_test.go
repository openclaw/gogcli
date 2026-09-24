package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const sheetsBatchMetadataFixture = `{"sheets":[{"properties":{"sheetId":0,"title":"Data","index":0,"gridProperties":{"rowCount":100,"columnCount":26}},"basicFilter":{"range":{"sheetId":0,"startRowIndex":0,"endRowIndex":5,"startColumnIndex":0,"endColumnIndex":2}},"charts":[{"chartId":101}],"bandedRanges":[{"bandedRangeId":11},{"bandedRangeId":22}],"tables":[{"tableId":"22","name":"ExistingTable","range":{"sheetId":0,"startRowIndex":0,"endRowIndex":5,"startColumnIndex":0,"endColumnIndex":2}}]},{"properties":{"sheetId":7,"title":"Other","index":1,"gridProperties":{"rowCount":100,"columnCount":26}}}],"namedRanges":[{"namedRangeId":"nr1","name":"ExistingRange","range":{"sheetId":0,"startRowIndex":0,"endRowIndex":5,"startColumnIndex":0,"endColumnIndex":2}}]}`

type sheetsPersistedHarness struct {
	runtime      *app.Runtime
	store        *docsbatch.Repository
	reads        atomic.Int32
	failGet      atomic.Bool
	malformedGet atomic.Bool
	posts        []sheetsBatchRequestBody
	post         func(http.ResponseWriter, *http.Request, sheetsBatchRequestBody)
}

func newSheetsPersistedHarness(t *testing.T) *sheetsPersistedHarness {
	t.Helper()
	setTestConfigHome(t)
	stateDir := t.TempDir()
	t.Setenv("GOG_STATE_DIR", stateDir)
	h := &sheetsPersistedHarness{store: newDocsBatchStoreAt(filepath.Join(stateDir, "batches"))}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			h.reads.Add(1)
			if r.URL.Path != "/v4/spreadsheets/sheet1" || r.URL.Query().Get("fields") != sheetsBatchMetadataFields || r.URL.Query().Get("includeGridData") == "true" {
				t.Errorf("unexpected metadata read: %s", r.URL)
			}
			if h.failGet.Load() {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":{"code":403,"message":"metadata unavailable"}}`)
				return
			}
			if h.malformedGet.Load() {
				_, _ = io.WriteString(w, `{"sheets":[{"properties":{"sheetId":"invalid"}}]}`)
				return
			}
			_, _ = io.WriteString(w, sheetsBatchMetadataFixture)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v4/spreadsheets/sheet1:batchUpdate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
		}
		if len(raw) != 1 || raw["requests"] == nil {
			t.Errorf("unexpected wire body (Sheets has no writeControl): %v", raw)
		}
		var body sheetsBatchRequestBody
		if err := json.Unmarshal(raw["requests"], &body.Requests); err != nil {
			t.Error(err)
		}
		h.posts = append(h.posts, body)
		if h.post != nil {
			h.post(w, r, body)
			return
		}
		_, _ = io.WriteString(w, `{"spreadsheetId":"sheet1","replies":[]}`)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		clonedURL := *request.URL
		clonedURL.Scheme, clonedURL.Host = target.Scheme, target.Host
		clone.URL = &clonedURL
		return http.DefaultTransport.RoundTrip(clone)
	})
	h.runtime = &app.Runtime{Services: app.Services{
		Sheets: func(context.Context, string) (*sheets.Service, error) {
			t.Error("queued command constructed a live SDK client")
			return nil, errors.New("unexpected SDK client")
		},
		SheetsHTTP: func(ctx context.Context, account string) (*http.Client, error) {
			if account != "test@example.com" || authclient.ClientOverrideFromContext(ctx) != "default" {
				t.Errorf("lost bound identity: %s %s", account, authclient.ClientOverrideFromContext(ctx))
			}
			return &http.Client{Transport: googleapi.NewRetryTransport(transport)}, nil
		},
	}}
	return h
}

func (h *sheetsPersistedHarness) run(t *testing.T, args ...string) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, append([]string{"--account", "test@example.com", "--client", "default", "--no-input", "--json"}, args...), h.runtime)
}

func (h *sheetsPersistedHarness) begin(t *testing.T) string {
	t.Helper()
	result := h.run(t, "batch", "begin", "--spreadsheet", "sheet1")
	if result.err != nil {
		t.Fatal(result.err)
	}
	var state docsbatch.State
	if err := json.Unmarshal([]byte(result.stdout), &state); err != nil {
		t.Fatal(err)
	}
	if state.Service != docsbatch.ServiceSheets || state.SpreadsheetID != "sheet1" || state.DocumentID != "" || state.PresentationID != "" || state.RequiredRevisionID != "" || len(state.SheetsBaseMetadata) != 0 {
		t.Fatalf("unexpected initial state: %+v", state)
	}
	return state.BatchID
}

func sheetsPersistedCommands() [][]string {
	return [][]string{
		{"format", "sheet1", "Data!A1:B2", "--format-json", `{"textFormat":{"bold":false}}`},
		{"number-format", "sheet1", "Data!B2:B5", "--type", "NUMBER"},
		{"conditional-format", "add", "sheet1", "Data!A1:B5", "--type", "not-blank", "--format-json", `{"textFormat":{"bold":false}}`},
		{"merge", "sheet1", "Data!D1:E1"},
		{"unmerge", "sheet1", "Data!D1:E1"},
		{"freeze", "sheet1", "--sheet", "Data", "--rows", "0"},
		{"resize-columns", "sheet1", "Data!A:B", "--width", "120"},
		{"resize-rows", "sheet1", "Data!1:5", "--auto"},
		{"filter", "set", "sheet1", "Data!A1:B5"},
		{"update-note", "sheet1", "Data!A1", "--note", ""},
		{"links", "set", "sheet1", "Data!C1", "https://example.com", "Example"},
		{"chart", "create", "sheet1", "--spec-json", `{"title":"Queued","basicChart":{"chartType":"COLUMN"}}`, "--sheet", "Data", "--anchor", "A1"},
		{"chart", "update", "sheet1", "101", "--spec-json", `{"title":"Updated","basicChart":{"chartType":"COLUMN"}}`},
		{"chart", "delete", "sheet1", "101"},
		{"named-ranges", "add", "sheet1", "NewRange", "Data!A1:B5"},
		{"named-ranges", "update", "sheet1", "ExistingRange", "--name", "RenamedRange"},
		{"named-ranges", "delete", "sheet1", "ExistingRange"},
		{"add-tab", "sheet1", "NewTab"},
		{"rename-tab", "sheet1", "Other", "Renamed"},
		{"delete-tab", "sheet1", "Other"},
		{"find-replace", "sheet1", "before", "after", "--sheet", "Data"},
		{"banding", "set", "sheet1", "Data!D1:E5"},
		{"banding", "clear", "sheet1", "--sheet", "Data", "--all"},
		{"table", "create", "sheet1", "Data!H1:I5", "--name", "NewTable", "--columns-json", `[{"columnName":"Label","columnType":"TEXT"},{"columnName":"Value","columnType":"DOUBLE"}]`},
		{"table", "delete", "sheet1", "ExistingTable", "--discard-data"},
		{"insert", "sheet1", "Data", "rows", "1"},
	}
}

func TestSheetsPersistedBatchQueuesEverySupportedCommand(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	for _, command := range sheetsPersistedCommands() {
		args := append([]string{"sheets"}, command...)
		result := h.run(t, append(args, "--batch", id)...)
		if result.err != nil || !strings.Contains(result.stdout, `"queued"`) {
			t.Fatalf("%v: %v; %s", command, result.err, result.stdout)
		}
		for _, claimed := range []string{`"deleted"`, `"occurrences_changed"`, `"chartId"`, `"sheetId"`, `"tableId"`} {
			if strings.Contains(result.stdout, claimed) {
				t.Errorf("queue output claims applied state %s: %s", claimed, result.stdout)
			}
		}
	}
	state, err := h.store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if h.reads.Load() != 1 || len(h.posts) != 0 || len(state.Requests) != 27 || state.RequiredRevisionID != "" {
		t.Fatalf("reads=%d posts=%d queued=%d revision=%q", h.reads.Load(), len(h.posts), len(state.Requests), state.RequiredRevisionID)
	}
	for index, want := range map[int]string{0: `"bold":false`, 5: `"frozenRowCount":0`, 9: `"note":""`, 23: `"bandedRangeId":22`} {
		if !compactJSONContains(t, state.Requests[index].Request, want) {
			t.Errorf("request %d missing %s: %s", index, want, state.Requests[index].Request)
		}
	}
	if result := h.run(t, "--force", "batch", "end", id); result.err != nil {
		t.Fatal(result.err)
	}
	if len(h.posts) != 1 || len(h.posts[0].Requests) != 27 {
		t.Fatalf("unexpected submissions: %+v", h.posts)
	}
	if _, err := h.store.Get(id); !errors.Is(err, docsbatch.ErrNotFound) {
		t.Fatalf("completed batch still exists: %v", err)
	}
}

func TestSheetsPersistedBatch120CommandsReuseBaseMetadata(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	for i := 0; i < 120; i++ {
		result := h.run(t, "sheets", "format", "sheet1", "Data!A1:B5", "--format-json", `{"textFormat":{"bold":false}}`, "--batch", id)
		if result.err != nil {
			t.Fatalf("append %d: %v", i, result.err)
		}
		// A subsequent live lookup would now fail; the captured base must be used.
		h.failGet.Store(true)
	}
	state, err := h.store.Get(id)
	if err != nil || len(state.Requests) != 120 || h.reads.Load() != 1 || len(h.posts) != 0 {
		t.Fatalf("error=%v reads=%d posts=%d state=%+v", err, h.reads.Load(), len(h.posts), state)
	}
	if result := h.run(t, "sheets", "format", "sheet1", "NotInBase!A1", "--format-json", `{"textFormat":{"bold":true}}`, "--batch", id); result.err == nil || !strings.Contains(result.err.Error(), "unknown sheet") {
		t.Fatalf("unknown captured title accepted: %v", result.err)
	}
	if h.reads.Load() != 1 {
		t.Fatal("missing base title triggered a refresh")
	}
}

func TestSheetsPersistedBatchRawOnlyIsOfflineAndLossless(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	const requests = `[{"futureRequest":{"zero":0,"false":false,"null":null,"empty":"","integer":9007199254740993}}]`
	result := h.run(t, "sheets", "batch-request", "sheet1", "--requests-json", requests, "--batch", id)
	if result.err != nil {
		t.Fatal(result.err)
	}
	state, err := h.store.Get(id)
	if err != nil || len(state.SheetsBaseMetadata) != 0 || h.reads.Load() != 0 {
		t.Fatalf("raw append captured metadata: %v %+v", err, state)
	}
	for _, command := range [][]string{{"batch", "show", id}, {"--dry-run", "batch", "end", id}} {
		result := h.run(t, command...)
		if result.err != nil || !strings.Contains(result.stdout, "9007199254740993") || strings.Contains(result.stdout, "writeControl") {
			t.Fatalf("preview lost wire data: %v %s", result.err, result.stdout)
		}
	}
	if result := h.run(t, "--force", "batch", "end", id); result.err != nil {
		t.Fatal(result.err)
	}
	if len(h.posts) != 1 || string(h.posts[0].Requests[0]) != requests[1:len(requests)-1] || h.reads.Load() != 0 {
		t.Fatalf("raw wire changed: %+v", h.posts)
	}
}

func TestSheetsPersistedBatchConcurrentCaptureAndFailure(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	ctx := withTestClientResolver(app.WithRuntime(context.Background(), h.runtime))
	ctx, err := prepareSheetsBatch(ctx, &RootFlags{Account: "test@example.com", Client: "default"}, id, "sheet1", "sheets.format")
	if err != nil {
		t.Fatal(err)
	}
	h.failGet.Store(true)
	if _, captureErr := sheetsBatchFromContext(ctx).metadata(ctx); captureErr == nil {
		t.Fatal("expected capture failure")
	}
	state, err := h.store.Get(id)
	if err != nil || len(state.Requests) != 0 || len(state.SheetsBaseMetadata) != 0 {
		t.Fatalf("failed capture altered batch: %v %+v", err, state)
	}
	h.failGet.Store(false)
	h.malformedGet.Store(true)
	if _, captureErr := sheetsBatchFromContext(ctx).metadata(ctx); captureErr == nil {
		t.Fatal("expected malformed metadata failure")
	}
	state, err = h.store.Get(id)
	if err != nil || len(state.Requests) != 0 || len(state.SheetsBaseMetadata) != 0 {
		t.Fatalf("malformed response poisoned cache: %v %+v", err, state)
	}
	h.malformedGet.Store(false)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			if _, captureErr := sheetsBatchFromContext(ctx).metadata(ctx); captureErr != nil {
				t.Error(captureErr)
			}
		})
	}
	wg.Wait()
	state, err = h.store.Get(id)
	if err != nil || h.reads.Load() != 3 || len(state.Requests) != 0 || len(state.SheetsBaseMetadata) == 0 {
		t.Fatalf("capture deleted empty batch or repeated GET: %v reads=%d state=%+v", err, h.reads.Load(), state)
	}
	if persistErr := h.store.WithState(id, func(transaction *docsbatch.Transaction) error {
		transaction.State().SheetsBaseMetadata = json.RawMessage(`{"sheets":[{"properties":{"sheetId":"invalid"}}]}`)
		return transaction.Persist()
	}); persistErr != nil {
		t.Fatal(persistErr)
	}
	if _, captureErr := sheetsBatchFromContext(ctx).metadata(ctx); captureErr == nil || h.reads.Load() != 3 {
		t.Fatalf("corrupt cache refreshed or succeeded: %v reads=%d", captureErr, h.reads.Load())
	}
}

func TestSheetsPersistedBatchDryRunsDoNotCaptureOrAppend(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	before, err := os.ReadFile(h.storePath(t, id))
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range sheetsPersistedCommands() {
		args := append([]string{"--dry-run", "sheets"}, command...)
		immediate := h.run(t, args...)
		if immediate.err != nil || strings.Contains(immediate.stdout, `"batch_id"`) || strings.Contains(immediate.stdout, `"batch"`) {
			t.Fatalf("non-batch preview changed: %v %s", immediate.err, immediate.stdout)
		}
		result := h.run(t, append(args, "--batch", id)...)
		if result.err != nil || !strings.Contains(result.stdout, `"dry_run": true`) || !strings.Contains(result.stdout, `"batch_id": "`+id+`"`) || strings.Contains(result.stdout, `"batch"`) {
			t.Fatalf("%v: %v %s", command, result.err, result.stdout)
		}
		invalid := h.run(t, append(args, "--batch", "invalid")...)
		if ExitCode(invalid.err) != 2 || invalid.stdout != "" {
			t.Fatalf("invalid batch preview succeeded: %v %s", invalid.err, invalid.stdout)
		}
	}
	after, err := os.ReadFile(h.storePath(t, id))
	if err != nil || string(before) != string(after) || h.reads.Load() != 0 || len(h.posts) != 0 {
		t.Fatalf("dry run changed state: %v reads=%d posts=%d", err, h.reads.Load(), len(h.posts))
	}
}

func (h *sheetsPersistedHarness) storePath(t *testing.T, id string) string {
	t.Helper()
	path, err := h.store.Path(id)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSheetsPersistedBatchStopsUncertainRecovery(t *testing.T) {
	for _, uncertain := range []string{"server", "decode", "network"} {
		t.Run(uncertain, func(t *testing.T) {
			h := newSheetsPersistedHarness(t)
			id := h.begin(t)
			requests := `[ {"addSheet":{"properties":{"title":"invalid"}}}, {"addSheet":{"properties":{"title":"success"}}}, {"addSheet":{"properties":{"title":"uncertain"}}}, {"addSheet":{"properties":{"title":"must-not-run"}}} ]`
			if result := h.run(t, "sheets", "batch-request", "sheet1", "--requests-json", requests, "--batch", id); result.err != nil {
				t.Fatal(result.err)
			}
			h.post = func(w http.ResponseWriter, r *http.Request, body sheetsBatchRequestBody) {
				if len(body.Requests) > 1 || strings.Contains(string(body.Requests[0]), "invalid") {
					w.WriteHeader(400)
					_, _ = io.WriteString(w, `{"error":{"code":400,"message":"invalid request"}}`)
					return
				}
				if strings.Contains(string(body.Requests[0]), "uncertain") {
					switch uncertain {
					case "server":
						w.WriteHeader(503)
					case "decode":
						_, _ = io.WriteString(w, `{} garbage`)
					case "network":
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
					}
					return
				}
				_, _ = io.WriteString(w, `{}`)
			}
			result := h.run(t, "--force", "batch", "end", id, "--continue-on-error")
			if result.err == nil || !strings.Contains(result.err.Error(), "inspect the spreadsheet") || len(h.posts) != 4 {
				t.Fatalf("error=%v posts=%d", result.err, len(h.posts))
			}
			state, err := h.store.Get(id)
			if err != nil || len(state.Requests) != 3 || strings.Contains(string(state.Requests[0].Request), "success") {
				t.Fatalf("incorrect retained queue: %v %+v", err, state)
			}
			for i, title := range []string{"invalid", "uncertain", "must-not-run"} {
				if !strings.Contains(string(state.Requests[i].Request), title) {
					t.Errorf("retained request %d: %s", i, state.Requests[i].Request)
				}
			}
		})
	}
}

func TestSheetsPersistedBatchSplitDoesNotRequireRevision(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	requests := make([]json.RawMessage, 501)
	for i := range requests {
		requests[i] = json.RawMessage(fmt.Sprintf(`{"addSheet":{"properties":{"title":"Tab %d"}}}`, i))
	}
	if _, err := h.store.Append(docsbatch.AppendOptions{BatchID: id, Identity: docsbatch.Identity{Service: docsbatch.ServiceSheets, SpreadsheetID: "sheet1", Account: "test@example.com", Client: "default"}, Requests: requests}); err != nil {
		t.Fatal(err)
	}
	if result := h.run(t, "--force", "batch", "end", id); ExitCode(result.err) != 2 || len(h.posts) != 0 {
		t.Fatalf("unexpected oversized atomic submission: %v", result.err)
	}
	if result := h.run(t, "--force", "batch", "end", id, "--auto-split"); result.err != nil {
		t.Fatal(result.err)
	}
	if len(h.posts) != 2 || len(h.posts[0].Requests) != 500 || len(h.posts[1].Requests) != 1 {
		t.Fatalf("unexpected split lengths: %+v", h.posts)
	}
}
