package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DocsPageLayoutCmd toggles the page layout on an existing Google Doc.
// The Docs UI exposes this via File → Page setup → Pageless/Pages. The Docs
// API exposes it via documents.batchUpdate with updateDocumentStyle.
//
// Sibling to the --pageless flag on `docs create` / `docs write` for the case
// where the doc already exists (e.g. created by Drive markdown conversion in
// an upstream step that didn't set the layout).
type DocsPageLayoutCmd struct {
	DocID       string          `arg:"" name:"docId" help:"Doc ID"`
	Layout      string          `name:"layout" enum:"pageless,pages,paged" default:"pageless" help:"Page layout: pageless or pages"`
	LayoutFlags DocsLayoutFlags `embed:""`
}

func (c *DocsPageLayoutCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}

	mode := ""
	if flagProvided(kctx, "layout") || !c.LayoutFlags.any() {
		var err error
		mode, err = normalizePageLayout(c.Layout)
		if err != nil {
			return err
		}
	}

	dryRunPayload := map[string]any{
		"documentId": docID,
	}
	if mode != "" {
		dryRunPayload["layout"] = c.Layout
		dryRunPayload["mode"] = mode
	}
	for k, v := range c.LayoutFlags.dryRunPayload() {
		dryRunPayload[k] = v
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.page-layout", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	if err := setDocumentStyle(ctx, svc, docID, docsDocumentStyleOptions{
		Mode:            mode,
		DocsLayoutFlags: c.LayoutFlags,
	}); err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return fmt.Errorf("set page layout: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
		}
		if mode != "" {
			payload["layout"] = c.Layout
			payload["mode"] = mode
		}
		for k, v := range c.LayoutFlags.dryRunPayload() {
			payload[k] = v
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Linef("documentId\t%s", docID)
	if mode != "" {
		u.Out().Linef("layout\t%s", c.Layout)
	}
	for k, v := range c.LayoutFlags.dryRunPayload() {
		u.Out().Linef("%s\t%s", k, v)
	}
	return nil
}

func normalizePageLayout(layout string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(layout)) {
	case "pageless":
		return docsDocumentModePageless, nil
	case "paged", "pages":
		return docsDocumentModePages, nil
	case "":
		return "", usage("empty --layout (expected pageless or pages)")
	default:
		return "", usage(fmt.Sprintf("invalid --layout %q (expected pageless or pages)", layout))
	}
}
