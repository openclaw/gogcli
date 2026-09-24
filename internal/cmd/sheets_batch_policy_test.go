package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/docsbatch"
)

func TestSheetsPersistedBatchSubmissionPolicy(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags []string
		baked string
		allow bool
	}{
		{name: "unrestricted", allow: true},
		{name: "batch grant", flags: []string{"--enable-commands", "batch"}},
		{name: "Sheets parent", flags: []string{"--enable-commands", "batch,sheets"}},
		{name: "deny sibling", flags: []string{"--disable-commands", "sheets.delete-tab"}},
		{name: "wildcard and deny", flags: []string{"--enable-commands", "*", "--disable-commands", "sheets.delete-tab"}},
		{name: "wildcard", flags: []string{"--enable-commands", "*"}, allow: true},
		{name: "explicit plus sibling deny", flags: []string{"--enable-commands", "batch,sheets.batch-request", "--disable-commands", "sheets.delete-tab"}, allow: true},
		{name: "exact", flags: []string{"--enable-commands-exact", "batch.end,sheets.batch-request"}, allow: true},
		{name: "parent deny", flags: []string{"--disable-commands", "sheets"}},
		{name: "baked parent", baked: "allow: [batch, sheets]"},
		{name: "baked explicit", baked: "allow: [batch, sheets.batch-request]\ndeny: [sheets.delete-tab]", allow: true},
		{name: "baked wildcard deny", baked: "allow: [all]\ndeny: [sheets.delete-tab]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newSheetsPersistedHarness(t)
			id := h.begin(t)
			if result := h.run(t, "sheets", "batch-request", "sheet1", "--requests-json", `[{"deleteSheet":{"sheetId":0}}]`, "--batch", id); result.err != nil {
				t.Fatal(result.err)
			}
			if test.baked != "" {
				withBakedSafetyProfile(t, "name: sheets-test\n"+test.baked+"\n")
			}
			for _, dryRun := range []bool{true, false} {
				args := append(append([]string{}, test.flags...), "--force", "batch", "end", id)
				if dryRun {
					args = append(args, "--dry-run")
				}
				result := h.run(t, args...)
				if test.allow && result.err != nil {
					t.Fatal(result.err)
				}
				if !test.allow && ExitCode(result.err) != 2 {
					t.Fatalf("policy was not enforced: %v", result.err)
				}
			}
			if !test.allow && (len(h.posts) != 0 || h.reads.Load() != 0) {
				t.Fatal("denied submission reached Google")
			}
		})
	}
}

func TestSheetsPersistedBatchReadOnlyAndConfirmationBeforeAuth(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	if result := h.run(t, "--readonly", "sheets", "batch-request", "sheet1", "--requests-json", `[{}]`, "--batch", id); result.err != nil {
		t.Fatal(result.err)
	}
	h.runtime.Services.SheetsHTTP = func(context.Context, string) (*http.Client, error) {
		t.Fatal("guard reached authentication")
		return nil, errRuntimeServiceRequired
	}
	for _, flags := range [][]string{nil, {"--readonly", "--force"}} {
		result := h.run(t, append(flags, "batch", "end", id)...)
		if ExitCode(result.err) != 2 {
			t.Fatalf("expected guard refusal: %v", result.err)
		}
	}
	if result := h.run(t, "--readonly", "--dry-run", "batch", "end", id); result.err != nil {
		t.Fatal(result.err)
	}
}

func TestSheetsPersistedBatchIdentityAndStoredTargets(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	id := h.begin(t)
	before, err := os.ReadFile(h.storePath(t, id))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ account, client, target string }{
		{"other@example.com", "default", "sheet1"},
		{"test@example.com", "other", "sheet1"},
		{"test@example.com", "default", "other-sheet"},
	} {
		result := executeWithTestRuntime(t, []string{"--account", test.account, "--client", test.client, "--json", "sheets", "format", test.target, "Data!A1", "--format-json", `{"textFormat":{"bold":true}}`, "--batch", id}, h.runtime)
		if result.err == nil || h.reads.Load() != 0 || len(h.posts) != 0 {
			t.Fatalf("identity mismatch accepted: %+v %v", test, result.err)
		}
	}
	after, err := os.ReadFile(h.storePath(t, id))
	if err != nil || string(before) != string(after) {
		t.Fatalf("mismatch changed state: %v", err)
	}
	for _, mutate := range []func(*docsbatch.State){
		func(s *docsbatch.State) { s.DocumentID = "doc1" },
		func(s *docsbatch.State) { s.PresentationID = "deck1" },
		func(s *docsbatch.State) { s.FormID = "form1" },
		func(s *docsbatch.State) { s.RequiredRevisionID = "not-a-Sheets-revision" },
	} {
		state, err := h.store.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		mutate(state)
		if err := validateBatchSubmission(&RootFlags{}, state); err == nil {
			t.Fatalf("invalid state allowed: %+v", state)
		}
	}
}

func TestSheetsPersistedBatchEmptyFlagsAndExcludedCommands(t *testing.T) {
	h := newSheetsPersistedHarness(t)
	for _, command := range sheetsPersistedCommands() {
		args := append([]string{"--dry-run", "sheets"}, command...)
		result := h.run(t, append(args, "--batch=")...)
		if ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "non-empty batch ID") {
			t.Fatalf("%v accepted empty batch: %v %s", command, result.err, result.stderr)
		}
	}
	for _, command := range [][]string{
		{"conditional-format", "clear", "sheet1", "--sheet", "Data", "--all"},
		{"delete-dimension", "sheet1", "Data!2:3", "--dimension", "ROWS"},
		{"reorder-tab", "sheet1", "--tab", "Data", "--to", "1"},
		{"duplicate-tab", "sheet1", "Data", "Duplicate"},
		{"validation", "set", "sheet1", "Data!A1", "--type", "BOOLEAN"},
		{"validation", "clear", "sheet1", "Data!A1"},
		{"copy-paste", "sheet1", "Data!A1", "Data!A2"},
		{"update", "sheet1", "Data!A1", "--values-json", `[[1]]`},
	} {
		result := h.run(t, append(append([]string{"sheets"}, command...), "--batch", "01900000-0000-7000-8000-000000000000")...)
		if ExitCode(result.err) != 2 {
			t.Fatalf("excluded command accepted --batch: %v %v", command, result.err)
		}
	}
	if h.reads.Load() != 0 || len(h.posts) != 0 {
		t.Fatal("invalid flag reached Google")
	}
}

func TestSheetsPersistedBatchSchemaPermission(t *testing.T) {
	setTestConfigHome(t)
	for _, enabled := range []string{"schema,batch", "schema,batch,sheets.batch-request"} {
		result := executeWithTestRuntime(t, []string{"--enable-commands", enabled, "schema"}, &app.Runtime{})
		if result.err != nil {
			t.Fatal(result.err)
		}
		var doc schemaDoc
		if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
			t.Fatal(err)
		}
		policy := doc.Automation.Safety.BatchServices["sheets"]
		if policy.AdditionalPermission != "sheets.batch-request" || policy.SubmissionAllowed != strings.Contains(enabled, "sheets.batch-request") {
			t.Fatalf("wrong schema permission: %+v", policy)
		}
	}
}
