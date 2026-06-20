package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsPageSizePresetBuildsDimensions(t *testing.T) {
	req, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		DocsLayoutFlags: DocsLayoutFlags{PageSize: "A4"},
	})
	if err != nil {
		t.Fatalf("buildUpdateDocumentStyleRequest: %v", err)
	}
	if req.Fields != "pageSize.width,pageSize.height" {
		t.Fatalf("fields = %q", req.Fields)
	}
	if got := req.DocumentStyle.PageSize.Width.Magnitude; got != 595.275 {
		t.Fatalf("width = %v", got)
	}
	if got := req.DocumentStyle.PageSize.Height.Magnitude; got != 841.890 {
		t.Fatalf("height = %v", got)
	}
}

func TestDocsPageSizeRejectsExplicitDimensions(t *testing.T) {
	_, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		DocsLayoutFlags: DocsLayoutFlags{PageSize: "Letter", PageWidth: "1in"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestDocsCellStyleBuildsTableAndTextRequests(t *testing.T) {
	cmd := &DocsCellStyleCmd{
		Row:             1,
		Col:             2,
		RowSpan:         1,
		ColSpan:         2,
		BackgroundColor: "#abc",
		TextColor:       "#123456",
		Bold:            true,
	}
	cell := &docs.TableCell{Content: []*docs.StructuralElement{{
		Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
			StartIndex: 10,
			EndIndex:   16,
			TextRun:    &docs.TextRun{Content: "Badge\n"},
		}}},
	}}}
	reqs, err := cmd.buildRequests(5, cell, "tab-1")
	if err != nil {
		t.Fatalf("buildRequests: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	cellReq := reqs[0].UpdateTableCellStyle
	if cellReq == nil || cellReq.Fields != "backgroundColor" {
		t.Fatalf("unexpected cell style request: %#v", reqs[0])
	}
	loc := cellReq.TableRange.TableCellLocation
	if loc.RowIndex != 0 || loc.ColumnIndex != 1 || loc.TableStartLocation.Index != 5 || loc.TableStartLocation.TabId != "tab-1" {
		t.Fatalf("unexpected cell location: %#v", loc)
	}
	textReq := reqs[1].UpdateTextStyle
	if textReq == nil || textReq.Range.StartIndex != 10 || textReq.Range.EndIndex != 15 || textReq.Range.TabId != "tab-1" {
		t.Fatalf("unexpected text style request: %#v", reqs[1])
	}
	if !textReq.TextStyle.Bold || textReq.Fields != "foregroundColor,bold" {
		t.Fatalf("unexpected text style: %#v fields=%q", textReq.TextStyle, textReq.Fields)
	}
}

func TestDocsCellStyleBuildsBordersPaddingAndAlignment(t *testing.T) {
	cmd := &DocsCellStyleCmd{
		Row:          2,
		Col:          3,
		RowSpan:      2,
		ColSpan:      4,
		BorderAll:    "1pt,#abc,DOT",
		BorderTop:    "2pt,#123456,DASH",
		PaddingAll:   "6pt",
		PaddingRight: "0",
		ContentAlign: "middle",
	}
	reqs, err := cmd.buildRequests(8, &docs.TableCell{}, "tab-2")
	if err != nil {
		t.Fatalf("buildRequests: %v", err)
	}
	if len(reqs) != 1 || reqs[0].UpdateTableCellStyle == nil {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	got := reqs[0].UpdateTableCellStyle
	wantFields := "borderRight,borderLeft,borderBottom,borderTop,paddingTop,paddingBottom,paddingLeft,paddingRight,contentAlignment"
	if got.Fields != wantFields {
		t.Fatalf("fields = %q, want %q", got.Fields, wantFields)
	}
	if got.TableRange.RowSpan != 2 || got.TableRange.ColumnSpan != 4 {
		t.Fatalf("table range = %#v", got.TableRange)
	}
	style := got.TableCellStyle
	if style.BorderRight.Width.Magnitude != 1 || style.BorderRight.DashStyle != "DOT" {
		t.Fatalf("right border = %#v", style.BorderRight)
	}
	if style.BorderTop.Width.Magnitude != 2 || style.BorderTop.DashStyle != "DASH" {
		t.Fatalf("top border = %#v", style.BorderTop)
	}
	if gotRed := style.BorderTop.Color.Color.RgbColor.Red; gotRed < 0.07 || gotRed > 0.08 {
		t.Fatalf("top border color = %#v", style.BorderTop.Color)
	}
	if style.PaddingTop.Magnitude != 6 || style.PaddingRight.Magnitude != 0 {
		t.Fatalf("padding = top %#v right %#v", style.PaddingTop, style.PaddingRight)
	}
	if style.ContentAlignment != "MIDDLE" {
		t.Fatalf("content alignment = %q", style.ContentAlignment)
	}
}

func TestDocsCellStyleRejectsInvalidTableStyles(t *testing.T) {
	tests := []struct {
		name string
		cmd  DocsCellStyleCmd
		want string
	}{
		{name: "border shape", cmd: DocsCellStyleCmd{BorderTop: "1pt,#fff,SOLID,extra"}, want: "expected WIDTH"},
		{name: "border dash", cmd: DocsCellStyleCmd{BorderTop: "1pt,#fff,WAVE"}, want: "expected SOLID"},
		{name: "border color", cmd: DocsCellStyleCmd{BorderTop: "1pt,nope"}, want: "must be #RRGGBB"},
		{name: "padding", cmd: DocsCellStyleCmd{PaddingAll: "-1pt"}, want: "non-negative length"},
		{name: "alignment", cmd: DocsCellStyleCmd{ContentAlign: "center"}, want: "top, middle, or bottom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := tt.cmd.buildCellStyle()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestDocsCellStyle_TableSelectionErrorsAreUsage(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(docBodyWithText("No tables\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	cmd := &DocsCellStyleCmd{}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)
	err := runKong(t, cmd, []string{"doc1", "--row", "1", "--col", "1", "--background-color", "#fff"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "document has no tables") {
		t.Fatalf("expected no-tables error, got %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestDocsCellStyleValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  DocsCellStyleCmd
		want string
	}{
		{
			name: "zero table index",
			cmd:  DocsCellStyleCmd{DocID: "doc1", TableIndex: 0, Row: 1, Col: 1, RowSpan: 1, ColSpan: 1, BackgroundColor: "#fff"},
			want: "--table-index cannot be 0",
		},
		{
			name: "zero row",
			cmd:  DocsCellStyleCmd{DocID: "doc1", TableIndex: 1, Row: 0, Col: 1, RowSpan: 1, ColSpan: 1, BackgroundColor: "#fff"},
			want: "--row must be >= 1",
		},
		{
			name: "zero column",
			cmd:  DocsCellStyleCmd{DocID: "doc1", TableIndex: 1, Row: 1, Col: 0, RowSpan: 1, ColSpan: 1, BackgroundColor: "#fff"},
			want: "--col must be >= 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cmd.Run(
				newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
				&RootFlags{Account: "a@b.com", DryRun: true},
			)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
	}
}

func TestDocsCellStyle_NegativeTableIndexReportsResolvedIndex(t *testing.T) {
	t.Parallel()

	var got docs.BatchUpdateDocumentRequest
	doc := &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev1",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{
				StartIndex: 1,
				EndIndex:   10,
				Table: &docs.Table{TableRows: []*docs.TableRow{
					{TableCells: []*docs.TableCell{cellUpdateTestCell(3, "First\n")}},
				}},
			},
			{
				StartIndex: 20,
				EndIndex:   30,
				Table: &docs.Table{TableRows: []*docs.TableRow{
					{TableCells: []*docs.TableCell{cellUpdateTestCell(23, "Last\n")}},
				}},
			},
		}},
	}
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	var output strings.Builder
	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), docSvc)
	err := runKong(
		t,
		&DocsCellStyleCmd{},
		[]string{"doc1", "--table-index=-1", "--row", "1", "--col", "1", "--background-color", "#fff"},
		ctx,
		&RootFlags{Account: "a@b.com"},
	)
	if err != nil {
		t.Fatalf("docs cell-style: %v", err)
	}
	if len(got.Requests) != 1 || got.Requests[0].UpdateTableCellStyle == nil {
		t.Fatalf("unexpected requests: %#v", got.Requests)
	}
	loc := got.Requests[0].UpdateTableCellStyle.TableRange.TableCellLocation
	if loc.TableStartLocation.Index != 20 || loc.RowIndex != 0 || loc.ColumnIndex != 0 {
		t.Fatalf("unexpected cell location: %#v", loc)
	}
	var payload struct {
		TableIndex int `json:"tableIndex"`
		Row        int `json:"row"`
		Col        int `json:"col"`
	}
	if err := json.Unmarshal([]byte(output.String()), &payload); err != nil {
		t.Fatalf("decode output %q: %v", output.String(), err)
	}
	if payload.TableIndex != 2 || payload.Row != 1 || payload.Col != 1 {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestDocsTableColumnWidthBuildsFixedRequest(t *testing.T) {
	cmd := &DocsTableColumnWidthCmd{Col: 2, Width: 120}
	req, err := cmd.buildRequest(5, 3, "tab-1")
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	got := req.UpdateTableColumnProperties
	if got == nil {
		t.Fatalf("missing update request: %#v", req)
	}
	if got.TableStartLocation.Index != 5 || got.TableStartLocation.TabId != "tab-1" {
		t.Fatalf("table start = %#v", got.TableStartLocation)
	}
	if len(got.ColumnIndices) != 1 || got.ColumnIndices[0] != 1 {
		t.Fatalf("column indices = %#v", got.ColumnIndices)
	}
	if got.Fields != "width,widthType" {
		t.Fatalf("fields = %q", got.Fields)
	}
	props := got.TableColumnProperties
	if props.WidthType != "FIXED_WIDTH" || props.Width == nil || props.Width.Magnitude != 120 || props.Width.Unit != "PT" {
		t.Fatalf("properties = %#v", props)
	}
}

func TestDocsTableColumnWidthBuildsEvenAllColumnsRequest(t *testing.T) {
	cmd := &DocsTableColumnWidthCmd{EvenlyDistributed: true}
	req, err := cmd.buildRequest(7, 2, "")
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	got := req.UpdateTableColumnProperties
	if got == nil {
		t.Fatalf("missing update request: %#v", req)
	}
	if len(got.ColumnIndices) != 0 {
		t.Fatalf("column indices = %#v", got.ColumnIndices)
	}
	if got.Fields != "widthType" || got.TableColumnProperties.WidthType != "EVENLY_DISTRIBUTED" {
		t.Fatalf("unexpected request: %#v", got)
	}
}

func TestDocsTableColumnWidthValidation(t *testing.T) {
	tests := []struct {
		name string
		cmd  DocsTableColumnWidthCmd
		want string
	}{
		{
			name: "missing mode",
			cmd:  DocsTableColumnWidthCmd{TableIndex: 1, Col: 1},
			want: "set --width or --evenly-distributed",
		},
		{
			name: "conflicting mode",
			cmd:  DocsTableColumnWidthCmd{TableIndex: 1, Col: 1, Width: 120, EvenlyDistributed: true},
			want: "mutually exclusive",
		},
		{
			name: "fixed requires column",
			cmd:  DocsTableColumnWidthCmd{TableIndex: 1, Width: 120},
			want: "--col is required",
		},
		{
			name: "minimum width",
			cmd:  DocsTableColumnWidthCmd{TableIndex: 1, Col: 1, Width: 4.9},
			want: "--width must be >= 5pt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestDocsTableColumnWidth_TargetErrorsAreUsage(t *testing.T) {
	doc := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{}}}
	if _, _, err := resolveDocsTableWithIndex(doc, 1); err == nil || ExitCode(err) != 2 {
		t.Fatalf("resolve no tables error = %v, exit=%d", err, ExitCode(err))
	}

	doc = cellUpdateTestDoc()
	if _, _, err := resolveDocsTableWithIndex(doc, 2); err == nil || ExitCode(err) != 2 {
		t.Fatalf("resolve out of range error = %v, exit=%d", err, ExitCode(err))
	}

	cmd := &DocsTableColumnWidthCmd{Col: 3, Width: 120}
	if _, err := cmd.buildRequest(5, 2, ""); err == nil || ExitCode(err) != 2 {
		t.Fatalf("col out of range error = %v, exit=%d", err, ExitCode(err))
	}
}

func TestDocsSmartChipCommandsBuildRequests(t *testing.T) {
	person := &docs.Request{InsertPerson: &docs.InsertPersonRequest{PersonProperties: &docs.PersonProperties{Email: "a@example.com"}}}
	setDocsInsertRequestLocation(person, 7, "tab-1")
	if person.InsertPerson.Location.Index != 7 || person.InsertPerson.Location.TabId != "tab-1" {
		t.Fatalf("person location = %#v", person.InsertPerson.Location)
	}

	dateFormat, err := normalizeDocsDateChipFormat("iso")
	if err != nil {
		t.Fatalf("normalize date: %v", err)
	}
	if dateFormat != "DATE_FORMAT_ISO8601" {
		t.Fatalf("date format = %q", dateFormat)
	}
}

func TestDocsInsertImageBuildsPlaceholderReplacement(t *testing.T) {
	cmd := &DocsInsertImageCmd{Width: 320}
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/documents/doc1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docBodyWithText("before IMG_HERE after\n"))
	}))
	defer cleanup()

	target := docsImageTarget{anchor: "IMG_HERE", mode: docsImageAnchorReplace}
	reqs, index, tabID, err := cmd.buildInsertRequests(context.Background(), docSvc, "doc1", target, "https://example.com/i.png")
	if err != nil {
		t.Fatalf("buildInsertRequests: %v", err)
	}
	if tabID != "" || index != 8 {
		t.Fatalf("index=%d tab=%q", index, tabID)
	}
	if len(reqs) != 2 || reqs[0].DeleteContentRange == nil || reqs[1].InsertInlineImage == nil {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	if reqs[1].InsertInlineImage.Uri != "https://example.com/i.png" || reqs[1].InsertInlineImage.ObjectSize.Width.Magnitude != 320 {
		t.Fatalf("unexpected image request: %#v", reqs[1].InsertInlineImage)
	}
}

