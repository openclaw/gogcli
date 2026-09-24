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

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/googleapi"
)

type slidesBatchHarness struct {
	runtime  *app.Runtime
	store    *docsbatch.Repository
	reads    int
	bodies   []slidesBatchWireBody
	revision string
	post     func(http.ResponseWriter, *http.Request, slidesBatchWireBody)
}

func newSlidesBatchHarness(t *testing.T) *slidesBatchHarness {
	t.Helper()
	setTestConfigHome(t)
	stateDir := t.TempDir()
	t.Setenv("GOG_STATE_DIR", stateDir)
	h := &slidesBatchHarness{store: newDocsBatchStoreAt(filepath.Join(stateDir, "batches")), revision: "rev1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			h.reads++
			if r.URL.Path != "/v1/presentations/deck1" || r.URL.Query().Get("fields") != "revisionId" {
				t.Errorf("unexpected read: %s", r.URL)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revisionId": h.revision})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/presentations/deck1:batchUpdate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		var body slidesBatchWireBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		h.bodies = append(h.bodies, body)
		if h.post != nil {
			h.post(w, r, body)
			return
		}
		_, _ = fmt.Fprintf(w, `{"writeControl":{"requiredRevisionId":"rev%d"}}`, len(h.bodies)+1)
	}))
	t.Cleanup(server.Close)
	previousURL := slidesBatchBaseURL
	slidesBatchBaseURL = server.URL
	t.Cleanup(func() { slidesBatchBaseURL = previousURL })
	svc, err := slides.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	h.runtime = &app.Runtime{Services: app.Services{
		Slides: func(context.Context, string) (*slides.Service, error) { return svc, nil },
		SlidesHTTP: func(ctx context.Context, account string) (*http.Client, error) {
			if account != "test@example.com" || authclient.ClientOverrideFromContext(ctx) != "default" {
				t.Errorf("submission lost bound identity: account=%q client=%q", account, authclient.ClientOverrideFromContext(ctx))
			}
			return &http.Client{Transport: googleapi.NewRetryTransport(http.DefaultTransport)}, nil
		},
	}}
	return h
}

func (h *slidesBatchHarness) run(t *testing.T, args ...string) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, append([]string{"--account", "test@example.com", "--client", "default", "--force", "--no-input", "--json"}, args...), h.runtime)
}

func (h *slidesBatchHarness) begin(t *testing.T) string {
	t.Helper()
	result := h.run(t, "batch", "begin", "--presentation", "deck1")
	if result.err != nil {
		t.Fatal(result.err)
	}
	var state docsbatch.State
	if err := json.Unmarshal([]byte(result.stdout), &state); err != nil {
		t.Fatal(err)
	}
	if state.Service != docsbatch.ServiceSlides || state.PresentationID != "deck1" || state.DocumentID != "" || state.RequiredRevisionID != "" {
		t.Fatalf("unexpected state: %+v", state)
	}
	return state.BatchID
}

