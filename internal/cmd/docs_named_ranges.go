package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsNamedRangesCmd struct {
	List    DocsNamedRangesListCmd    `cmd:"" default:"withargs" help:"List named ranges"`
	Create  DocsNamedRangesCreateCmd  `cmd:"" aliases:"add,new" help:"Create a named range"`
	Delete  DocsNamedRangesDeleteCmd  `cmd:"" aliases:"rm,remove,del" help:"Delete a named range"`
	Replace DocsNamedRangesReplaceCmd `cmd:"" aliases:"set,update" help:"Replace a named range with plain text"`
}

type DocsNamedRangesListCmd struct {
	DocID string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	Name  string `name:"name" help:"Filter by exact named range name"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsNamedRangesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	tab, err := resolveTabArg(ctx, c.Tab, c.TabID)
	if err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, tab)
	if err != nil {
		return err
	}
	items, err := docsNamedRangeItemsForLoaded(loaded)
	if err != nil {
		return err
	}
	if name := strings.TrimSpace(c.Name); name != "" {
		items = filterDocsNamedRangesByName(items, name)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"documentId":  docID,
			"tabId":       loaded.tabID,
			"namedRanges": items,
		})
	}
	if len(items) == 0 {
		if !outfmt.IsPlain(ctx) {
			u.Err().Println("No named ranges")
		}
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	if !outfmt.IsPlain(ctx) {
		fmt.Fprintln(w, "NAME\tID\tSTART\tEND\tTAB_ID\tSEGMENT_ID")
	}
	for _, item := range items {
		if len(item.Ranges) == 0 {
			fmt.Fprintf(w, "%s\t%s\t\t\t\t\n", docsNamedRangeTSV(item.Name), docsNamedRangeTSV(item.NamedRangeID))
			continue
		}
		for _, span := range item.Ranges {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n",
				docsNamedRangeTSV(item.Name),
				docsNamedRangeTSV(item.NamedRangeID),
				span.StartIndex,
				span.EndIndex,
				docsNamedRangeTSV(span.TabID),
				docsNamedRangeTSV(span.SegmentID),
			)
		}
	}
	return nil
}

type DocsNamedRangesCreateCmd struct {
	DocID      string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	Name       string `name:"name" help:"Unique named range name"`
	At         string `name:"at" help:"Create the range around literal matched text"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	Start      *int64 `name:"start" help:"Range start UTF-16 index (inclusive)"`
	End        *int64 `name:"end" help:"Range end UTF-16 index (exclusive)"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsNamedRangesCreateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	name := strings.TrimSpace(c.Name)
	if docID == "" {
		return usage("empty docId")
	}
	if name == "" {
		return usage("empty --name")
	}
	if utf16Len(name) > 256 {
		return usage("--name must be at most 256 UTF-16 code units")
	}
	placement, err := docsedit.PlanRangePlacement(docsedit.RangePlacementOptions{
		Start: c.Start,
		End:   c.End,
		Anchor: docsedit.AnchorOptions{
			Text:       c.At,
			Provided:   flagProvided(kctx, "at"),
			Occurrence: c.Occurrence,
			MatchCase:  c.MatchCase,
		},
	})
	if err != nil {
		return usage(err.Error())
	}
	tab, err := resolveTabArg(ctx, c.Tab, c.TabID)
	if err != nil {
		return err
	}

	dryRunPayload := map[string]any{
		"document_id": docID,
		"name":        name,
		"tab":         tab,
	}
	if placement.Kind == docsedit.PlacementAnchor {
		addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{
			At:         placement.Anchor.Text,
			Occurrence: placement.Anchor.Occurrence,
			MatchCase:  placement.Anchor.MatchCase,
		})
	} else {
		dryRunPayload["start_index"] = placement.Range.Start
		dryRunPayload["end_index"] = placement.Range.End
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.named-range.create", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	var loaded *docsLoadedTarget
	var span docsNamedRangeSpan
	if placement.Kind == docsedit.PlacementAnchor {
		resolved, resolveErr := resolveDocsAtAnchor(ctx, svc, docID, docsAtAnchorFlags{
			At:         placement.Anchor.Text,
			Occurrence: placement.Anchor.Occurrence,
			MatchCase:  placement.Anchor.MatchCase,
			Tab:        tab,
		})
		if resolveErr != nil {
			return resolveErr
		}
		loaded = &docsLoadedTarget{full: resolved.Document, target: resolved.Document, tabID: resolved.Match.TabID}
		span = docsNamedRangeSpan{
			StartIndex: resolved.Match.StartIndex,
			EndIndex:   resolved.Match.EndIndex,
			TabID:      resolved.Match.TabID,
		}
	} else {
		loaded, err = loadDocsTargetDocument(ctx, svc, docID, tab)
		if err != nil {
			return err
		}
		span = docsNamedRangeSpan{
			StartIndex: placement.Range.Start,
			EndIndex:   placement.Range.End,
			TabID:      loaded.tabID,
		}
	}

	items, err := docsNamedRangeItemsForLoaded(loaded)
	if err != nil {
		return err
	}
	if matches := filterDocsNamedRangesByName(items, name); len(matches) > 0 {
		return usagef("named range name already exists: %q", name)
	}

	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		WriteControl: docsRequiredRevisionWriteControl(loaded.full.RevisionId),
		Requests: []*docs.Request{{
			CreateNamedRange: &docs.CreateNamedRangeRequest{
				Name: name,
				Range: &docs.Range{
					StartIndex: span.StartIndex,
					EndIndex:   span.EndIndex,
					TabId:      span.TabID,
				},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create named range: %w", err)
	}
	createdID := ""
	if resp != nil && len(resp.Replies) > 0 && resp.Replies[0] != nil && resp.Replies[0].CreateNamedRange != nil {
		createdID = strings.TrimSpace(resp.Replies[0].CreateNamedRange.NamedRangeId)
	}
	if createdID == "" {
		return fmt.Errorf("create named range: response missing namedRangeId")
	}
	created := docsNamedRangeItem{Name: name, NamedRangeID: createdID, Ranges: []docsNamedRangeSpan{span}}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"documentId": docID,
			"namedRange": created,
		})
	}
	writeDocsNamedRangeTextResult(ui.FromContext(ctx), docID, created)
	return nil
}

type DocsNamedRangesDeleteCmd struct {
	DocID    string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	NameOrID string `arg:"" name:"nameOrId" help:"Exact named range name or ID"`
	Tab      string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID    string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsNamedRangesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	in := strings.TrimSpace(c.NameOrID)
	if docID == "" {
		return usage("empty docId")
	}
	if in == "" {
		return usage("empty nameOrId")
	}
	tab, err := resolveTabArg(ctx, c.Tab, c.TabID)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.named-range.delete", map[string]any{
		"document_id": docID,
		"name_or_id":  in,
		"tab":         tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	loaded, item, err := loadAndResolveDocsNamedRange(ctx, svc, docID, tab, in)
	if err != nil {
		return err
	}
	deleteReq := &docs.DeleteNamedRangeRequest{NamedRangeId: item.NamedRangeID}
	if loaded.tabID != "" {
		deleteReq.TabsCriteria = &docs.TabsCriteria{TabIds: []string{loaded.tabID}}
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		WriteControl: docsRequiredRevisionWriteControl(loaded.full.RevisionId),
		Requests:     []*docs.Request{{DeleteNamedRange: deleteReq}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete named range: %w", err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"documentId": docID,
			"deleted":    map[string]any{"name": item.Name, "namedRangeId": item.NamedRangeID},
		})
	}
	writeDocsNamedRangeTextResult(ui.FromContext(ctx), docID, item)
	ui.FromContext(ctx).Out().Linef("deleted\ttrue")
	return nil
}

type DocsNamedRangesReplaceCmd struct {
	DocID    string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	NameOrID string `arg:"" name:"nameOrId" help:"Exact named range name or ID"`
	Text     string `name:"text" aliases:"content" help:"Plain replacement text (empty text clears the range)"`
	File     string `name:"file" help:"Plain text file path ('-' for stdin)"`
	Tab      string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID    string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsNamedRangesReplaceCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	in := strings.TrimSpace(c.NameOrID)
	if docID == "" {
		return usage("empty docId")
	}
	if in == "" {
		return usage("empty nameOrId")
	}
	text, provided, err := resolveTextInput(ctx, c.Text, c.File, kctx)
	if err != nil {
		return err
	}
	if !provided {
		return usage("required: --text or --file")
	}
	tab, err := resolveTabArg(ctx, c.Tab, c.TabID)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.named-range.replace", map[string]any{
		"document_id": docID,
		"name_or_id":  in,
		"text_length": utf16Len(text),
		"tab":         tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	loaded, item, err := loadAndResolveDocsNamedRange(ctx, svc, docID, tab, in)
	if err != nil {
		return err
	}
	replaceReq := &docs.ReplaceNamedRangeContentRequest{
		NamedRangeId: item.NamedRangeID,
		Text:         text,
	}
	if text == "" {
		replaceReq.ForceSendFields = []string{"Text"}
	}
	if loaded.tabID != "" {
		replaceReq.TabsCriteria = &docs.TabsCriteria{TabIds: []string{loaded.tabID}}
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		WriteControl: docsRequiredRevisionWriteControl(loaded.full.RevisionId),
		Requests:     []*docs.Request{{ReplaceNamedRangeContent: replaceReq}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace named range: %w", err)
	}
	updatedTab := tab
	if tab == "" && loaded.tabID != "" {
		updatedTab = loaded.tabID
	}
	updated, err := loadDocsTargetDocument(ctx, svc, docID, updatedTab)
	if err != nil {
		return fmt.Errorf("read replaced named range: %w", err)
	}
	updatedItems, err := docsNamedRangeItemsForLoaded(updated)
	if err != nil {
		return fmt.Errorf("read replaced named range: %w", err)
	}
	updatedItem, found, err := resolveDocsNamedRange(item.NamedRangeID, updatedItems)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("replaced named range not found (id=%q)", item.NamedRangeID)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"documentId": docID,
			"namedRange": updatedItem,
			"replaced":   true,
			"textLength": utf16Len(text),
		})
	}
	u := ui.FromContext(ctx)
	writeDocsNamedRangeTextResult(u, docID, updatedItem)
	u.Out().Linef("replaced\ttrue")
	u.Out().Linef("textLength\t%d", utf16Len(text))
	return nil
}

type docsNamedRangeSpan struct {
	StartIndex int64  `json:"startIndex"`
	EndIndex   int64  `json:"endIndex"`
	TabID      string `json:"tabId,omitempty"`
	SegmentID  string `json:"segmentId,omitempty"`
}

type docsNamedRangeItem struct {
	Name         string               `json:"name"`
	NamedRangeID string               `json:"namedRangeId"`
	Ranges       []docsNamedRangeSpan `json:"ranges"`
}

func loadAndResolveDocsNamedRange(ctx context.Context, svc *docs.Service, docID, tab, in string) (*docsLoadedTarget, docsNamedRangeItem, error) {
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, tab)
	if err != nil {
		return nil, docsNamedRangeItem{}, err
	}
	items, err := docsNamedRangeItemsForLoaded(loaded)
	if err != nil {
		return nil, docsNamedRangeItem{}, err
	}
	item, found, err := resolveDocsNamedRange(in, items)
	if err != nil {
		return nil, docsNamedRangeItem{}, err
	}
	if !found {
		return nil, docsNamedRangeItem{}, &ExitError{
			Code: emptyResultsExitCode,
			Err:  fmt.Errorf("named range not found: %q", in),
		}
	}
	if loaded.tabID == "" {
		scopedLoaded, scopedItem, scoped, scopeErr := scopeDocsNamedRangeToOwningTab(ctx, svc, docID, item)
		if scopeErr != nil {
			return nil, docsNamedRangeItem{}, scopeErr
		}
		if scoped {
			return scopedLoaded, scopedItem, nil
		}
	}
	return loaded, item, nil
}

func scopeDocsNamedRangeToOwningTab(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	item docsNamedRangeItem,
) (*docsLoadedTarget, docsNamedRangeItem, bool, error) {
	doc, err := svc.Documents.Get(docID).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		return nil, docsNamedRangeItem{}, false, err
	}
	if doc == nil || len(doc.Tabs) == 0 {
		return nil, docsNamedRangeItem{}, false, nil
	}

	var matchedLoaded *docsLoadedTarget
	var matchedItem docsNamedRangeItem
	for _, tab := range flattenTabs(doc.Tabs) {
		if tab == nil || tab.TabProperties == nil || tab.DocumentTab == nil {
			continue
		}
		tabID := strings.TrimSpace(tab.TabProperties.TabId)
		if tabID == "" {
			continue
		}
		loaded := &docsLoadedTarget{full: doc, tabID: tabID}
		items, itemsErr := docsNamedRangeItemsForLoaded(loaded)
		if itemsErr != nil {
			return nil, docsNamedRangeItem{}, false, itemsErr
		}
		candidate, found, resolveErr := resolveDocsNamedRange(item.NamedRangeID, items)
		if resolveErr != nil {
			return nil, docsNamedRangeItem{}, false, resolveErr
		}
		if !found {
			continue
		}
		if matchedLoaded != nil {
			return nil, docsNamedRangeItem{}, false, fmt.Errorf(
				"named range ID %q exists in multiple tabs; pass --tab",
				item.NamedRangeID,
			)
		}
		matchedLoaded = loaded
		matchedItem = candidate
	}
	if matchedLoaded == nil {
		return nil, docsNamedRangeItem{}, false, nil
	}
	return matchedLoaded, matchedItem, true, nil
}

func docsNamedRangeItemsForLoaded(loaded *docsLoadedTarget) ([]docsNamedRangeItem, error) {
	if loaded == nil || loaded.full == nil {
		return nil, fmt.Errorf("document not loaded")
	}
	namedRanges := loaded.full.NamedRanges
	if loaded.tabID != "" {
		tab, err := findTab(flattenTabs(loaded.full.Tabs), loaded.tabID)
		if err != nil {
			return nil, err
		}
		if tab.DocumentTab == nil {
			return nil, fmt.Errorf("tab has no document content: %s", loaded.tabID)
		}
		namedRanges = tab.DocumentTab.NamedRanges
	}

	items := make([]docsNamedRangeItem, 0)
	for mapName, group := range namedRanges {
		groupName := strings.TrimSpace(group.Name)
		if groupName == "" {
			groupName = strings.TrimSpace(mapName)
		}
		for _, namedRange := range group.NamedRanges {
			if namedRange == nil {
				continue
			}
			name := strings.TrimSpace(namedRange.Name)
			if name == "" {
				name = groupName
			}
			item := docsNamedRangeItem{
				Name:         name,
				NamedRangeID: strings.TrimSpace(namedRange.NamedRangeId),
				Ranges:       make([]docsNamedRangeSpan, 0, len(namedRange.Ranges)),
			}
			for _, span := range namedRange.Ranges {
				if span == nil {
					continue
				}
				item.Ranges = append(item.Ranges, docsNamedRangeSpan{
					StartIndex: span.StartIndex,
					EndIndex:   span.EndIndex,
					TabID:      strings.TrimSpace(span.TabId),
					SegmentID:  strings.TrimSpace(span.SegmentId),
				})
			}
			sort.Slice(item.Ranges, func(i, j int) bool {
				if item.Ranges[i].TabID != item.Ranges[j].TabID {
					return item.Ranges[i].TabID < item.Ranges[j].TabID
				}
				if item.Ranges[i].SegmentID != item.Ranges[j].SegmentID {
					return item.Ranges[i].SegmentID < item.Ranges[j].SegmentID
				}
				if item.Ranges[i].StartIndex != item.Ranges[j].StartIndex {
					return item.Ranges[i].StartIndex < item.Ranges[j].StartIndex
				}
				return item.Ranges[i].EndIndex < item.Ranges[j].EndIndex
			})
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].NamedRangeID < items[j].NamedRangeID
	})
	return items, nil
}

func filterDocsNamedRangesByName(items []docsNamedRangeItem, name string) []docsNamedRangeItem {
	filtered := make([]docsNamedRangeItem, 0)
	for _, item := range items {
		if item.Name == name {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func resolveDocsNamedRange(in string, items []docsNamedRangeItem) (docsNamedRangeItem, bool, error) {
	for _, item := range items {
		if item.NamedRangeID == in {
			return item, true, nil
		}
	}
	matches := filterDocsNamedRangesByName(items, in)
	switch len(matches) {
	case 0:
		return docsNamedRangeItem{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, item.NamedRangeID)
		}
		sort.Strings(ids)
		return docsNamedRangeItem{}, false, usagef("ambiguous named range %q; use ID: %s", in, strings.Join(ids, ", "))
	}
}

func docsNamedRangeTSV(value string) string {
	return strings.NewReplacer(
		"\t", `\t`,
		"\r", `\r`,
		"\n", `\n`,
	).Replace(value)
}

func writeDocsNamedRangeTextResult(u *ui.UI, docID string, item docsNamedRangeItem) {
	u.Out().Linef("documentId\t%s", docsNamedRangeTSV(docID))
	u.Out().Linef("name\t%s", docsNamedRangeTSV(item.Name))
	u.Out().Linef("namedRangeId\t%s", docsNamedRangeTSV(item.NamedRangeID))
	u.Out().Linef("rangeCount\t%d", len(item.Ranges))
	for i, span := range item.Ranges {
		prefix := fmt.Sprintf("range%d", i+1)
		u.Out().Linef("%sStartIndex\t%d", prefix, span.StartIndex)
		u.Out().Linef("%sEndIndex\t%d", prefix, span.EndIndex)
		u.Out().Linef("%sTabId\t%s", prefix, docsNamedRangeTSV(span.TabID))
		u.Out().Linef("%sSegmentId\t%s", prefix, docsNamedRangeTSV(span.SegmentID))
	}
}
