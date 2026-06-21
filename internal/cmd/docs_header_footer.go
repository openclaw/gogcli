package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsHeaderCmd struct {
	List   DocsHeaderListCmd   `cmd:"" name:"list" aliases:"ls" help:"List headers and their segment IDs"`
	Create DocsHeaderCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create and optionally populate a header"`
	Delete DocsHeaderDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a header"`
}

type DocsFooterCmd struct {
	List   DocsFooterListCmd   `cmd:"" name:"list" aliases:"ls" help:"List footers and their segment IDs"`
	Create DocsFooterCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create and optionally populate a footer"`
	Delete DocsFooterDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a footer"`
}

type DocsHeaderListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Limit results to a tab title or ID"`
}

type DocsFooterListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Limit results to a tab title or ID"`
}

type DocsHeaderCreateCmd struct {
	DocID     string                 `arg:"" name:"docId" help:"Doc ID"`
	Text      string                 `name:"text" help:"Initial header text"`
	File      string                 `name:"file" help:"Read initial header text from a file ('-' for stdin)"`
	Placement DocsBodyPlacementFlags `embed:""`
}

type DocsFooterCreateCmd struct {
	DocID     string                 `arg:"" name:"docId" help:"Doc ID"`
	Text      string                 `name:"text" help:"Initial footer text"`
	File      string                 `name:"file" help:"Read initial footer text from a file ('-' for stdin)"`
	Placement DocsBodyPlacementFlags `embed:""`
}

type DocsHeaderDeleteCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	SegmentID string `arg:"" name:"headerId" help:"Exact header segment ID"`
	Tab       string `name:"tab" help:"Tab title or ID containing the header"`
}

type DocsFooterDeleteCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	SegmentID string `arg:"" name:"footerId" help:"Exact footer segment ID"`
	Tab       string `name:"tab" help:"Tab title or ID containing the footer"`
}

type docsSegmentListItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	TabID  string `json:"tabId,omitempty"`
	Length int64  `json:"length"`
}

func (c *DocsHeaderListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsSegmentList(ctx, flags, c.DocID, c.Tab, docsSegmentKindHeader)
}

func (c *DocsFooterListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsSegmentList(ctx, flags, c.DocID, c.Tab, docsSegmentKindFooter)
}

func runDocsSegmentList(ctx context.Context, flags *RootFlags, rawDocID, tabQuery, kind string) error {
	docID := normalizeGoogleID(strings.TrimSpace(rawDocID))
	if docID == "" {
		return usage("empty docId")
	}
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	doc, err := svc.Documents.Get(docID).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		return err
	}
	items, err := collectDocsSegments(doc, tabQuery, kind)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		key := kind + "s"
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"documentId": docID, key: items})
	}
	u := ui.FromContext(ctx)
	for _, item := range items {
		u.Out().Linef("%s\t%s\t%s\t%d", item.ID, item.Type, item.TabID, item.Length)
	}
	return nil
}

