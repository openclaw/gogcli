package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsTableRowStyleBuildsExplicitFalseValues(t *testing.T) {
	falseValue := false
	cmd := &DocsTableRowStyleCmd{
		MinHeight:       "0",
		PreventOverflow: &falseValue,
	}
	style, fields, err := cmd.buildStyle()
	if err != nil {
		t.Fatalf("buildStyle: %v", err)
	}
	if got := strings.Join(fields, ","); got != "minRowHeight,preventOverflow" {
		t.Fatalf("fields = %q", got)
	}
	encoded, err := json.Marshal(style)
	if err != nil {
		t.Fatalf("marshal style: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, `"preventOverflow":false`) {
		t.Fatalf("style JSON = %s", got)
	}
	if style.MinRowHeight == nil || style.MinRowHeight.Magnitude != 0 || style.MinRowHeight.Unit != "PT" {
		t.Fatalf("min row height = %#v", style.MinRowHeight)
	}
}

func TestResolveDocsTableStyleRow(t *testing.T) {
	table := docsTableOpsTestElement(5, "Header", 3).Table
	rows, resolved, err := resolveDocsTableStyleRow(table, nil)
	if err != nil || rows != nil || resolved != "all" {
		t.Fatalf("all rows = %#v, %#v, %v", rows, resolved, err)
	}
	last := -1
	rows, resolved, err = resolveDocsTableStyleRow(table, &last)
	if err != nil || len(rows) != 1 || rows[0] != 2 || resolved != 3 {
		t.Fatalf("last row = %#v, %#v, %v", rows, resolved, err)
	}
	outOfRange := 4
	if _, _, err := resolveDocsTableStyleRow(table, &outOfRange); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
}

func TestDocsTableRowStyleRunUsesRevisionAndResolvedRow(t *testing.T) {
	doc := docsTableOpsTestDocument(docsTableOpsTestElement(5, "Header", 2))
	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			_ = json.NewEncoder(w).Encode(&docs.BatchUpdateDocumentResponse{DocumentId: "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	cmd := &DocsTableRowStyleCmd{}
	err := runKong(t, cmd, []string{
		"doc1", "--row=-1", "--min-height", "24pt", "--prevent-overflow",
	}, newDocsTableOpsTestContext(t, docSvc), &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.WriteControl == nil || got.WriteControl.RequiredRevisionId != "rev-1" {
		t.Fatalf("write control = %#v", got.WriteControl)
	}
	if len(got.Requests) != 1 || got.Requests[0].UpdateTableRowStyle == nil {
		t.Fatalf("requests = %#v", got.Requests)
	}
	request := got.Requests[0].UpdateTableRowStyle
	if request.TableStartLocation.Index != 5 || len(request.RowIndices) != 1 || request.RowIndices[0] != 1 {
		t.Fatalf("target = %#v rows=%#v", request.TableStartLocation, request.RowIndices)
	}
	if request.Fields != "minRowHeight,preventOverflow" {
		t.Fatalf("fields = %q", request.Fields)
	}
	if request.TableRowStyle.MinRowHeight.Magnitude != 24 || !request.TableRowStyle.PreventOverflow {
		t.Fatalf("style = %#v", request.TableRowStyle)
	}
}

func TestDocsTableRowStyleRequiresStyleFlag(t *testing.T) {
	cmd := &DocsTableRowStyleCmd{DocID: "doc1", Table: "1"}
	err := cmd.Run(newCmdRuntimeOutputContext(t, nil, nil), &RootFlags{Account: "a@b.com", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "no row style flags") {
		t.Fatalf("expected style error, got %v", err)
	}
}
