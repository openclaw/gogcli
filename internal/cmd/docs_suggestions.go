package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

type DocsSuggestionsCmd struct {
	List DocsSuggestionsListCmd `cmd:"" name:"list" aliases:"ls" help:"List pending text insertions and deletions"`
}

type DocsSuggestionsListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for the first tab)"`
}

type docsSuggestionListItem struct {
	SuggestionID string `json:"suggestionId"`
	Kind         string `json:"kind"`
	Segment      string `json:"segment"`
	StartIndex   int64  `json:"startIndex"`
	EndIndex     int64  `json:"endIndex"`
	Text         string `json:"text"`
}

func (c *DocsSuggestionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if id == "" {
		return usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	tabQuery := strings.TrimSpace(c.Tab)
	call := svc.Documents.Get(id).
		SuggestionsViewMode("SUGGESTIONS_INLINE").
		Context(ctx)
	if tabQuery != "" {
		call = call.IncludeTabsContent(true)
	}
	doc, err := call.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return fmt.Errorf("doc not found")
	}

	tabID := ""
	if tabQuery != "" {
		tab, tabErr := findTab(flattenTabs(doc.Tabs), tabQuery)
		if tabErr != nil {
			return tabErr
		}
		projected, projectErr := projectRawDocumentTab(doc, tab)
		if projectErr != nil {
			return projectErr
		}
		doc = projected
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
		}
	}

	items := enumerateDocsSuggestions(doc)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"documentId":  doc.DocumentId,
			"tabId":       tabID,
			"suggestions": items,
		})
	}

	w, flush := tableWriter(ctx)
	defer flush()
	if !outfmt.IsPlain(ctx) {
		fmt.Fprintln(w, "SUGGESTION ID\tKIND\tSEGMENT\tSTART\tEND\tTEXT")
	}
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n",
			item.SuggestionID,
			item.Kind,
			item.Segment,
			item.StartIndex,
			item.EndIndex,
			docsTSVField(item.Text),
		)
	}
	return nil
}

func enumerateDocsSuggestions(doc *docs.Document) []docsSuggestionListItem {
	if doc == nil {
		return []docsSuggestionListItem{}
	}

	items := make([]docsSuggestionListItem, 0)
	collectDocsSuggestionSegment(&items, "body", bodyContent(doc.Body))
	for _, id := range sortedDocsMapKeys(doc.Headers) {
		header := doc.Headers[id]
		collectDocsSuggestionSegment(&items, "header:"+id, header.Content)
	}
	for _, id := range sortedDocsMapKeys(doc.Footers) {
		footer := doc.Footers[id]
		collectDocsSuggestionSegment(&items, "footer:"+id, footer.Content)
	}
	for _, id := range sortedDocsMapKeys(doc.Footnotes) {
		footnote := doc.Footnotes[id]
		collectDocsSuggestionSegment(&items, "footnote:"+id, footnote.Content)
	}
	return items
}

func bodyContent(body *docs.Body) []*docs.StructuralElement {
	if body == nil {
		return nil
	}
	return body.Content
}

func sortedDocsMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func collectDocsSuggestionSegment(
	items *[]docsSuggestionListItem,
	segment string,
	content []*docs.StructuralElement,
) {
	lastByKey := make(map[string]int)
	var walk func([]*docs.StructuralElement)
	walk = func(elements []*docs.StructuralElement) {
		for _, element := range elements {
			if element == nil {
				continue
			}
			if element.Paragraph != nil {
				for _, paragraphElement := range element.Paragraph.Elements {
					if paragraphElement == nil || paragraphElement.TextRun == nil {
						continue
					}
					appendDocsSuggestionRuns(items, lastByKey, segment, paragraphElement)
				}
			}
			if element.Table != nil {
				for _, row := range element.Table.TableRows {
					for _, cell := range row.TableCells {
						walk(cell.Content)
					}
				}
			}
			if element.TableOfContents != nil {
				walk(element.TableOfContents.Content)
			}
		}
	}
	walk(content)
}

func appendDocsSuggestionRuns(
	items *[]docsSuggestionListItem,
	lastByKey map[string]int,
	segment string,
	element *docs.ParagraphElement,
) {
	appendIDs := func(kind string, ids []string) {
		for _, id := range sortedUniqueStrings(ids) {
			key := kind + "\x00" + id
			if index, ok := lastByKey[key]; ok && (*items)[index].EndIndex == element.StartIndex {
				(*items)[index].EndIndex = element.EndIndex
				(*items)[index].Text += element.TextRun.Content
				continue
			}
			*items = append(*items, docsSuggestionListItem{
				SuggestionID: id,
				Kind:         kind,
				Segment:      segment,
				StartIndex:   element.StartIndex,
				EndIndex:     element.EndIndex,
				Text:         element.TextRun.Content,
			})
			lastByKey[key] = len(*items) - 1
		}
	}

	appendIDs("insertion", element.TextRun.SuggestedInsertionIds)
	appendIDs("deletion", element.TextRun.SuggestedDeletionIds)
}

func sortedUniqueStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
