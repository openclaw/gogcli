package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/docsbatch"
)

func TestSlidesBatchBeginTargets(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--doc", "doc1"}, "docs"},
		{[]string{"--presentation", "deck1"}, "slides"},
		{[]string{"--service", "slides", "--presentation", "deck1"}, "slides"},
		{nil, ""},
		{[]string{"--doc", "doc1", "--presentation", "deck1"}, ""},
		{[]string{"--service", "docs", "--presentation", "deck1"}, ""},
		{[]string{"--service", "unknown", "--doc", "doc1"}, ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			result := executeWithTestRuntime(t, append([]string{"--dry-run", "--json", "batch", "begin"}, test.args...), nil)
			if test.want == "" {
				if ExitCode(result.err) != 2 {
					t.Fatalf("expected usage error: %v", result.err)
				}
			} else if result.err != nil || !strings.Contains(result.stdout, `"service": "`+test.want+`"`) {
				t.Fatalf("error=%v output=%s", result.err, result.stdout)
			}
		})
	}
}

func TestSlidesBatchSubmissionPermissions(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags []string
		baked string
		allow bool
	}{
		{name: "unrestricted", allow: true},
		{name: "legacy batch grant", flags: []string{"--enable-commands", "batch"}},
		{name: "service parent", flags: []string{"--enable-commands", "batch,slides"}},
		{name: "deny only", flags: []string{"--disable-commands", "slides.element.delete"}},
		{name: "wildcard with deny", flags: []string{"--enable-commands", "*", "--disable-commands", "slides.element.delete"}},
		{name: "full", flags: []string{"--enable-commands", "*"}, allow: true},
		{name: "explicit", flags: []string{"--enable-commands", "batch,slides.batch-submit", "--disable-commands", "slides.element.delete"}, allow: true},
		{name: "exact", flags: []string{"--enable-commands-exact", "batch.end,slides.batch-submit"}, allow: true},
		{name: "parent deny", flags: []string{"--enable-commands", "batch,slides.batch-submit", "--disable-commands", "slides"}},
		{name: "capability deny", flags: []string{"--disable-commands", "slides.batch-submit"}},
		{name: "baked legacy", baked: "allow: [batch]"},
		{name: "baked service parent", baked: "allow: [batch, slides]"},
		{name: "baked deny only", baked: "deny: [slides.element.delete]"},
		{name: "baked wildcard with deny", baked: "allow: [all]\ndeny: [slides.element.delete]"},
		{name: "baked full", baked: "allow: [all]", allow: true},
		{name: "baked explicit", baked: "allow: [batch, slides.batch-submit]\ndeny: [slides.element.delete]", allow: true},
		{name: "baked parent deny", baked: "allow: [batch, slides.batch-submit]\ndeny: [slides]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newSlidesBatchHarness(t)
			id := h.begin(t)
			if result := h.run(t, "slides", "insert-text", "deck1", "shape1", "text", "--batch", id); result.err != nil {
				t.Fatal(result.err)
			}
			if test.baked != "" {
				withBakedSafetyProfile(t, "name: batch-test\n"+test.baked+"\n")
			}
			for _, dryRun := range []bool{true, false} {
				args := append(append([]string{}, test.flags...), "batch", "end", id)
				if dryRun {
					args = append(args, "--dry-run")
				}
				result := h.run(t, args...)
				if test.allow && result.err != nil {
					t.Fatal(result.err)
				}
				if !test.allow && (ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "slides batch-submit")) {
					t.Fatalf("expected service permission refusal: %v %s", result.err, result.stderr)
				}
			}
			wantWrites := 0
			if test.allow {
				wantWrites = 1
			}
			if len(h.bodies) != wantWrites {
				t.Fatalf("writes=%d want=%d", len(h.bodies), wantWrites)
			}
		})
	}
}

