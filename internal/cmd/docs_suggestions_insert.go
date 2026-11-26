package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
)

type DocsSuggestionsInsertCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	Text     string `arg:"" name:"text" help:"Plain text to suggest"`
	Index    int64  `required:"" help:"Docs UTF-16 offset in the first tab body (minimum 1)"`
	Revision string `required:"" help:"Required document revision ID from the same read as the index"`
}

func (c *DocsSuggestionsInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if id == "" {
		return usage("empty docId")
	}
	if c.Text == "" {
		return usage("text must not be empty")
	}
	if c.Index < 1 {
		return usage("--index must be >= 1")
	}
	if strings.TrimSpace(c.Revision) == "" {
		return usage("--revision must not be empty")
	}

	// Do not use the ordinary edit or batch queue helpers: they don't carry
	// suggestion mode. Pin the revision so stale indices are not rebased.
	req := &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{WriteMode: "SUGGEST", RequiredRevisionId: c.Revision},
		Requests: []*docs.Request{{InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: c.Index},
			Text:     c.Text,
		}}},
	}
	if err := dryRunExit(ctx, flags, "docs.suggestions.insert", map[string]any{
		"documentId": id,
		"body":       req,
	}); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := svc.Documents.BatchUpdate(id, req).Context(googleapi.WithoutRetries(ctx)).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return docsNotFoundError(id, err)
		}
		return fmt.Errorf("suggestion request failed; inspect the document before retrying: %w", err)
	}

	var ids []string
	if resp != nil && resp.DocumentId == id && resp.CommentUpdateState == "ALL_SAVED" && len(resp.SuggestionResponses) == 1 && resp.SuggestionResponses[0] != nil {
		ids = sortedUniqueStrings(resp.SuggestionResponses[0].CreatedSuggestionIds)
	}
	if len(ids) == 0 {
		return fmt.Errorf("the response did not confirm creation of a suggestion; inspect the document before retrying")
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"documentId": id, "suggestionIds": ids})
	}
	for _, suggestionID := range ids {
		if outfmt.IsPlain(ctx) {
			fmt.Fprintln(stdoutWriter(ctx), suggestionID)
		} else {
			fmt.Fprintf(stdoutWriter(ctx), "Created suggestion: %s\n", suggestionID)
		}
	}
	return nil
}
