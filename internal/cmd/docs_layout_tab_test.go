package cmd

import "testing"

// buildUpdateDocumentStyleRequest must carry the TabId through to the Docs API
// so document-style changes (e.g. pageless) can target a non-default tab.
// Regression guard for: page-layout/write --pageless silently hitting only the
// default tab in multi-tab docs.
func TestBuildUpdateDocumentStyleRequest_TabID(t *testing.T) {
	t.Parallel()

	req, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		Mode:  docsDocumentModePageless,
		TabID: "t.abc123",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.TabId != "t.abc123" {
		t.Fatalf("expected TabId t.abc123, got %q", req.TabId)
	}
	if req.DocumentStyle == nil || req.DocumentStyle.DocumentFormat == nil ||
		req.DocumentStyle.DocumentFormat.DocumentMode != docsDocumentModePageless {
		t.Fatalf("expected pageless documentMode, got %#v", req.DocumentStyle)
	}
}

// Default behaviour (no tab) must leave TabId empty so the request applies to
// the document's default tab, preserving existing single-tab behaviour.
func TestBuildUpdateDocumentStyleRequest_NoTabID(t *testing.T) {
	t.Parallel()

	req, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		Mode: docsDocumentModePageless,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.TabId != "" {
		t.Fatalf("expected empty TabId, got %q", req.TabId)
	}
}
