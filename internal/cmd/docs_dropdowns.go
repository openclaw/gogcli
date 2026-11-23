package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// Dropdown definition and option IDs are opaque; these prefixes are what the
// Docs API returns today and only decide whether a lookup by title is needed.
const (
	docsDropdownDefinitionIDPrefix = "kix."
	docsDropdownOptionIDPrefix     = "dropdownItem."
)

type DocsInsertDropdownCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Definition string `name:"definition" required:"" help:"Dropdown definition ID or title (see docs list-dropdowns)"`
	Option     string `name:"option" help:"Option ID or display value to select (default: the definition's first option)"`
	Index      *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	At         string `name:"at" help:"Anchor by literal text, delete the match, and insert the dropdown there"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	AtEnd      bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Batch      string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

type DocsListDropdownsCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Only list dropdowns defined in this tab (title or ID)"`
}

type docsDropdownDefinitionInfo struct {
	TabID   string                   `json:"tabId,omitempty"`
	ID      string                   `json:"id"`
	Title   string                   `json:"title,omitempty"`
	Options []docsDropdownOptionInfo `json:"options"`
}

type docsDropdownOptionInfo struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func (c *DocsInsertDropdownCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	definition := strings.TrimSpace(c.Definition)
	if definition == "" {
		return usage("empty --definition")
	}
	option := strings.TrimSpace(c.Option)
	loc := docsInsertLocationFlags{
		docID:      c.DocID,
		index:      c.Index,
		at:         c.At,
		atProvided: flagProvided(kctx, "at"),
		occurrence: c.Occurrence,
		matchCase:  c.MatchCase,
		replaceAt:  true,
		atEnd:      c.AtEnd,
		tab:        c.Tab,
		batch:      c.Batch,
	}
	payload := map[string]any{"definition": definition}
	if option != "" {
		payload["option"] = option
	}

	if docsDropdownNeedsLookup(definition, option) {
		if dryRunErr := dryRunDocsSingleInsert(ctx, flags, "docs.insert-dropdown", loc, payload); dryRunErr != nil {
			return dryRunErr
		}
		svc, err := requireDocsService(ctx, flags)
		if err != nil {
			return err
		}
		defs, err := fetchDocsDropdownDefinitions(ctx, svc, strings.TrimSpace(c.DocID), c.Tab, true)
		if err != nil {
			return err
		}
		def, err := resolveDocsDropdownDefinition(defs, definition)
		if err != nil {
			return err
		}
		definition = def.ID
		if option != "" {
			opt, optErr := resolveDocsDropdownOption(def, option)
			if optErr != nil {
				return optErr
			}
			option = opt.ID
		}
		payload["definitionId"] = definition
		if option != "" {
			payload["optionId"] = option
		}
	}

	req := &docs.Request{InsertDropdown: &docs.InsertDropdownRequest{
		DropdownDefinitionId: definition,
		SelectedOptionId:     option,
	}}
	return runDocsSingleInsert(ctx, flags, "docs.insert-dropdown", loc, req, payload)
}

func (c *DocsListDropdownsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	defs, err := fetchDocsDropdownDefinitions(ctx, svc, docID, c.Tab, false)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"documentId": docID, "dropdowns": defs})
	}
	u.Out().Linef("TAB\tDEFINITION\tTITLE\tOPTION\tVALUE")
	for _, def := range defs {
		if len(def.Options) == 0 {
			u.Out().Linef("%s\t%s\t%s\t\t", def.TabID, def.ID, def.Title)
			continue
		}
		for _, opt := range def.Options {
			u.Out().Linef("%s\t%s\t%s\t%s\t%s", def.TabID, def.ID, def.Title, opt.ID, opt.Value)
		}
	}
	return nil
}

func docsDropdownNeedsLookup(definition, option string) bool {
	if !strings.HasPrefix(definition, docsDropdownDefinitionIDPrefix) {
		return true
	}
	return option != "" && !strings.HasPrefix(option, docsDropdownOptionIDPrefix)
}

// Definition IDs are only unique within a tab, so an insert resolves names in
// the tab it writes to: --tab, or the first tab, which is where the API writes
// when no tab is given.
func fetchDocsDropdownDefinitions(ctx context.Context, svc *docs.Service, docID, tabQuery string, insertTarget bool) ([]docsDropdownDefinitionInfo, error) {
	doc, err := svc.Documents.Get(docID).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, docsNotFoundError(docID, err)
		}
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("doc not found")
	}
	tabs := flattenTabs(doc.Tabs)
	if strings.TrimSpace(tabQuery) != "" {
		tab, tabErr := findTab(tabs, tabQuery)
		if tabErr != nil {
			return nil, tabErr
		}
		tabs = []*docs.Tab{tab}
	} else if insertTarget && len(tabs) > 0 {
		tabs = tabs[:1]
	}
	return docsDropdownDefinitionsFromTabs(tabs), nil
}

func docsDropdownDefinitionsFromTabs(tabs []*docs.Tab) []docsDropdownDefinitionInfo {
	var out []docsDropdownDefinitionInfo
	for _, tab := range tabs {
		if tab == nil || tab.DocumentTab == nil {
			continue
		}
		tabID := ""
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
		}
		ids := make([]string, 0, len(tab.DocumentTab.DropdownDefinitions))
		for id := range tab.DocumentTab.DropdownDefinitions {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			def := tab.DocumentTab.DropdownDefinitions[id]
			info := docsDropdownDefinitionInfo{TabID: tabID, ID: id, Options: []docsDropdownOptionInfo{}}
			if def.DropdownDefinitionId != "" {
				info.ID = def.DropdownDefinitionId
			}
			if props := def.DropdownDefinitionProperties; props != nil {
				info.Title = props.Title
				for _, opt := range props.Options {
					if opt == nil {
						continue
					}
					info.Options = append(info.Options, docsDropdownOptionInfo{ID: opt.OptionId, Value: opt.DisplayValue})
				}
			}
			out = append(out, info)
		}
	}
	return out
}

func resolveDocsDropdownDefinition(defs []docsDropdownDefinitionInfo, query string) (docsDropdownDefinitionInfo, error) {
	for _, def := range defs {
		if def.ID == query {
			return def, nil
		}
	}
	var matches []docsDropdownDefinitionInfo
	for _, def := range defs {
		if def.Title != "" && strings.EqualFold(def.Title, query) {
			matches = append(matches, def)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return docsDropdownDefinitionInfo{}, usage(fmt.Sprintf("dropdown definition not found: %q (see docs list-dropdowns)", query))
	default:
		return docsDropdownDefinitionInfo{}, usage(fmt.Sprintf("dropdown title %q matches %d definitions; pass the definition ID", query, len(matches)))
	}
}

func resolveDocsDropdownOption(def docsDropdownDefinitionInfo, query string) (docsDropdownOptionInfo, error) {
	for _, opt := range def.Options {
		if opt.ID == query {
			return opt, nil
		}
	}
	for _, opt := range def.Options {
		if strings.EqualFold(opt.Value, query) {
			return opt, nil
		}
	}
	values := make([]string, 0, len(def.Options))
	for _, opt := range def.Options {
		values = append(values, fmt.Sprintf("%q", opt.Value))
	}
	return docsDropdownOptionInfo{}, usage(fmt.Sprintf("option %q not in dropdown %s (available: %s)", query, def.ID, strings.Join(values, ", ")))
}
