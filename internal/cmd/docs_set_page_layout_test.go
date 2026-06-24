package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsPageLayoutCmd_PagelessDefault(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request
	var targetDocID string

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			// Capture the doc ID from the path: /v1/documents/{id}:batchUpdate
			path := strings.TrimPrefix(r.URL.Path, "/v1/documents/")
			targetDocID = strings.TrimSuffix(path, ":batchUpdate")
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	if err := runKong(t, &DocsPageLayoutCmd{}, []string{"doc1"}, ctx, flags); err != nil {
		t.Fatalf("page-layout: %v", err)
	}

	if targetDocID != "doc1" {
		t.Fatalf("expected batchUpdate on doc1, got %q", targetDocID)
	}
	if len(batchRequests) != 1 || len(batchRequests[0]) != 1 {
		t.Fatalf("expected 1 batch request with 1 op, got %#v", batchRequests)
	}
	upd := batchRequests[0][0].UpdateDocumentStyle
	if upd == nil {
		t.Fatalf("expected UpdateDocumentStyle, got %#v", batchRequests[0][0])
	}
	if upd.Fields != "documentFormat" {
		t.Fatalf("expected fields=documentFormat, got %q", upd.Fields)
	}
	if upd.DocumentStyle == nil || upd.DocumentStyle.DocumentFormat == nil {
		t.Fatalf("expected DocumentStyle.DocumentFormat, got %#v", upd.DocumentStyle)
	}
	if upd.DocumentStyle.DocumentFormat.DocumentMode != docsDocumentModePageless {
		t.Fatalf("expected documentMode=PAGELESS, got %q", upd.DocumentStyle.DocumentFormat.DocumentMode)
	}
}

func TestDocsPageLayoutCmd_Pages(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	if err := runKong(t, &DocsPageLayoutCmd{}, []string{"doc1", "--layout=pages"}, ctx, flags); err != nil {
		t.Fatalf("page-layout pages: %v", err)
	}

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(batchRequests))
	}
	upd := batchRequests[0][0].UpdateDocumentStyle
	if upd == nil || upd.DocumentStyle == nil || upd.DocumentStyle.DocumentFormat == nil {
		t.Fatalf("unexpected request shape: %#v", batchRequests[0][0])
	}
	if upd.DocumentStyle.DocumentFormat.DocumentMode != docsDocumentModePages {
		t.Fatalf("expected documentMode=PAGES, got %q", upd.DocumentStyle.DocumentFormat.DocumentMode)
	}
}

func TestDocsPageLayoutCmd_TabTitleTargetsResolvedTab(t *testing.T) {
	t.Parallel()

	var update *docs.UpdateDocumentStyleRequest
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/documents/doc1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"tabs": []map[string]any{{
					"tabProperties": map[string]any{"tabId": "t.secondary", "title": "Secondary"},
					"documentTab": map[string]any{
						"body": map[string]any{"content": []map[string]any{{"startIndex": 0, "endIndex": 2}}},
					},
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(req.Requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(req.Requests))
			}
			update = req.Requests[0].UpdateDocumentStyle
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)
	if err := runKong(t, &DocsPageLayoutCmd{}, []string{"doc1", "--tab", "Secondary"}, ctx, flags); err != nil {
		t.Fatalf("page-layout tab: %v", err)
	}
	if update == nil {
		t.Fatal("missing UpdateDocumentStyle request")
	}
	if update.TabId != "t.secondary" {
		t.Fatalf("tabId = %q, want t.secondary", update.TabId)
	}
}

func TestDocsPageLayoutCmd_EmptyDocID(t *testing.T) {
	t.Parallel()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	err := runKong(t, &DocsPageLayoutCmd{}, []string{""}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty docId") {
		t.Fatalf("expected empty docId error, got %v", err)
	}
}

func TestDocsPageLayoutCmd_InvalidLayoutRejected(t *testing.T) {
	t.Parallel()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	err := runKong(t, &DocsPageLayoutCmd{}, []string{"doc1", "--layout=portrait"}, ctx, flags)
	if err == nil {
		t.Fatalf("expected enum validation error, got nil")
	}
}