func TestSlidesBatchSchemaPermissions(t *testing.T) {
	for _, test := range []struct {
		flags        []string
		docs, slides bool
	}{
		{nil, true, true},
		{[]string{"--enable-commands", "schema,batch"}, true, false},
		{[]string{"--enable-commands", "schema,batch,slides.batch-submit"}, true, true},
		{[]string{"--readonly"}, false, false},
		{[]string{"--enable-commands", "schema,slides.batch-submit"}, false, false},
	} {
		t.Run(strings.Join(test.flags, " "), func(t *testing.T) {
			setTestConfigHome(t)
			result := executeWithTestRuntime(t, append(test.flags, "schema"), nil)
			if result.err != nil {
				t.Fatal(result.err)
			}
			var doc schemaDoc
			if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
				t.Fatal(err)
			}
			services := doc.Automation.Safety.BatchServices
			if services["docs"].SubmissionAllowed != test.docs || services["slides"].SubmissionAllowed != test.slides || services["slides"].AdditionalPermission != "slides.batch-submit" {
				t.Fatalf("unexpected batch capabilities: %+v", services)
			}
		})
	}
}

func TestSlidesBatchSplitChainsRevision(t *testing.T) {
	for _, missingRevision := range []bool{false, true} {
		t.Run(fmt.Sprint(missingRevision), func(t *testing.T) {
			h := newSlidesBatchHarness(t)
			id := h.begin(t)
			requests := make([]json.RawMessage, persistedBatchRequestCap+1)
			for i := range requests {
				requests[i] = json.RawMessage(`{"insertText":{"objectId":"shape1","text":"text"}}`)
			}
			if _, err := h.store.Append(docsbatch.AppendOptions{
				BatchID: id, Command: "slides.insert-text", RevisionID: "rev1", Requests: requests,
				Identity: docsbatch.Identity{Service: docsbatch.ServiceSlides, PresentationID: "deck1", Account: "test@example.com", Client: "default"},
			}); err != nil {
				t.Fatal(err)
			}
			if result := h.run(t, "batch", "end", id); ExitCode(result.err) != 2 || len(h.bodies) != 0 {
				t.Fatalf("oversized atomic batch was submitted: %v", result.err)
			}
			if missingRevision {
				h.post = func(w http.ResponseWriter, _ *http.Request, _ slidesBatchWireBody) { _, _ = fmt.Fprint(w, `{}`) }
			}
			result := h.run(t, "batch", "end", id, "--auto-split")
			if missingRevision {
				if result.err == nil || !strings.Contains(result.err.Error(), "omitted the revision") {
					t.Fatalf("unexpected error: %v", result.err)
				}
				state, err := h.store.Get(id)
				if err != nil || len(state.Requests) != 1 {
					t.Fatalf("progress was not retained: %+v %v", state, err)
				}
			} else {
				if result.err != nil {
					t.Fatal(result.err)
				}
				if len(h.bodies) != 2 || len(h.bodies[0].Requests) != 500 || len(h.bodies[1].Requests) != 1 || h.bodies[1].WriteControl.RequiredRevisionId != "rev2" {
					t.Fatalf("incorrect split: %+v", h.bodies)
				}
			}
		})
	}
}

func TestSlidesBatchContinueRetainsFailureAndUsesBoundIdentity(t *testing.T) {
	h := newSlidesBatchHarness(t)
	id := h.begin(t)
	for _, text := range []string{"valid", "invalid"} {
		if result := h.run(t, "slides", "insert-text", "deck1", "shape1", text, "--batch", id); result.err != nil {
			t.Fatal(result.err)
		}
	}
	h.post = func(w http.ResponseWriter, _ *http.Request, body slidesBatchWireBody) {
		if len(body.Requests) != 1 || strings.Contains(string(body.Requests[0]), "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"invalid request"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"writeControl":{"requiredRevisionId":"rev2"}}`)
	}
	result := executeWithTestRuntime(t, []string{"--account", "other@example.com", "--client", "other", "--no-input", "--json", "batch", "end", id, "--continue-on-error"}, h.runtime)
	if result.err == nil || !strings.Contains(result.stdout, `"failed": 1`) || !strings.Contains(result.stdout, `"atomic": false`) {
		t.Fatalf("failure summary lost: %v %s", result.err, result.stdout)
	}
	state, err := h.store.Get(id)
	if err != nil || len(state.Requests) != 1 || state.RequiredRevisionID != "rev2" || !strings.Contains(string(state.Requests[0].Request), "invalid") {
		t.Fatalf("wrong retained state: %+v %v", state, err)
	}
	if len(h.bodies) != 3 || h.bodies[2].WriteControl.RequiredRevisionId != "rev2" {
		t.Fatalf("revision not advanced: %+v", h.bodies)
	}
}
