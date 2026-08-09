package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/docsedit"
)

// docsCommentLocation is the per-comment attribution attached by --locate.
// Matches always cover every tab so callers can spot a quote that resolves in
// more than one place.
type docsCommentLocation struct {
	Matches  []docsedit.TextRange `json:"matches"`
	Orphaned bool                 `json:"orphaned"`
}

// driveCommentWithLocation renders a Drive comment plus a "location" key.
// drive.Comment declares MarshalJSON on a value receiver, so the method is
// promoted onto this struct and would drop Location without the override
// below - the same pattern eventWithCalendar uses in calendar_list.go.
type driveCommentWithLocation struct {
	*drive.Comment
	Location  *docsCommentLocation
	tabLabels []string
}

func (c *driveCommentWithLocation) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	raw := map[string]any{}
	if c.Comment != nil {
		data, err := json.Marshal(c.Comment)
		if err != nil {
			return nil, err
		}
		if string(data) != "null" {
			if err := json.Unmarshal(data, &raw); err != nil {
				return nil, err
			}
		}
	}
	if c.Location != nil {
		raw["location"] = c.Location
	}
	return json.Marshal(raw)
}

// docsCommentLocator resolves any number of comments against a single fetched
// document, where `docs comments locate` costs one fetch per comment.
type docsCommentLocator struct {
	locator   DocsCommentsLocateCmd
	doc       *docs.Document
	tabTitles map[string]string
	targetTab *docs.Tab
}

// newDocsCommentLocator expects tabQuery to have gone through resolveTabArg.
func newDocsCommentLocator(ctx context.Context, svc *docs.Service, docID, tabQuery string) (*docsCommentLocator, error) {
	doc, err := fetchDocForCommentLocation(ctx, svc, docID)
	if err != nil {
		return nil, err
	}

	tabs := flattenTabs(doc.Tabs)
	titles := make(map[string]string, len(tabs))
	for _, tab := range tabs {
		if tab.TabProperties == nil {
			continue
		}
		titles[tab.TabProperties.TabId] = tab.TabProperties.Title
	}

	located := &docsCommentLocator{
		locator:   DocsCommentsLocateCmd{NormalizeWhitespace: true},
		doc:       doc,
		tabTitles: titles,
	}
	if strings.TrimSpace(tabQuery) == "" {
		return located, nil
	}

	tab, err := findTab(tabs, tabQuery)
	if err != nil {
		return nil, err
	}
	if tab.TabProperties == nil || strings.TrimSpace(tab.TabProperties.TabId) == "" {
		return nil, fmt.Errorf("tab has no ID: %s", tabQuery)
	}
	located.targetTab = tab
	return located, nil
}

func fetchDocForCommentLocation(ctx context.Context, svc *docs.Service, docID string) (*docs.Document, error) {
	doc, err := svc.Documents.Get(docID).Context(ctx).IncludeTabsContent(true).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, err
	}
	return requireRawResponse(doc, "doc not found")
}

// attach drops comments without a match in the target tab, which includes
// orphans and comments with no quoted text at all.
func (l *docsCommentLocator) attach(comments []*drive.Comment) []*driveCommentWithLocation {
	located := make([]*driveCommentWithLocation, 0, len(comments))
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		location := l.resolve(comment)
		if l.targetTab != nil && !locationTouchesTab(location, l.targetTab.TabProperties.TabId) {
			continue
		}
		located = append(located, &driveCommentWithLocation{
			Comment:   comment,
			Location:  &location,
			tabLabels: l.tabLabels(location),
		})
	}
	return located
}

func (l *docsCommentLocator) resolve(comment *drive.Comment) docsCommentLocation {
	quote := docsCommentQuote(comment)
	matches := []docsedit.TextRange{}
	if strings.TrimSpace(quote) != "" {
		matches = append(matches, l.locator.findQuoteMatchesAcrossDocument(l.doc, quote)...)
	}
	return docsCommentLocation{Matches: matches, Orphaned: len(matches) == 0}
}

func (l *docsCommentLocator) tabLabels(location docsCommentLocation) []string {
	var labels []string
	seen := map[string]bool{}
	for _, match := range location.Matches {
		label := strings.TrimSpace(l.tabTitles[match.TabID])
		if label == "" {
			label = match.TabID
		}
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	return labels
}

func locationTouchesTab(location docsCommentLocation, tabID string) bool {
	for _, match := range location.Matches {
		if match.TabID == tabID {
			return true
		}
	}
	return false
}

// docsCommentListTab is the resolved --tab reported in JSON output.
type docsCommentListTab struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

func docsCommentTabLabel(tab *docs.Tab) string {
	if tab == nil || tab.TabProperties == nil {
		return ""
	}
	if title := strings.TrimSpace(tab.TabProperties.Title); title != "" {
		return title
	}
	return tab.TabProperties.TabId
}

func newDocsCommentListTab(tab *docs.Tab) *docsCommentListTab {
	if tab == nil || tab.TabProperties == nil {
		return nil
	}
	return &docsCommentListTab{ID: tab.TabProperties.TabId, Title: tab.TabProperties.Title}
}
