package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func docsDropdownTestDoc() *docs.Document {
	doc := docsFindRangeDoc(docsFindRangeParagraph(1, "Status: TODO here\n"))
	doc.RevisionId = "rev-dropdown"
	doc.Tabs = []*docs.Tab{{
		TabProperties: &docs.TabProperties{TabId: "t.0", Title: "Tab 1"},
		DocumentTab: &docs.DocumentTab{
			Body: doc.Body,
			DropdownDefinitions: map[string]docs.DropdownDefinition{
				"kix.status": {
					DropdownDefinitionId: "kix.status",
					DropdownDefinitionProperties: &docs.DropdownDefinitionProperties{
						Title: "Status",
						Options: []*docs.DropdownOption{
							{OptionId: "dropdownItem.shipped", DisplayValue: "🚀 Shipped"},
							{OptionId: "dropdownItem.proposed", DisplayValue: "💡 Proposed"},
						},
					},
				},
			},
		},
	}}
	return doc
}

func TestDocsInsertDropdownByIDSkipsLookup(t *testing.T) {
	t.Parallel()

	rec := &docsAtAnchorRecorder{}
	svc := setupDocsAtAnchorTestService(t, docsDropdownTestDoc(), rec)
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	args := []string{"doc1", "--definition", "kix.status", "--option", "dropdownItem.proposed", "--index", "9"}
	if err := runKong(t, &DocsInsertDropdownCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("insert-dropdown: %v", err)
	}
	if rec.getCalls != 0 {
		t.Fatalf("get calls = %d, want 0 when both IDs are given", rec.getCalls)
	}
	if len(rec.batchRequests) != 1 || len(rec.batchRequests[0]) != 1 {
		t.Fatalf("requests = %#v, want one insert-dropdown", rec.batchRequests)
	}
	got := rec.batchRequests[0][0].InsertDropdown
	if got == nil || got.DropdownDefinitionId != "kix.status" || got.SelectedOptionId != "dropdownItem.proposed" || got.Location.Index != 9 {
		t.Fatalf("insert dropdown = %#v", got)
	}
}

func TestDocsInsertDropdownAtAnchorResolvesTitleAndValue(t *testing.T) {
	t.Parallel()

	rec := &docsAtAnchorRecorder{}
	svc := setupDocsAtAnchorTestService(t, docsDropdownTestDoc(), rec)
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	args := []string{"doc1", "--definition", "status", "--option", "🚀 shipped", "--at", "TODO"}
	if err := runKong(t, &DocsInsertDropdownCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("insert-dropdown --at: %v", err)
	}
	if len(rec.batchRequests) != 1 {
		t.Fatalf("batch calls = %d, want one atomic call", len(rec.batchRequests))
	}
	reqs := rec.batchRequests[0]
	if len(reqs) != 2 || reqs[0].DeleteContentRange == nil || reqs[1].InsertDropdown == nil {
		t.Fatalf("requests = %#v, want delete+insert-dropdown", reqs)
	}
	if got := reqs[0].DeleteContentRange.Range; got.StartIndex != 9 || got.EndIndex != 13 {
		t.Fatalf("delete range = %#v, want 9:13", got)
	}
	dd := reqs[1].InsertDropdown
	if dd.DropdownDefinitionId != "kix.status" || dd.SelectedOptionId != "dropdownItem.shipped" || dd.Location.Index != 9 {
		t.Fatalf("insert dropdown = %#v", dd)
	}
	assertDocsAtAnchorWriteControl(t, rec, 0, "rev-dropdown")
}

func TestDocsInsertDropdownUnknownOptionListsValues(t *testing.T) {
	t.Parallel()

	rec := &docsAtAnchorRecorder{}
	svc := setupDocsAtAnchorTestService(t, docsDropdownTestDoc(), rec)
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	err := runKong(t, &DocsInsertDropdownCmd{}, []string{"doc1", "--definition", "kix.status", "--option", "Blocked", "--at-end"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), `"💡 Proposed"`) {
		t.Fatalf("err = %v, want unknown option listing available values", err)
	}
	if len(rec.batchRequests) != 0 {
		t.Fatalf("batch calls = %d, want none after a failed lookup", len(rec.batchRequests))
	}
}

