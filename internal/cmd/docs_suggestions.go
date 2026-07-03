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
	collectDocsSuggestionSegment(&items, doc, "body", bodyContent(doc.Body))
	for _, id := range sortedDocsMapKeys(doc.Headers) {
		header := doc.Headers[id]
		collectDocsSuggestionSegment(&items, doc, "header:"+id, header.Content)
	}
	for _, id := range sortedDocsMapKeys(doc.Footers) {
		footer := doc.Footers[id]
		collectDocsSuggestionSegment(&items, doc, "footer:"+id, footer.Content)
	}
	for _, id := range sortedDocsMapKeys(doc.Footnotes) {
		footnote := doc.Footnotes[id]
		collectDocsSuggestionSegment(&items, doc, "footnote:"+id, footnote.Content)
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
	doc *docs.Document,
	segment string,
	content []*docs.StructuralElement,
) {
	lastByKey := make(map[string]int)
	var walk func([]*docs.StructuralElement, []string, []string)
	walk = func(elements []*docs.StructuralElement, insertionIDs, deletionIDs []string) {
		for _, element := range elements {
			if element == nil {
				continue
			}
			if element.SectionBreak != nil {
				appendDocsSuggestionRange(
					items,
					lastByKey,
					segment,
					element.StartIndex,
					element.EndIndex,
					"",
					mergeDocsSuggestionIDs(insertionIDs, element.SectionBreak.SuggestedInsertionIds),
					mergeDocsSuggestionIDs(deletionIDs, element.SectionBreak.SuggestedDeletionIds),
				)
			}
			if element.Paragraph != nil {
				appendDocsParagraphMapSuggestions(
					items,
					lastByKey,
					doc,
					segment,
					element.StartIndex,
					element.Paragraph,
				)
				for _, paragraphElement := range element.Paragraph.Elements {
					if paragraphElement == nil {
						continue
					}
					appendDocsSuggestionElement(
						items,
						lastByKey,
						segment,
						paragraphElement,
						doc.InlineObjects,
						insertionIDs,
						deletionIDs,
					)
				}
			}
			if element.Table != nil {
				tableInsertions := mergeDocsSuggestionIDs(insertionIDs, element.Table.SuggestedInsertionIds)
				tableDeletions := mergeDocsSuggestionIDs(deletionIDs, element.Table.SuggestedDeletionIds)
				for _, row := range element.Table.TableRows {
					if row == nil {
						continue
					}
					rowInsertions := mergeDocsSuggestionIDs(tableInsertions, row.SuggestedInsertionIds)
					rowDeletions := mergeDocsSuggestionIDs(tableDeletions, row.SuggestedDeletionIds)
					for _, cell := range row.TableCells {
						if cell == nil {
							continue
						}
						walk(
							cell.Content,
							mergeDocsSuggestionIDs(rowInsertions, cell.SuggestedInsertionIds),
							mergeDocsSuggestionIDs(rowDeletions, cell.SuggestedDeletionIds),
						)
					}
				}
			}
			if element.TableOfContents != nil {
				walk(
					element.TableOfContents.Content,
					mergeDocsSuggestionIDs(insertionIDs, element.TableOfContents.SuggestedInsertionIds),
					mergeDocsSuggestionIDs(deletionIDs, element.TableOfContents.SuggestedDeletionIds),
				)
			}
		}
	}
	walk(content, nil, nil)
}

func appendDocsParagraphMapSuggestions(
	items *[]docsSuggestionListItem,
	lastByKey map[string]int,
	doc *docs.Document,
	segment string,
	anchorIndex int64,
	paragraph *docs.Paragraph,
) {
	if paragraph.Bullet != nil {
		if list, ok := doc.Lists[paragraph.Bullet.ListId]; ok {
			appendDocsSuggestionRange(
				items,
				lastByKey,
				segment,
				anchorIndex,
				anchorIndex,
				"",
				[]string{list.SuggestedInsertionId},
				list.SuggestedDeletionIds,
			)
		}
	}

	objectIDs := append([]string(nil), paragraph.PositionedObjectIds...)
	for _, suggestionID := range sortedDocsMapKeys(paragraph.SuggestedPositionedObjectIds) {
		objectIDs = append(objectIDs, paragraph.SuggestedPositionedObjectIds[suggestionID].ObjectIds...)
	}
	for _, objectID := range sortedUniqueStrings(objectIDs) {
		object, ok := doc.PositionedObjects[objectID]
		if !ok {
			continue
		}
		appendDocsSuggestionRange(
			items,
			lastByKey,
			segment,
			anchorIndex,
			anchorIndex,
			"",
			[]string{object.SuggestedInsertionId},
			object.SuggestedDeletionIds,
		)
	}
}

