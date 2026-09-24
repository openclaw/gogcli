package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/docsbatch"
)

type batchServiceTarget struct {
	service string
	flag    string
	set     func(*docsbatch.State, string)
}

func allBatchServiceTargets() []batchServiceTarget {
	return []batchServiceTarget{
		{docsbatch.ServiceDocs, "--doc", func(s *docsbatch.State, id string) { s.DocumentID = id }},
		{docsbatch.ServiceSlides, "--presentation", func(s *docsbatch.State, id string) { s.PresentationID = id }},
		{docsbatch.ServiceForms, "--form", func(s *docsbatch.State, id string) { s.FormID = id }},
		{docsbatch.ServiceSheets, "--spreadsheet", func(s *docsbatch.State, id string) { s.SpreadsheetID = id }},
	}
}

func TestBatchServicesBeginRequiresExactlyOneTarget(t *testing.T) {
	setTestConfigHome(t)
	for _, target := range allBatchServiceTargets() {
		t.Run(target.service, func(t *testing.T) {
			args := []string{"--dry-run", "--json", "batch", "begin", "--service", target.service, target.flag, "target1"}
			result := executeWithTestRuntime(t, args, nil)
			if result.err != nil || !strings.Contains(result.stdout, `"service": "`+target.service+`"`) {
				t.Fatalf("single target failed: %v %s", result.err, result.stdout)
			}
			for _, other := range allBatchServiceTargets() {
				if other.service == target.service {
					continue
				}
				result := executeWithTestRuntime(t, append(append([]string{}, args...), other.flag, "other1"), nil)
				if ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "exactly one") || result.stdout != "" {
					t.Fatalf("accepted %s + %s: %v %s", target.flag, other.flag, result.err, result.stdout)
				}
			}
		})
	}
}

func TestBatchServicesRejectEveryForeignStoredTarget(t *testing.T) {
	for _, target := range allBatchServiceTargets() {
		t.Run(target.service, func(t *testing.T) {
			count := 0
			state := docsbatch.State{Service: target.service, Account: "test@example.com", Client: "default", RequiredRevisionID: "rev1"}
			if target.service == docsbatch.ServiceForms {
				state.InitialFormItems = &count
			}
			if target.service == docsbatch.ServiceSheets {
				state.RequiredRevisionID = ""
			}
			target.set(&state, "target1")
			if err := validateBatchSubmission(&RootFlags{}, &state); err != nil {
				t.Fatalf("valid state rejected: %v", err)
			}
			identity := docsbatch.Identity{Service: state.Service, DocumentID: state.DocumentID, PresentationID: state.PresentationID, FormID: state.FormID, SpreadsheetID: state.SpreadsheetID, Account: state.Account, Client: state.Client}
			for _, other := range allBatchServiceTargets() {
				if other.service == target.service {
					continue
				}
				mixed := state
				other.set(&mixed, "foreign1")
				if err := validateBatchSubmission(&RootFlags{}, &mixed); ExitCode(err) != 2 {
					t.Fatalf("submission accepted %s + %s: %v", target.service, other.service, err)
				}
				if err := docsbatch.ValidateIdentity(&mixed, identity); !errors.Is(err, docsbatch.ErrIdentityMismatch) {
					t.Fatalf("append identity accepted %s + %s: %v", target.service, other.service, err)
				}
			}
			wantRevision := target.service != docsbatch.ServiceSheets
			if batchUsesRevisions(target.service) != wantRevision {
				t.Fatalf("incorrect revision policy for %s", target.service)
			}
			body, err := json.Marshal(batchWirePayload(&state, []docsbatch.RequestEntry{{Request: json.RawMessage(`{"test":0}`)}}))
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(body, &wire); err != nil {
				t.Fatal(err)
			}
			if (wire["writeControl"] != nil) != wantRevision {
				t.Fatalf("wrong %s wire precondition: %s", target.service, body)
			}
		})
	}
}

func TestBatchServicesSchemaPermissionsRemainIndependent(t *testing.T) {
	setTestConfigHome(t)
	capabilities := map[string]string{"docs": "", "slides": "slides.batch-submit", "forms": "forms.batch-submit", "sheets": "sheets.batch-request"}
	for _, granted := range []string{"", "slides.batch-submit", "forms.batch-submit", "sheets.batch-request"} {
		for _, readonly := range []bool{false, true} {
			args := []string{"--enable-commands", "schema,batch"}
			if granted != "" {
				args[1] += "," + granted
			}
			if readonly {
				args = append(args, "--readonly")
			}
			result := executeWithTestRuntime(t, append(args, "schema"), nil)
			if result.err != nil {
				t.Fatal(result.err)
			}
			var doc schemaDoc
			if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Automation.Safety.BatchServices) != 4 {
				t.Fatalf("missing batch service: %+v", doc.Automation.Safety.BatchServices)
			}
			for service, capability := range capabilities {
				got := doc.Automation.Safety.BatchServices[service]
				wantAllowed := !readonly && (capability == "" || capability == granted)
				if got.AdditionalPermission != capability || got.SubmissionAllowed != wantAllowed {
					t.Fatalf("grant=%q readonly=%t service=%s: %+v", granted, readonly, service, got)
				}
			}
		}
	}
}

func TestBatchServicesFormsCapRetainsAtomicOnlyAdvice(t *testing.T) {
	h := newFormsBatchHarness(t)
	id := h.begin(t)
	if result := h.run(t, "forms", "update", "form1", "--quiz=false", "--batch", id); result.err != nil {
		t.Fatal(result.err)
	}
	if err := h.store.WithState(id, func(transaction *docsbatch.Transaction) error {
		state := transaction.State()
		entry := state.Requests[0]
		state.Requests = make([]docsbatch.RequestEntry, 501)
		for i := range state.Requests {
			state.Requests[i] = entry
		}
		return transaction.Persist()
	}); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{false, true} {
		args := []string{"batch", "end", id}
		if dry {
			args = append(args, "--dry-run")
		}
		result := h.run(t, args...)
		if ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "Forms batch has 501") || strings.Contains(result.err.Error(), "use --auto-split") {
			t.Fatalf("wrong Forms cap behavior: %v", result.err)
		}
	}
	if len(h.bodies) != 0 {
		t.Fatal("oversized Forms batch reached submission")
	}
}