func collectDocsSegments(doc *docs.Document, tabQuery, kind string) ([]docsSegmentListItem, error) {
	items := make([]docsSegmentListItem, 0)
	add := func(tabID string, content map[string][]*docs.StructuralElement) {
		for id, elements := range content {
			items = append(items, docsSegmentListItem{ID: id, Type: kind, TabID: tabID, Length: docsStructuralContentEnd(elements)})
		}
	}
	collectTab := func(tab *docs.Tab) {
		if tab == nil || tab.DocumentTab == nil {
			return
		}
		tabID := ""
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
		}
		add(tabID, docsSegmentElements(tab.DocumentTab, kind))
	}
	if strings.TrimSpace(tabQuery) != "" {
		tab, err := findTab(flattenTabs(doc.Tabs), tabQuery)
		if err != nil {
			return nil, err
		}
		collectTab(tab)
	} else {
		for _, tab := range flattenTabs(doc.Tabs) {
			collectTab(tab)
		}
		add("", docsLegacySegmentElements(doc, kind))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TabID != items[j].TabID {
			return items[i].TabID < items[j].TabID
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func docsSegmentElements(tab *docs.DocumentTab, kind string) map[string][]*docs.StructuralElement {
	result := make(map[string][]*docs.StructuralElement)
	switch kind {
	case docsSegmentKindHeader:
		for id, segment := range tab.Headers {
			result[id] = segment.Content
		}
	case docsSegmentKindFooter:
		for id, segment := range tab.Footers {
			result[id] = segment.Content
		}
	}
	return result
}

func docsLegacySegmentElements(doc *docs.Document, kind string) map[string][]*docs.StructuralElement {
	return docsSegmentElements(&docs.DocumentTab{Headers: doc.Headers, Footers: doc.Footers}, kind)
}

func docsStructuralContentEnd(content []*docs.StructuralElement) int64 {
	var end int64
	for _, element := range content {
		if element != nil && element.EndIndex > end {
			end = element.EndIndex
		}
	}
	return end
}

func (c *DocsHeaderCreateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	return runDocsSegmentCreate(ctx, kctx, flags, c.DocID, c.Text, c.File, c.Placement, docsSegmentKindHeader)
}

func (c *DocsFooterCreateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	return runDocsSegmentCreate(ctx, kctx, flags, c.DocID, c.Text, c.File, c.Placement, docsSegmentKindFooter)
}

func runDocsSegmentCreate(ctx context.Context, kctx *kong.Context, flags *RootFlags, rawDocID, textFlag, fileFlag string, placementFlags DocsBodyPlacementFlags, kind string) error {
	docID := normalizeGoogleID(strings.TrimSpace(rawDocID))
	if docID == "" {
		return usage("empty docId")
	}
	text, provided, err := resolveTextInput(ctx, textFlag, fileFlag, kctx)
	if err != nil {
		return err
	}
	hasPlacement := placementFlags.Index != nil || placementFlags.AtEnd || flagProvided(kctx, "at")
	var placementPayload map[string]any
	if hasPlacement {
		placement, planErr := placementFlags.plan(kctx)
		if planErr != nil {
			return planErr
		}
		placementPayload = placementFlags.dryRunPayload(placement)
	} else {
		placementPayload = map[string]any{"tab": placementFlags.Tab, "section": "document-default"}
	}
	placementPayload["documentId"] = docID
	placementPayload["segmentType"] = kind
	placementPayload["textBytes"] = len(text)
	if dryRunErr := dryRunExit(ctx, flags, "docs."+kind+".create", placementPayload); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	tabID, sectionLocation, err := resolveDocsCreateSegmentLocation(ctx, kctx, svc, docID, placementFlags, hasPlacement)
	if err != nil {
		return err
	}
	request := &docs.Request{}
	if kind == docsSegmentKindHeader {
		request.CreateHeader = &docs.CreateHeaderRequest{Type: "DEFAULT", SectionBreakLocation: sectionLocation}
	} else {
		request.CreateFooter = &docs.CreateFooterRequest{Type: "DEFAULT", SectionBreakLocation: sectionLocation}
	}
	response, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: []*docs.Request{request}}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create %s: %w", kind, err)
	}
	segmentID := docsCreatedSegmentID(response, kind)
	if segmentID == "" {
		return fmt.Errorf("create %s response missing segment ID", kind)
	}
	requestCount := 1
	if provided && text != "" {
		_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: []*docs.Request{{
			InsertText: &docs.InsertTextRequest{
				EndOfSegmentLocation: &docs.EndOfSegmentLocation{SegmentId: segmentID, TabId: tabID},
				Text:                 text,
			},
		}}}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("%s %s was created but could not be populated: %w", kind, segmentID, err)
		}
		requestCount++
	}
	return writeDocsSegmentMutationResult(ctx, docID, tabID, segmentID, kind, requestCount, false)
}

func resolveDocsCreateSegmentLocation(ctx context.Context, kctx *kong.Context, svc *docs.Service, docID string, flags DocsBodyPlacementFlags, hasPlacement bool) (string, *docs.Location, error) {
	if !hasPlacement {
		if strings.TrimSpace(flags.Tab) == "" {
			return "", nil, nil
		}
		loaded, err := loadDocsTargetDocument(ctx, svc, docID, flags.Tab)
		if err != nil {
			return "", nil, err
		}
		return loaded.tabID, docsDefaultSectionLocation(loaded.tabID), nil
	}
	placement, err := flags.plan(kctx)
	if err != nil {
		return "", nil, err
	}
	resolved, err := resolveDocsPlacement(ctx, svc, docID, flags.Tab, placement)
	if err != nil {
		return "", nil, err
	}
	if resolved.InTable {
		return "", nil, usage("header/footer sections cannot be selected from inside a table")
	}
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, resolved.TabID)
	if err != nil {
		return "", nil, err
	}
	location := docsSectionBreakLocation(loaded.target, loaded.tabID, resolved.Index)
	if location == nil {
		location = docsDefaultSectionLocation(loaded.tabID)
	}
	return loaded.tabID, location, nil
}