func appendDocsSuggestionElement(
	items *[]docsSuggestionListItem,
	lastByKey map[string]int,
	segment string,
	element *docs.ParagraphElement,
	inlineObjects map[string]docs.InlineObject,
	inheritedInsertionIDs []string,
	inheritedDeletionIDs []string,
) {
	text, insertionIDs, deletionIDs, ok := docsParagraphElementSuggestions(element, inlineObjects)
	if !ok {
		return
	}
	appendDocsSuggestionRange(
		items,
		lastByKey,
		segment,
		element.StartIndex,
		element.EndIndex,
		text,
		mergeDocsSuggestionIDs(inheritedInsertionIDs, insertionIDs),
		mergeDocsSuggestionIDs(inheritedDeletionIDs, deletionIDs),
	)
}

func appendDocsSuggestionRange(
	items *[]docsSuggestionListItem,
	lastByKey map[string]int,
	segment string,
	startIndex int64,
	endIndex int64,
	text string,
	insertionIDs []string,
	deletionIDs []string,
) {
	appendIDs := func(kind string, ids []string) {
		for _, id := range sortedUniqueStrings(ids) {
			key := kind + "\x00" + id
			if index, ok := lastByKey[key]; ok && (*items)[index].EndIndex == startIndex {
				(*items)[index].EndIndex = endIndex
				(*items)[index].Text += text
				continue
			}
			*items = append(*items, docsSuggestionListItem{
				SuggestionID: id,
				Kind:         kind,
				Segment:      segment,
				StartIndex:   startIndex,
				EndIndex:     endIndex,
				Text:         text,
			})
			lastByKey[key] = len(*items) - 1
		}
	}

	appendIDs("insertion", insertionIDs)
	appendIDs("deletion", deletionIDs)
}

func docsParagraphElementSuggestions(
	element *docs.ParagraphElement,
	inlineObjects map[string]docs.InlineObject,
) (
	text string,
	insertionIDs []string,
	deletionIDs []string,
	ok bool,
) {
	switch {
	case element.TextRun != nil:
		return element.TextRun.Content,
			element.TextRun.SuggestedInsertionIds,
			element.TextRun.SuggestedDeletionIds,
			true
	case element.AutoText != nil:
		return "", element.AutoText.SuggestedInsertionIds, element.AutoText.SuggestedDeletionIds, true
	case element.ColumnBreak != nil:
		return "", element.ColumnBreak.SuggestedInsertionIds, element.ColumnBreak.SuggestedDeletionIds, true
	case element.DateElement != nil:
		return "", element.DateElement.SuggestedInsertionIds, element.DateElement.SuggestedDeletionIds, true
	case element.Equation != nil:
		return "", element.Equation.SuggestedInsertionIds, element.Equation.SuggestedDeletionIds, true
	case element.FootnoteReference != nil:
		return "", element.FootnoteReference.SuggestedInsertionIds, element.FootnoteReference.SuggestedDeletionIds, true
	case element.HorizontalRule != nil:
		return "", element.HorizontalRule.SuggestedInsertionIds, element.HorizontalRule.SuggestedDeletionIds, true
	case element.InlineObjectElement != nil:
		insertionIDs := element.InlineObjectElement.SuggestedInsertionIds
		deletionIDs := element.InlineObjectElement.SuggestedDeletionIds
		if object, ok := inlineObjects[element.InlineObjectElement.InlineObjectId]; ok {
			insertionIDs = mergeDocsSuggestionIDs(insertionIDs, []string{object.SuggestedInsertionId})
			deletionIDs = mergeDocsSuggestionIDs(deletionIDs, object.SuggestedDeletionIds)
		}
		return "", insertionIDs, deletionIDs, true
	case element.PageBreak != nil:
		return "", element.PageBreak.SuggestedInsertionIds, element.PageBreak.SuggestedDeletionIds, true
	case element.Person != nil:
		return "", element.Person.SuggestedInsertionIds, element.Person.SuggestedDeletionIds, true
	case element.RichLink != nil:
		return "", element.RichLink.SuggestedInsertionIds, element.RichLink.SuggestedDeletionIds, true
	default:
		return "", nil, nil, false
	}
}

func mergeDocsSuggestionIDs(groups ...[]string) []string {
	var values []string
	for _, group := range groups {
		values = append(values, group...)
	}
	return sortedUniqueStrings(values)
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