func TestSlidesBatchQueuesDependentObjectsAndSubmitsLosslessly(t *testing.T) {
	h := newSlidesBatchHarness(t)
	batchID := h.begin(t)
	if h.reads != 0 {
		t.Fatal("begin contacted Google")
	}
	commands := [][]string{
		{"slides", "element", "create-shape", "deck1", "slide1", "--object-id", "shape1"},
		{"slides", "element", "style", "deck1", "shape1", "--fill-color", "#3367d6"},
		{"slides", "insert-text", "deck1", "shape1", "Caption"},
		{"slides", "style-text", "deck1", "shape1", "--range", "0:7", "--no-bold"},
		{"slides", "element", "transform", "deck1", "shape1", "--translate-x", "0"},
		{"slides", "table", "create", "deck1", "slide1", "--object-id", "table1", "--rows", "2", "--cols", "2"},
		{"slides", "table", "column", "size", "deck1", "table1", "--col", "0", "--width", "120"},
		{"slides", "insert-text", "deck1", "table1", "Cell", "--row", "0", "--col", "0"},
		{"slides", "table", "cell", "style", "deck1", "table1", "--row", "0", "--col", "0", "--fill-color", "#ffffff", "--no-bold"},
		{"slides", "paragraph-style", "deck1", "table1", "--row", "0", "--col", "0", "--space-above", "0"},
		{"slides", "element", "delete", "deck1", "shape1"},
	}
	for _, command := range commands {
		result := h.run(t, append(command, "--batch", batchID)...)
		if result.err != nil {
			t.Fatalf("%v: %v", command, result.err)
		}
		if !strings.Contains(result.stdout, `"queued"`) || strings.Contains(result.stdout, `"deleted": true`) {
			t.Fatalf("incorrect queued result: %s", result.stdout)
		}
	}
	state, err := h.store.Get(batchID)
	if err != nil {
		t.Fatal(err)
	}
	if h.reads != 1 || len(h.bodies) != 0 || len(state.Requests) != 12 {
		t.Fatalf("reads=%d writes=%d queued=%d", h.reads, len(h.bodies), len(state.Requests))
	}
	for index, want := range map[int]string{
		0: `"objectId":"shape1"`, 3: `"bold":false`, 4: `"translateX":0`, 5: `"objectId":"table1"`,
		7: `"cellLocation":{"columnIndex":0,"rowIndex":0}`, 10: `"magnitude":0`,
	} {
		if !compactJSONContains(t, state.Requests[index].Request, want) {
			t.Errorf("request %d missing %s: %s", index, want, state.Requests[index].Request)
		}
	}
	if result := h.run(t, "batch", "end", batchID); result.err != nil {
		t.Fatal(result.err)
	}
	if len(h.bodies) != 1 || h.bodies[0].WriteControl.RequiredRevisionId != "rev1" || len(h.bodies[0].Requests) != len(state.Requests) {
		t.Fatalf("unexpected submission: %+v", h.bodies)
	}
	for i, raw := range h.bodies[0].Requests {
		var stored bytes.Buffer
		if err := json.Compact(&stored, state.Requests[i].Request); err != nil {
			t.Fatal(err)
		}
		if string(raw) != stored.String() {
			t.Errorf("wire request changed: %s != %s", raw, state.Requests[i].Request)
		}
	}
	if _, err := h.store.Get(batchID); !errors.Is(err, docsbatch.ErrNotFound) {
		t.Fatalf("batch not removed: %v", err)
	}
}

func TestSlidesBatchGeneratedIDsAndQueueOnlyValidation(t *testing.T) {
	h := newSlidesBatchHarness(t)
	batchID := h.begin(t)
	result := h.run(t, "slides", "new-slide", "deck1", "--batch", batchID)
	if result.err != nil {
		t.Fatal(result.err)
	}
	var slide struct {
		ObjectID string `json:"slideObjectId"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &slide); err != nil || slide.ObjectID == "" {
		t.Fatalf("missing slide ID: %s; %v", result.stdout, err)
	}
	result = h.run(t, "slides", "table", "create", "deck1", slide.ObjectID, "--rows", "2", "--cols", "2", "--batch", batchID)
	if result.err != nil {
		t.Fatal(result.err)
	}
	var table struct {
		ObjectID string `json:"tableObjectId"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &table); err != nil || table.ObjectID == "" {
		t.Fatalf("missing table ID: %s; %v", result.stdout, err)
	}
	commands := [][]string{
		{"slides", "table", "row", "insert", "deck1", table.ObjectID, "--row", "1"},
		{"slides", "table", "row", "size", "deck1", table.ObjectID, "--row", "2", "--height", "0"},
		{"slides", "table", "column", "insert", "deck1", table.ObjectID, "--col", "1"},
		{"slides", "table", "merge", "deck1", table.ObjectID, "--row", "0", "--col", "0", "--col-span", "3"},
		{"slides", "table", "unmerge", "deck1", table.ObjectID, "--row", "0", "--col", "0", "--col-span", "3"},
		{"slides", "table", "border", "style", "deck1", table.ObjectID, "--row", "0", "--col", "0", "--col-span", "3", "--border-color", "#000000"},
		{"slides", "table", "row", "delete", "deck1", table.ObjectID, "--row", "2"},
		{"slides", "table", "column", "delete", "deck1", table.ObjectID, "--col", "2"},
	}
	for _, command := range commands {
		if result := h.run(t, append(command, "--batch", batchID)...); result.err != nil {
			t.Fatalf("%v: %v", command, result.err)
		}
	}
	if h.reads != 1 || len(h.bodies) != 0 {
		t.Fatalf("unexpected remote work: reads=%d writes=%d", h.reads, len(h.bodies))
	}
}