func docsDefaultSectionLocation(tabID string) *docs.Location {
	if tabID == "" {
		return nil
	}
	location := &docs.Location{Index: 0, TabId: tabID}
	forceZeroLocationIndex(location)
	return location
}

func docsSectionBreakLocation(doc *docs.Document, tabID string, index int64) *docs.Location {
	var selected *docs.StructuralElement
	if doc != nil && doc.Body != nil {
		for _, element := range doc.Body.Content {
			if element == nil || element.SectionBreak == nil || element.StartIndex > index {
				continue
			}
			if selected == nil || element.StartIndex > selected.StartIndex {
				selected = element
			}
		}
	}
	if selected == nil {
		return nil
	}
	location := &docs.Location{Index: selected.StartIndex, TabId: tabID}
	forceZeroLocationIndex(location)
	return location
}

func docsCreatedSegmentID(response *docs.BatchUpdateDocumentResponse, kind string) string {
	if response == nil {
		return ""
	}
	for _, reply := range response.Replies {
		if reply == nil {
			continue
		}
		if kind == docsSegmentKindHeader && reply.CreateHeader != nil {
			return reply.CreateHeader.HeaderId
		}
		if kind == docsSegmentKindFooter && reply.CreateFooter != nil {
			return reply.CreateFooter.FooterId
		}
	}
	return ""
}

func (c *DocsHeaderDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsSegmentDelete(ctx, flags, c.DocID, c.Tab, c.SegmentID, docsSegmentKindHeader)
}

func (c *DocsFooterDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runDocsSegmentDelete(ctx, flags, c.DocID, c.Tab, c.SegmentID, docsSegmentKindFooter)
}

func runDocsSegmentDelete(ctx context.Context, flags *RootFlags, rawDocID, tabQuery, rawSegmentID, kind string) error {
	docID := normalizeGoogleID(strings.TrimSpace(rawDocID))
	segmentID := strings.TrimSpace(rawSegmentID)
	if docID == "" {
		return usage("empty docId")
	}
	if segmentID == "" {
		return usage("empty segment ID")
	}
	if err := dryRunAndConfirmDestructive(ctx, flags, "docs."+kind+".delete", map[string]any{
		"documentId": docID, "segmentId": segmentID, "segmentType": kind, "tab": tabQuery,
	}, fmt.Sprintf("delete %s %s from doc %s", kind, segmentID, docID)); err != nil {
		return err
	}
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	loaded, err := loadDocsTargetSegment(ctx, svc, docID, tabQuery, segmentID)
	if err != nil {
		return err
	}
	if loaded.segmentKind != kind {
		return usagef("segment %s is a %s, not a %s", segmentID, loaded.segmentKind, kind)
	}
	request := &docs.Request{}
	if kind == docsSegmentKindHeader {
		request.DeleteHeader = &docs.DeleteHeaderRequest{HeaderId: segmentID, TabId: loaded.tabID}
	} else {
		request.DeleteFooter = &docs.DeleteFooterRequest{FooterId: segmentID, TabId: loaded.tabID}
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: []*docs.Request{request}}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("delete %s: %w", kind, err)
	}
	return writeDocsSegmentMutationResult(ctx, docID, loaded.tabID, segmentID, kind, 1, true)
}

func writeDocsSegmentMutationResult(ctx context.Context, docID, tabID, segmentID, kind string, requests int, deleted bool) error {
	payload := map[string]any{
		"documentId": docID, "segmentId": segmentID, "segmentType": kind, "requests": requests,
	}
	if tabID != "" {
		payload["tabId"] = tabID
	}
	if deleted {
		payload["deleted"] = true
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("segmentId\t%s", segmentID)
	u.Out().Linef("segmentType\t%s", kind)
	u.Out().Linef("requests\t%d", requests)
	if tabID != "" {
		u.Out().Linef("tabId\t%s", tabID)
	}
	if deleted {
		u.Out().Linef("deleted\ttrue")
	}
	return nil
}
