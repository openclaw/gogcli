package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsStructuralCommandsBuildExpectedRequests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cmd    any
		args   []string
		assert func(*testing.T, []*docs.Request)
	}{
		{
			name: "section break",
			cmd:  &DocsSectionBreakCmd{},
			args: []string{"doc1", "--index", "7", "--type", "continuous"},
			assert: func(t *testing.T, requests []*docs.Request) {
				t.Helper()
				req := requests[0].InsertSectionBreak
				if req == nil || req.Location.Index != 7 || req.SectionType != "CONTINUOUS" {
					t.Fatalf("unexpected section break: %#v", requests)
				}
			},
		},
		{
			name: "horizontal rule",
			cmd:  &DocsHorizontalRuleCmd{},
			args: []string{"doc1", "--index", "8"},
			assert: func(t *testing.T, requests []*docs.Request) {
				t.Helper()
				if len(requests) != 2 || requests[0].InsertText == nil || requests[0].InsertText.Text != "\n" || requests[1].UpdateParagraphStyle == nil {
					t.Fatalf("unexpected horizontal rule: %#v", requests)
				}
				if requests[1].UpdateParagraphStyle.Fields != "borderBottom" {
					t.Fatalf("unexpected border request: %#v", requests[1])
				}
			},
		},
		{
			name: "section columns",
			cmd:  &DocsSectionColumnsCmd{},
			args: []string{"doc1", "--index", "9", "--count", "3", "--separator", "between"},
			assert: func(t *testing.T, requests []*docs.Request) {
				t.Helper()
				req := requests[0].UpdateSectionStyle
				if req == nil || req.Range.StartIndex != 9 || req.Range.EndIndex != 10 {
					t.Fatalf("unexpected section columns range: %#v", requests)
				}
				if len(req.SectionStyle.ColumnProperties) != 3 || req.SectionStyle.ColumnSeparatorStyle != "BETWEEN_EACH_COLUMN" {
					t.Fatalf("unexpected section style: %#v", req.SectionStyle)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var captured []*docs.Request
			svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
					var body docs.BatchUpdateDocumentRequest
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatalf("decode request: %v", err)
					}
					captured = body.Requests
					_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
				default:
					http.NotFound(w, r)
				}
			}))
			defer cleanup()
			ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
			if err := runKong(t, tt.cmd, tt.args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatalf("run: %v", err)
			}
			tt.assert(t, captured)
		})
	}
}

func TestDocsInsertFootnoteCreatesThenPopulatesSegment(t *testing.T) {
	t.Parallel()
	svc, calls, cleanup := newDocsCreateAndPopulateService(t, map[string]any{
		"createFootnote": map[string]any{"footnoteId": "fn-1"},
	})
	defer cleanup()

	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &DocsFootnoteCmd{}, []string{"doc1", "--index", "5", "--text", "Footnote body"}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("insert footnote: %v", err)
	}
	if len(*calls) != 2 || (*calls)[0][0].CreateFootnote == nil || (*calls)[0][0].CreateFootnote.Location.Index != 5 {
		t.Fatalf("create requests = %#v", *calls)
	}
	insert := (*calls)[1][0].InsertText
	if insert == nil || insert.Text != "Footnote body" || insert.EndOfSegmentLocation == nil || insert.EndOfSegmentLocation.SegmentId != "fn-1" {
		t.Fatalf("populate request = %#v", (*calls)[1])
	}
}

func TestDocsSectionColumnsRejectsUnsupportedCountBeforeAuth(t *testing.T) {
	t.Parallel()
	err := runKong(
		t,
		&DocsSectionColumnsCmd{},
		[]string{"doc1", "--count", "4"},
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		&RootFlags{Account: "a@b.com"},
	)
	if err == nil || !strings.Contains(err.Error(), "between 1 and 3") {
		t.Fatalf("unexpected error: %v", err)
	}
}