func TestDocsInsertImageBuildsNonDestructiveAnchorInsertions(t *testing.T) {
	cmd := &DocsInsertImageCmd{Width: 320}
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/documents/doc1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docBodyWithText("before IMG_HERE after\n"))
	}))
	defer cleanup()

	for _, tc := range []struct {
		name  string
		mode  docsImageAnchorMode
		index int64
	}{
		{name: "before", mode: docsImageAnchorBefore, index: 8},
		{name: "after", mode: docsImageAnchorAfter, index: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := docsImageTarget{anchor: "IMG_HERE", mode: tc.mode}
			reqs, index, _, err := cmd.buildInsertRequests(context.Background(), docSvc, "doc1", target, "https://example.com/i.png")
			if err != nil {
				t.Fatalf("buildInsertRequests: %v", err)
			}
			if index != tc.index || len(reqs) != 1 || reqs[0].InsertInlineImage == nil {
				t.Fatalf("index=%d requests=%#v", index, reqs)
			}
			if reqs[0].InsertInlineImage.Location.Index != tc.index {
				t.Fatalf("image location = %#v", reqs[0].InsertInlineImage.Location)
			}
		})
	}

	target := docsImageTarget{anchor: "IMG_HERE", mode: docsImageAnchorAfter}
	reqs, index, _, err := cmd.buildLinkFallbackRequests(context.Background(), docSvc, "doc1", target, "https://example.com/file")
	if err != nil {
		t.Fatalf("buildLinkFallbackRequests: %v", err)
	}
	if index != 16 || len(reqs) != 1 || reqs[0].InsertText == nil || reqs[0].InsertText.Location.Index != 16 {
		t.Fatalf("index=%d requests=%#v", index, reqs)
	}
}
