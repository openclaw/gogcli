package cmd

import (
	"context"
	"strings"

	"github.com/openclaw/gogcli/internal/ui"
)

// resolveTabArg returns the effective tab value from --tab or the deprecated
// --tab-id flag. It rejects supplying both and emits a deprecation warning
// when --tab-id is used.
func resolveTabArg(ctx context.Context, tab, tabID string) (string, error) {
	tab = strings.TrimSpace(tab)
	tabID = strings.TrimSpace(tabID)
	if tab != "" && tabID != "" {
		return "", usage("--tab and --tab-id are mutually exclusive (--tab-id is deprecated; use --tab)")
	}
	if tabID != "" {
		u := ui.FromContext(ctx)
		u.Err().Linef("Warning: --tab-id is deprecated; use --tab instead")
		return tabID, nil
	}
	return tab, nil
}

type DocsEditCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Find       string `arg:"" name:"find" help:"Text to find"`
	ReplaceStr string `arg:"" name:"replace" help:"Replacement text"`
	MatchCase  bool   `name:"match-case" help:"Case-sensitive matching"`
}

func (c *DocsEditCmd) Run(ctx context.Context, flags *RootFlags) error {
	return (&DocsFindReplaceCmd{
		DocID:       c.DocID,
		Find:        c.Find,
		ReplaceText: c.ReplaceStr,
		MatchCase:   c.MatchCase,
	}).Run(ctx, flags)
}