func TestSlidesBatchBindingAndMissingRevision(t *testing.T) {
	for _, mismatch := range []string{"account", "client", "target", "service", "revision"} {
		t.Run(mismatch, func(t *testing.T) {
			h := newSlidesBatchHarness(t)
			batchID := h.begin(t)
			args := []string{"--account", "test@example.com", "--client", "default", "slides", "insert-text", "deck1", "shape1", "text", "--batch", batchID}
			switch mismatch {
			case "account":
				args[1] = "other@example.com"
			case "client":
				args[3] = "other"
			case "target":
				args[6] = "other-deck"
			case "service":
				if err := h.store.WithState(batchID, func(tx *docsbatch.Transaction) error {
					tx.State().Service = docsbatch.ServiceDocs
					tx.State().Requests = []docsbatch.RequestEntry{{Request: json.RawMessage(`{}`)}}
					return tx.PersistOrDelete()
				}); err != nil {
					t.Fatal(err)
				}
			case "revision":
				h.revision = ""
			}
			result := executeWithTestRuntime(t, args, h.runtime)
			if result.err == nil || len(h.bodies) != 0 {
				t.Fatalf("expected refusal: error=%v writes=%d", result.err, len(h.bodies))
			}
			if mismatch != "revision" && h.reads != 0 {
				t.Fatal("identity mismatch read Google")
			}
		})
	}
}

func TestSlidesBatchDryRunAndFailedSubmissionRetainState(t *testing.T) {
	for _, mode := range []string{"dry-run", "400", "503", "redirect", "readonly"} {
		t.Run(mode, func(t *testing.T) {
			h := newSlidesBatchHarness(t)
			batchID := h.begin(t)
			if result := h.run(t, "slides", "insert-text", "deck1", "shape1", "text", "--batch", batchID); result.err != nil {
				t.Fatal(result.err)
			}
			statePath, err := h.store.Path(batchID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			h.post = func(w http.ResponseWriter, r *http.Request, _ slidesBatchWireBody) {
				switch mode {
				case "400":
					w.WriteHeader(http.StatusBadRequest)
				case "503":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "redirect":
					w.Header().Set("Location", r.URL.Path)
					w.WriteHeader(http.StatusTemporaryRedirect)
				default:
					t.Error("preview/read-only mode submitted a mutation")
				}
				_, _ = fmt.Fprint(w, `{"error":{"message":"rejected"}}`)
			}
			args := []string{"batch", "end", batchID}
			if mode == "dry-run" || mode == "readonly" {
				args = append(args, "--"+mode)
			}
			result := h.run(t, args...)
			if mode == "dry-run" && result.err != nil {
				t.Fatal(result.err)
			}
			if mode != "dry-run" && result.err == nil {
				t.Fatal("expected submission failure")
			}
			after, err := os.ReadFile(statePath)
			if err != nil || string(before) != string(after) {
				t.Fatalf("retained state changed: %v", err)
			}
			wantWrites := 1
			if mode == "dry-run" || mode == "readonly" {
				wantWrites = 0
			}
			if len(h.bodies) != wantWrites {
				t.Fatalf("writes=%d, want %d", len(h.bodies), wantWrites)
			}
		})
	}
}

func TestSlidesBatchReplacementRejectedBeforeInput(t *testing.T) {
	input := &batchInputReader{}
	err := executeWithRuntime([]string{"slides", "insert-text", "deck1", "shape1", "-", "--replace", "--batch", "01900000-0000-7000-8000-000000000000"}, &app.Runtime{IO: app.IO{In: input, Out: io.Discard, Err: io.Discard}})
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--replace cannot") || input.read {
		t.Fatalf("error=%v inputRead=%t", err, input.read)
	}
}
