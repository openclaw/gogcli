package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/docs/v1"
)

type DocsSuggestionsInsertCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	Text     string `arg:"" name:"text" help:"Plain text to suggest"`
	Index    int64  `required:"" help:"Docs UTF-16 offset in the selected tab body (minimum 1)"`
	TabID    string `name:"tab-id" help:"Exact tab ID (defaults to the first tab)"`
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

	return runDocsSuggestions(ctx, flags, "docs.suggestions.insert", id, c.Revision, []*docs.Request{{InsertText: &docs.InsertTextRequest{
		Location: &docs.Location{Index: c.Index, TabId: c.TabID},
		Text:     c.Text,
	}}})
}