func TestNormalizePageLayout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"pageless", docsDocumentModePageless, false},
		{"PAGELESS", docsDocumentModePageless, false},
		{"paged", docsDocumentModePages, false},
		{"pages", docsDocumentModePages, false},
		{"  Paged  ", docsDocumentModePages, false},
		{"", "", true},
		{"weird", "", true},
	}
	for _, tc := range cases {
		got, err := normalizePageLayout(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizePageLayout(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizePageLayout(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizePageLayout(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDocsPageLayoutCmd_DryRun(t *testing.T) {
	t.Parallel()

	ctx := withDocsTestServiceFactory(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*docs.Service, error) {
		t.Fatal("docs service should not be created on dry-run")
		return nil, errors.New("unexpected docs service creation")
	})

	flags := &RootFlags{Account: "a@b.com", DryRun: true}

	err := (&DocsPageLayoutCmd{DocID: "doc1", Layout: "pageless"}).Run(ctx, nil, flags)
	var exitErr *ExitError
	if err == nil {
		t.Fatalf("expected dry-run ExitError, got nil")
	}
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}
}

func TestDocsPageLayoutCmd_PageSizeAndMargins(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	args := []string{"doc1", "--layout=pages", "--page-width=8.5in", "--page-height=11in", "--margin-left=0.5in", "--margin-right=36"}
	if err := runKong(t, &DocsPageLayoutCmd{}, args, ctx, flags); err != nil {
		t.Fatalf("page-layout margins: %v", err)
	}

	if len(batchRequests) != 1 || len(batchRequests[0]) != 1 {
		t.Fatalf("expected 1 batch request with 1 op, got %#v", batchRequests)
	}
	upd := batchRequests[0][0].UpdateDocumentStyle
	if upd == nil || upd.DocumentStyle == nil {
		t.Fatalf("expected UpdateDocumentStyle, got %#v", batchRequests[0][0])
	}
	if upd.Fields != "documentFormat,pageSize.width,pageSize.height,marginLeft,marginRight" {
		t.Fatalf("fields = %q", upd.Fields)
	}
	style := upd.DocumentStyle
	if style.PageSize.Width.Magnitude != 612 || style.PageSize.Height.Magnitude != 792 {
		t.Fatalf("unexpected page size: %#v", style.PageSize)
	}
	if style.MarginLeft.Magnitude != 36 || style.MarginRight.Magnitude != 36 {
		t.Fatalf("unexpected margins: left=%#v right=%#v", style.MarginLeft, style.MarginRight)
	}
}

func TestDocsPageLayoutCmd_PageSizeWithoutLayoutPreservesMode(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	if err := runKong(t, &DocsPageLayoutCmd{}, []string{"doc1", "--page-width=960"}, ctx, flags); err != nil {
		t.Fatalf("page-layout width: %v", err)
	}

	upd := batchRequests[0][0].UpdateDocumentStyle
	if upd.Fields != "pageSize.width" {
		t.Fatalf("fields = %q", upd.Fields)
	}
	if upd.DocumentStyle.DocumentFormat != nil {
		t.Fatalf("unexpected document mode update: %#v", upd.DocumentStyle.DocumentFormat)
	}
	if upd.DocumentStyle.PageSize.Width.Magnitude != 960 {
		t.Fatalf("page width = %#v", upd.DocumentStyle.PageSize.Width)
	}
}

func TestBuildUpdateDocumentStyleRequest_ZeroMarginAllowed(t *testing.T) {
	t.Parallel()

	req, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		DocsLayoutFlags: DocsLayoutFlags{MarginLeft: "0", MarginRight: "0pt"},
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.Fields != "marginLeft,marginRight" {
		t.Fatalf("fields = %q", req.Fields)
	}
	if req.DocumentStyle.MarginLeft.Magnitude != 0 || req.DocumentStyle.MarginRight.Magnitude != 0 {
		t.Fatalf("margins = left %#v right %#v", req.DocumentStyle.MarginLeft, req.DocumentStyle.MarginRight)
	}
	if len(req.DocumentStyle.MarginLeft.ForceSendFields) == 0 || req.DocumentStyle.MarginLeft.ForceSendFields[0] != "Magnitude" {
		t.Fatalf("left margin should force-send zero magnitude: %#v", req.DocumentStyle.MarginLeft)
	}
}
