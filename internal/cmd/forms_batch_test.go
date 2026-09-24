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
	"sync"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/docsbatch"
	"github.com/openclaw/gogcli/internal/googleapi"
)

type formsBatchHarness struct {
	runtime  *app.Runtime
	store    *docsbatch.Repository
	reads    int
	revision string
	bodies   []formsBatchWireBody
	post     func(http.ResponseWriter, *http.Request)
}

func newFormsBatchHarness(t *testing.T) *formsBatchHarness {
	t.Helper()
	setTestConfigHome(t)
	stateDir := t.TempDir()
	t.Setenv("GOG_STATE_DIR", stateDir)
	h := &formsBatchHarness{store: newDocsBatchStoreAt(filepath.Join(stateDir, "batches")), revision: "rev1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			h.reads++
			if r.URL.Path != "/v1/forms/form1" || r.URL.Query().Get("fields") != "revisionId,items(itemId)" {
				t.Errorf("unexpected read: %s", r.URL)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revisionId": h.revision, "items": []any{}})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/forms/form1:batchUpdate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		var body formsBatchWireBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		h.bodies = append(h.bodies, body)
		if h.post != nil {
			h.post(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"writeControl":{"requiredRevisionId":"rev2"}}`)
	}))
	t.Cleanup(server.Close)
	previousURL := formsBatchBaseURL
	formsBatchBaseURL = server.URL
	t.Cleanup(func() { formsBatchBaseURL = previousURL })
	svc, err := formsapi.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	h.runtime = &app.Runtime{Services: app.Services{
		Forms: func(context.Context, string) (*formsapi.Service, error) { return svc, nil },
		FormsHTTP: func(ctx context.Context, account string) (*http.Client, error) {
			if account != "test@example.com" || authclient.ClientOverrideFromContext(ctx) != "default" {
				t.Errorf("lost bound identity: %q %q", account, authclient.ClientOverrideFromContext(ctx))
			}
			return &http.Client{Transport: googleapi.NewRetryTransport(http.DefaultTransport)}, nil
		},
	}}
	return h
}

func (h *formsBatchHarness) run(t *testing.T, args ...string) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, append([]string{"--account", "test@example.com", "--client", "default", "--force", "--no-input", "--json"}, args...), h.runtime)
}

func (h *formsBatchHarness) begin(t *testing.T) string {
	t.Helper()
	result := h.run(t, "batch", "begin", "--form", "https://docs.google.com/forms/d/form1/edit")
	if result.err != nil {
		t.Fatal(result.err)
	}
	var state docsbatch.State
	if err := json.Unmarshal([]byte(result.stdout), &state); err != nil {
		t.Fatal(err)
	}
	if state.FormID != "form1" || state.Service != docsbatch.ServiceForms || state.RequiredRevisionID != "" || state.InitialFormItems != nil {
		t.Fatalf("unexpected new batch: %+v", state)
	}
	return state.BatchID
}

func TestFormsBatchOrderedProjectionAndLosslessSubmit(t *testing.T) {
	h := newFormsBatchHarness(t)
	batchID := h.begin(t)
	if h.reads != 0 {
		t.Fatal("begin read Google")
	}
	commands := [][]string{
		{"forms", "add-question", "form1", "--title", "A"},
		{"forms", "questions", "add", "form1", "--title", "B"},
		{"forms", "add-question", "form1", "--title", "C", "--index", "0"},
		{"forms", "questions", "move", "form1", "0", "2"},
		{"forms", "questions", "delete", "form1", "1"},
		{"forms", "add-question", "form1", "--title", "D"},
		{"forms", "update", "form1", "--title", "New title", "--quiz=false"},
	}
	for _, args := range commands {
		result := h.run(t, append(args, "--batch", batchID)...)
		if result.err != nil {
			t.Fatalf("%v: %v", args, result.err)
		}
		if !strings.Contains(result.stdout, `"queued"`) || strings.Contains(result.stdout, `"created"`) || strings.Contains(result.stdout, `"deleted"`) {
			t.Fatalf("wrong queued result: %s", result.stdout)
		}
	}
	state, err := h.store.Get(batchID)
	if err != nil {
		t.Fatal(err)
	}
	if h.reads != 1 || len(h.bodies) != 0 || len(state.Requests) != 8 || state.InitialFormItems == nil || *state.InitialFormItems != 0 {
		t.Fatalf("reads=%d posts=%d state=%+v", h.reads, len(h.bodies), state)
	}
	for index, want := range map[int]string{0: `"index":0`, 1: `"index":1`, 2: `"index":0`, 5: `"index":2`, 7: `"isQuiz":false`} {
		if !compactJSONContains(t, state.Requests[index].Request, want) {
			t.Errorf("request %d: %s", index, state.Requests[index].Request)
		}
	}
	if editErr := h.store.WithState(batchID, func(tx *docsbatch.Transaction) error {
		tx.State().Requests = append(tx.State().Requests, docsbatch.RequestEntry{Command: "future request", Request: json.RawMessage(`{"updateFormInfo":{"info":{"description":"","future":false},"updateMask":"description"}}`)})
		return tx.PersistOrDelete()
	}); editErr != nil {
		t.Fatal(editErr)
	}
	state, err = h.store.Get(batchID)
	if err != nil {
		t.Fatal(err)
	}
	if result := h.run(t, "batch", "end", batchID); result.err != nil {
		t.Fatal(result.err)
	}
	if len(h.bodies) != 1 || h.bodies[0].WriteControl.RequiredRevisionId != "rev1" || len(h.bodies[0].Requests) != len(state.Requests) {
		t.Fatalf("unexpected body: %+v", h.bodies)
	}
	for i, raw := range h.bodies[0].Requests {
		var compact bytes.Buffer
		if err := json.Compact(&compact, state.Requests[i].Request); err != nil {
			t.Fatal(err)
		}
		if string(raw) != compact.String() {
			t.Errorf("changed raw request %d", i)
		}
	}
	if _, err := h.store.Get(batchID); !errors.Is(err, docsbatch.ErrNotFound) {
		t.Fatalf("batch retained: %v", err)
	}
}

func TestFormsBatchConcurrentAppends(t *testing.T) {
	h := newFormsBatchHarness(t)
	id := h.begin(t)
	const count = 8
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			result := h.run(t, "forms", "add-question", "form1", "--title", fmt.Sprintf("Q%d", i), "--batch", id)
			if result.err != nil {
				t.Errorf("queue: %v", result.err)
			}
		})
	}
	wg.Wait()
	state, err := h.store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Requests) != count || h.reads != 1 {
		t.Fatalf("queued=%d reads=%d", len(state.Requests), h.reads)
	}
	for i, entry := range state.Requests {
		var request formsapi.Request
		if err := json.Unmarshal(entry.Request, &request); err != nil {
			t.Fatal(err)
		}
		if request.CreateItem.Location.Index != int64(i) {
			t.Fatalf("request %d inserts at %d", i, request.CreateItem.Location.Index)
		}
	}
}

func TestFormsBatchRetainsStateWithoutReplay(t *testing.T) {
	for _, mode := range []string{"dry-run", "readonly", "400", "429", "503", "redirect", "malformed", "null", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			h := newFormsBatchHarness(t)
			id := h.begin(t)
			if result := h.run(t, "forms", "update", "form1", "--quiz=false", "--batch", id); result.err != nil {
				t.Fatal(result.err)
			}
			path, err := h.store.Path(id)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			h.post = func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "400":
					w.WriteHeader(http.StatusBadRequest)
				case "429":
					w.WriteHeader(http.StatusTooManyRequests)
				case "503":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "redirect":
					w.Header().Set("Location", r.URL.Path)
					w.WriteHeader(http.StatusTemporaryRedirect)
				case "malformed":
					_, _ = fmt.Fprint(w, `{`)
					return
				case "null":
					_, _ = fmt.Fprint(w, `null`)
					return
				case "trailing":
					_, _ = fmt.Fprint(w, `{} {}`)
					return
				default:
					t.Error("unexpected write")
				}
				_, _ = fmt.Fprint(w, `{"error":{"message":"rejected"}}`)
			}
			args := []string{"batch", "end", id}
			if mode == "dry-run" || mode == "readonly" {
				args = append(args, "--"+mode)
			}
			result := h.run(t, args...)
			if mode == "dry-run" && result.err != nil {
				t.Fatal(result.err)
			}
			if mode != "dry-run" && result.err == nil {
				t.Fatal("expected refusal")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("state changed: %v", err)
			}
			want := 1
			if mode == "dry-run" || mode == "readonly" {
				want = 0
			}
			if len(h.bodies) != want {
				t.Fatalf("writes=%d want=%d", len(h.bodies), want)
			}
		})
	}
}

func TestFormsBatchGuardsBeforeHTTP(t *testing.T) {
	h := newFormsBatchHarness(t)
	id := h.begin(t)
	for _, command := range [][]string{
		{"forms", "add-question", "form1", "--title", "A"},
		{"forms", "delete-question", "form1", "0"},
		{"forms", "move-question", "form1", "0", "1"},
		{"forms", "update", "form1", "--quiz=false"},
	} {
		for _, batchID := range []string{"", id} {
			args := append(append([]string{}, command...), "--dry-run")
			if batchID != "" {
				args = append(args, "--batch", batchID)
			}
			result := h.run(t, args...)
			if result.err != nil {
				t.Fatal(result.err)
			}
			var preview struct {
				Request map[string]any `json:"request"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &preview); err != nil {
				t.Fatal(err)
			}
			got, present := preview.Request["batch_id"]
			if present != (batchID != "") || (present && got != batchID) {
				t.Fatalf("unexpected batch preview: %s", result.stdout)
			}
		}
	}
	if h.reads != 0 || len(h.bodies) != 0 {
		t.Fatal("typed previews contacted Google")
	}
	for _, args := range [][]string{
		{"forms", "add-question", "other-form", "--title", "A", "--batch", id},
		{"forms", "add-question", "form1", "--title", "A", "--batch", id, "--account", "other@example.com"},
		{"forms", "add-question", "form1", "--title", "A", "--batch", id, "--client", "other"},
	} {
		if result := h.run(t, args...); result.err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if h.reads != 0 {
		t.Fatalf("identity mismatch read Google: %d", h.reads)
	}
	h.revision = ""
	if result := h.run(t, "forms", "update", "form1", "--quiz=false", "--batch", id); result.err == nil {
		t.Fatal("accepted missing revision")
	}
	h.revision = "rev1"
	if result := h.run(t, "forms", "update", "form1", "--quiz=false", "--batch", id); result.err != nil {
		t.Fatal(result.err)
	}
	for _, flag := range []string{"--continue-on-error", "--auto-split"} {
		for _, dry := range []bool{false, true} {
			args := []string{"batch", "end", id, flag}
			if dry {
				args = append(args, "--dry-run")
			}
			result := h.run(t, args...)
			if ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "atomic submission only") {
				t.Fatalf("%v: %v", args, result.err)
			}
		}
	}
	for _, args := range [][]string{
		{"forms", "delete-question", "form1", "0", "--batch", id},
		{"forms", "move-question", "form1", "0", "1", "--batch", id},
		{"forms", "add-question", "form1", "--title", "A", "--index", "2", "--batch", id},
	} {
		if result := h.run(t, args...); ExitCode(result.err) != 2 {
			t.Fatalf("invalid queued index accepted: %v: %v", args, result.err)
		}
	}
	if len(h.bodies) != 0 {
		t.Fatal("guard wrote to Google")
	}
}

func TestFormsBatchSubmissionPermissions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   []string
		profile string
		allow   bool
	}{
		{name: "unrestricted", allow: true},
		{name: "parent", flags: []string{"--enable-commands", "batch,forms"}},
		{name: "sibling deny", flags: []string{"--disable-commands", "forms.delete-question"}},
		{name: "exact", flags: []string{"--enable-commands", "batch.end,forms.batch-submit"}, allow: true},
		{name: "endpoint denied", flags: []string{"--disable-commands", "forms.batch-submit"}},
		{name: "baked parent", profile: "allow: [batch, forms]"},
		{name: "baked exact", profile: "allow: [batch.end, forms.batch-submit]", allow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFormsBatchHarness(t)
			id := h.begin(t)
			if result := h.run(t, "forms", "update", "form1", "--quiz=false", "--batch", id); result.err != nil {
				t.Fatal(result.err)
			}
			if tc.profile != "" {
				withBakedSafetyProfile(t, "name: forms-batch\n"+tc.profile+"\n")
			}
			result := h.run(t, append([]string{"batch", "end", id, "--dry-run"}, tc.flags...)...)
			if (result.err == nil) != tc.allow {
				t.Fatalf("allow=%t err=%v", tc.allow, result.err)
			}
			if len(h.bodies) != 0 {
				t.Fatal("permission preview submitted")
			}
		})
	}
}
