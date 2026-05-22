package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsPageLayoutCmd_PagelessDefault(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

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
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)

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
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

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
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)

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

func TestDocsPageLayoutCmd_EmptyDocID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)
	err := runKong(t, &DocsPageLayoutCmd{}, []string{""}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty docId") {
		t.Fatalf("expected empty docId error, got %v", err)
	}
}

func TestDocsPageLayoutCmd_InvalidLayoutRejected(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)
	err := runKong(t, &DocsPageLayoutCmd{}, []string{"doc1", "--layout=portrait"}, ctx, flags)
	if err == nil {
		t.Fatalf("expected enum validation error, got nil")
	}
}

func TestNormalizePageLayout(t *testing.T) {
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
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	newDocsService = func(context.Context, string) (*docs.Service, error) {
		t.Fatal("docs service should not be created on dry-run")
		return nil, errors.New("unexpected docs service creation")
	}

	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	ctx := newDocsJSONContext(t)

	err := (&DocsPageLayoutCmd{DocID: "doc1", Layout: "pageless"}).Run(ctx, flags)
	var exitErr *ExitError
	if err == nil {
		t.Fatalf("expected dry-run ExitError, got nil")
	}
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}
}
