package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsAddTabCmd struct {
	DocID     string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	Title     string `name:"title" help:"User-visible tab title"`
	Index     *int64 `name:"index" help:"Zero-based tab index within the parent"`
	ParentTab string `name:"parent-tab" help:"Optional parent tab title or ID"`
	IconEmoji string `name:"icon-emoji" help:"Optional tab emoji icon"`
}

func (c *DocsAddTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	props := &docs.TabProperties{}
	if title := strings.TrimSpace(c.Title); title != "" {
		props.Title = title
	}
	if c.Index != nil {
		props.Index = *c.Index
		props.ForceSendFields = append(props.ForceSendFields, "Index")
	}
	if emoji := strings.TrimSpace(c.IconEmoji); emoji != "" {
		props.IconEmoji = emoji
	}
	if parent := strings.TrimSpace(c.ParentTab); parent != "" {
		parentID, parentErr := docsResolveTabID(ctx, svc, docID, parent)
		if parentErr != nil {
			return parentErr
		}
		props.ParentTabId = parentID
	}

	if dryRunErr := dryRunExit(ctx, flags, "docs.add-tab", map[string]any{
		"doc_id":     docID,
		"title":      props.Title,
		"index":      c.Index,
		"parent_tab": props.ParentTabId,
		"icon_emoji": props.IconEmoji,
	}); dryRunErr != nil {
		return dryRunErr
	}

	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			AddDocumentTab: &docs.AddDocumentTabRequest{TabProperties: props},
		}},
	}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if resp == nil {
		return errors.New("add tab failed")
	}

	var created *docs.TabProperties
	if len(resp.Replies) > 0 && resp.Replies[0] != nil && resp.Replies[0].AddDocumentTab != nil {
		created = resp.Replies[0].AddDocumentTab.TabProperties
	}
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"documentId": docID}
		if created != nil {
			payload["tab"] = tabPropertiesJSON(created)
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("docId\t%s", docID)
	if created != nil {
		u.Out().Printf("tabId\t%s", created.TabId)
		u.Out().Printf("title\t%s", created.Title)
		u.Out().Printf("index\t%d", created.Index)
		if created.ParentTabId != "" {
			u.Out().Printf("parentTabId\t%s", created.ParentTabId)
		}
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsRenameTabCmd struct {
	DocID string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	Tab   string `name:"tab" help:"Existing tab title or ID"`
	Title string `name:"title" help:"New user-visible tab title"`
}

func (c *DocsRenameTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	tabQuery := strings.TrimSpace(c.Tab)
	newTitle := strings.TrimSpace(c.Title)
	if docID == "" {
		return usage("empty docId")
	}
	if tabQuery == "" {
		return usage("empty --tab")
	}
	if newTitle == "" {
		return usage("empty --title")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	resolved, err := docsResolveTab(ctx, svc, docID, tabQuery)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "docs.rename-tab", map[string]any{
		"doc_id": docID,
		"tab_id": resolved.TabProperties.TabId,
		"title":  newTitle,
	}); dryRunErr != nil {
		return dryRunErr
	}

	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			UpdateDocumentTabProperties: &docs.UpdateDocumentTabPropertiesRequest{
				Fields: "title",
				TabProperties: &docs.TabProperties{
					TabId: resolved.TabProperties.TabId,
					Title: newTitle,
				},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"tab":        map[string]any{"id": resolved.TabProperties.TabId, "title": newTitle},
		}
		if resp != nil && resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("docId\t%s", docID)
	u.Out().Printf("tabId\t%s", resolved.TabProperties.TabId)
	u.Out().Printf("title\t%s", newTitle)
	if resp != nil && resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsDeleteTabCmd struct {
	DocID string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	Tab   string `name:"tab" help:"Existing tab title or ID"`
}

func (c *DocsDeleteTabCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	tabQuery := strings.TrimSpace(c.Tab)
	if docID == "" {
		return usage("empty docId")
	}
	if tabQuery == "" {
		return usage("empty --tab")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	resolved, err := docsResolveTab(ctx, svc, docID, tabQuery)
	if err != nil {
		return err
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete tab %s from doc %s", resolved.TabProperties.TabId, docID)); confirmErr != nil {
		return confirmErr
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.delete-tab", map[string]any{
		"doc_id": docID,
		"tab_id": resolved.TabProperties.TabId,
	}); dryRunErr != nil {
		return dryRunErr
	}

	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			DeleteTab: &docs.DeleteTabRequest{TabId: resolved.TabProperties.TabId},
		}},
	}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"deleted":    true,
			"tab":        map[string]any{"id": resolved.TabProperties.TabId, "title": resolved.TabProperties.Title},
		}
		if resp != nil && resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("docId\t%s", docID)
	u.Out().Printf("tabId\t%s", resolved.TabProperties.TabId)
	u.Out().Printf("deleted\ttrue")
	if resolved.TabProperties.Title != "" {
		u.Out().Printf("title\t%s", resolved.TabProperties.Title)
	}
	if resp != nil && resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func docsResolveTab(ctx context.Context, svc *docs.Service, docID, query string) (*docs.Tab, error) {
	doc, err := svc.Documents.Get(docID).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("doc not found")
	}
	return findTab(flattenTabs(doc.Tabs), query)
}

func docsResolveTabID(ctx context.Context, svc *docs.Service, docID, query string) (string, error) {
	tab, err := docsResolveTab(ctx, svc, docID, query)
	if err != nil {
		return "", err
	}
	if tab == nil || tab.TabProperties == nil || strings.TrimSpace(tab.TabProperties.TabId) == "" {
		return "", fmt.Errorf("tab not found: %q", query)
	}
	return tab.TabProperties.TabId, nil
}

func tabPropertiesJSON(props *docs.TabProperties) map[string]any {
	if props == nil {
		return nil
	}
	payload := map[string]any{
		"id":    props.TabId,
		"title": props.Title,
		"index": props.Index,
	}
	if props.ParentTabId != "" {
		payload["parentTabId"] = props.ParentTabId
	}
	if props.IconEmoji != "" {
		payload["iconEmoji"] = props.IconEmoji
	}
	if props.NestingLevel != 0 {
		payload["nestingLevel"] = props.NestingLevel
	}
	return payload
}