func TestDocsListDropdownsJSON(t *testing.T) {
	t.Parallel()

	rec := &docsAtAnchorRecorder{}
	svc := setupDocsAtAnchorTestService(t, docsDropdownTestDoc(), rec)
	var out bytes.Buffer
	ctx := withDocsTestService(outfmt.WithMode(newCmdRuntimeOutputContext(t, &out, io.Discard), outfmt.Mode{JSON: true}), svc)

	if err := runKong(t, &DocsListDropdownsCmd{}, []string{"doc1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("list-dropdowns: %v", err)
	}
	var got struct {
		Dropdowns []docsDropdownDefinitionInfo `json:"dropdowns"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", out.String(), err)
	}
	if len(got.Dropdowns) != 1 {
		t.Fatalf("dropdowns = %#v, want one", got.Dropdowns)
	}
	def := got.Dropdowns[0]
	if def.ID != "kix.status" || def.Title != "Status" || def.TabID != "t.0" || len(def.Options) != 2 || def.Options[1].Value != "💡 Proposed" {
		t.Fatalf("definition = %#v", def)
	}
}

func docsDropdownTwoTabDoc() *docs.Document {
	statusTab := func(id, title, defID, optID, value string) *docs.Tab {
		return &docs.Tab{
			TabProperties: &docs.TabProperties{TabId: id, Title: title},
			DocumentTab: &docs.DocumentTab{
				Body: &docs.Body{Content: []*docs.StructuralElement{docsFindRangeParagraph(1, "Status here\n")}},
				DropdownDefinitions: map[string]docs.DropdownDefinition{
					defID: {
						DropdownDefinitionId: defID,
						DropdownDefinitionProperties: &docs.DropdownDefinitionProperties{
							Title:   "Status",
							Options: []*docs.DropdownOption{{OptionId: optID, DisplayValue: value}},
						},
					},
				},
			},
		}
	}
	doc := docsFindRangeDoc(docsFindRangeParagraph(1, "Status here\n"))
	doc.Tabs = []*docs.Tab{
		statusTab("t.plan", "Planning", "kix.status", "dropdownItem.plan", "Active"),
		statusTab("t.arch", "Archive", "kix.status", "dropdownItem.arch", "Archived"),
	}
	return doc
}

func TestDocsInsertDropdownResolvesWithinTargetTab(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		args     []string
		wantOpt  string
		wantTab  string
		wantFail string
	}{
		{name: "explicit tab", args: []string{"--tab", "Archive", "--option", "archived"}, wantOpt: "dropdownItem.arch", wantTab: "t.arch"},
		{name: "default first tab", args: []string{"--option", "active"}, wantOpt: "dropdownItem.plan"},
		{name: "value from another tab", args: []string{"--tab", "Planning", "--option", "archived"}, wantFail: `option "archived" not in dropdown`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &docsAtAnchorRecorder{}
			svc := setupDocsAtAnchorTestService(t, docsDropdownTwoTabDoc(), rec)
			ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

			args := append([]string{"doc1", "--definition", "Status", "--at-end"}, tc.args...)
			err := runKong(t, &DocsInsertDropdownCmd{}, args, ctx, &RootFlags{Account: "a@b.com"})
			if tc.wantFail != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantFail) {
					t.Fatalf("err = %v, want %q", err, tc.wantFail)
				}
				return
			}
			if err != nil {
				t.Fatalf("insert-dropdown: %v", err)
			}
			dd := rec.batchRequests[0][len(rec.batchRequests[0])-1].InsertDropdown
			if dd == nil || dd.DropdownDefinitionId != "kix.status" || dd.SelectedOptionId != tc.wantOpt || dd.Location.TabId != tc.wantTab {
				t.Fatalf("insert dropdown = %#v, want option %s in tab %q", dd, tc.wantOpt, tc.wantTab)
			}
		})
	}
}
