package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/docsmarkdown"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsCellUpdateCmd struct {
	DocID       string `arg:"" name:"docId" help:"Doc ID"`
	TableIndex  int    `name:"table-index" help:"1-based table index in document order; negative indexes count from the end" default:"1"`
	Row         int    `name:"row" required:"" help:"1-based row number"`
	Col         int    `name:"col" required:"" help:"1-based column number"`
	Content     string `name:"content" help:"Replacement content (omit when using --content-file)"`
	ContentFile string `name:"content-file" help:"Read replacement content from a file"`
	Format      string `name:"format" help:"Content format: markdown|plain" default:"markdown" enum:"markdown,plain"`
	Append      bool   `name:"append" help:"Append inside the cell instead of replacing existing cell content"`
	Tab         string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID       string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsCellUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.TableIndex == 0 {
		return usage("--table-index cannot be 0")
	}
	if c.Row < 1 {
		return usage("--row must be >= 1")
	}
	if c.Col < 1 {
		return usage("--col must be >= 1")
	}
	content, err := c.resolveContent()
	if err != nil {
		return err
	}
	format := strings.ToLower(strings.TrimSpace(c.Format))
	if format == "" {
		format = docsContentFormatMarkdown
	}
	if format != docsContentFormatMarkdown && format != docsContentFormatPlain {
		return usage("--format must be markdown or plain")
	}
	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	if dryRunErr := dryRunExit(ctx, flags, "docs.cell-update", map[string]any{
		"document_id": docID,
		"table_index": c.TableIndex,
		"row":         c.Row,
		"col":         c.Col,
		"format":      format,
		"append":      c.Append,
		"tab":         c.Tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = loaded.tabID

	ref := &tableCellRef{tableIndex: c.TableIndex, row: c.Row, col: c.Col}
	cell, err := findTableCell(loaded.target, ref)
	if err != nil {
		return fmt.Errorf("find table cell: %w", err)
	}
	cellText, startIdx, endIdx := getCellText(cell)
	if startIdx <= 0 || endIdx <= startIdx {
		return fmt.Errorf("target cell has no editable text range")
	}
	writeEnd := endIdx
	if strings.HasSuffix(cellText, "\n") {
		writeEnd--
	}
	writeStart := startIdx
	prefixBoundary := false
	if c.Append {
		writeStart = writeEnd
		prefixBoundary = format == docsContentFormatMarkdown &&
			strings.TrimSpace(strings.TrimSuffix(cellText, "\n")) != "" &&
			markdownCellAppendNeedsBoundary(docsmarkdown.ParseMarkdown(content))
	}

	if err := updateDocsCellContent(ctx, svc, loaded.full, writeStart, writeEnd, content, format, c.Append, prefixBoundary, c.Tab); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"tableIndex": c.TableIndex,
			"row":        c.Row,
			"col":        c.Col,
			"format":     format,
			"append":     c.Append,
			"updated":    true,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("table_index\t%d", c.TableIndex)
	u.Out().Linef("row\t%d", c.Row)
	u.Out().Linef("col\t%d", c.Col)
	u.Out().Linef("updated\ttrue")
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsCellUpdateCmd) resolveContent() (string, error) {
	if c.ContentFile != "" && c.Content != "" {
		return "", usage("cannot use both --content and --content-file")
	}
	if c.ContentFile == "" {
		return c.Content, nil
	}
	data, err := os.ReadFile(c.ContentFile)
	if err != nil {
		return "", fmt.Errorf("read content file: %w", err)
	}
	return string(data), nil
}

func rewriteDocsCellUpdateContentArgs(model *kong.Application, args []string) []string {
	if model == nil || model.Node == nil {
		return args
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--content" ||
			!strings.HasPrefix(args[i+1], "- ") ||
			!isDocsCellUpdateCommand(commandNodeBefore(model.Node, args[:i])) {
			continue
		}
		out := append([]string(nil), args[:i]...)
		out = append(out, "--content="+args[i+1])
		out = append(out, args[i+2:]...)
		return out
	}
	return args
}

func isDocsCellUpdateCommand(node *kong.Node) bool {
	return node != nil &&
		node.Name == "cell-update" &&
		node.Parent != nil &&
		node.Parent.Name == "docs"
}

func updateDocsCellContent(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, content, format string, appendOnly bool, prefixBoundary bool, tabID string) error {
	var requests []*docs.Request
	if !appendOnly && startIdx < endIdx {
		requests = append(requests, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: startIdx, EndIndex: endIdx, TabId: tabID},
			},
		})
	}
	if content != "" {
		if format == docsContentFormatMarkdown {
			baseIndex := startIdx
			prefix := ""
			if prefixBoundary {
				baseIndex++
				prefix = "\n"
			}
			formatReqs, textToInsert, _, formatErr := buildMarkdownCellContent(content, baseIndex, tabID)
			if formatErr != nil {
				return formatErr
			}
			if textToInsert == "" {
				return usage("markdown content produced no editable cell text")
			}
			requests = append(requests, &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: startIdx, TabId: tabID},
					Text:     prefix + textToInsert,
				},
			})
			requests = append(requests, formatReqs...)
		} else {
			requests = append(requests, &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: startIdx, TabId: tabID},
					Text:     content,
				},
			})
		}
	}
	if len(requests) == 0 {
		return nil
	}
	_, err := svc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{RequiredRevisionId: doc.RevisionId},
		Requests:     requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("cell update: %w", err)
	}
	return nil
}

func buildMarkdownCellContent(content string, baseIndex int64, tabID string) ([]*docs.Request, string, int64, error) {
	elements := docsmarkdown.ParseMarkdown(content)
	formatReqs, text, tables := docsmarkdown.MarkdownToDocsRequests(elements, baseIndex, tabID)
	if len(tables) > 0 {
		return nil, "", 0, usage("markdown tables are not supported inside table cells")
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, "", 0, nil
	}
	formatReqs = clampDocsCellFormatRequests(formatReqs, baseIndex+utf16Len(text))

	// CreateParagraphBullets consumes one leading tab per nesting level.
	// Report final document growth so later native-table offsets stay exact.
	insertedLen := utf16Len(text)
	for _, element := range elements {
		if (element.Type == docsmarkdown.MDListItem || element.Type == docsmarkdown.MDNumberedList) && element.Level > 0 {
			insertedLen -= int64(element.Level)
		}
	}
	return formatReqs, text, insertedLen, nil
}

func markdownCellAppendNeedsBoundary(elements []docsmarkdown.MarkdownElement) bool {
	if len(elements) != 1 {
		return len(elements) > 1
	}
	switch elements[0].Type {
	case docsmarkdown.MDText, docsmarkdown.MDParagraph:
		return false
	default:
		return true
	}
}

func clampDocsCellFormatRequests(requests []*docs.Request, maxEnd int64) []*docs.Request {
	out := requests[:0]
	for _, req := range requests {
		r := docsRequestRange(req)
		if r == nil {
			out = append(out, req)
			continue
		}
		if r.EndIndex > maxEnd {
			r.EndIndex = maxEnd
		}
		if r.StartIndex >= r.EndIndex {
			continue
		}
		out = append(out, req)
	}
	return out
}

func docsRequestRange(req *docs.Request) *docs.Range {
	switch {
	case req.UpdateTextStyle != nil:
		return req.UpdateTextStyle.Range
	case req.UpdateParagraphStyle != nil:
		return req.UpdateParagraphStyle.Range
	case req.CreateParagraphBullets != nil:
		return req.CreateParagraphBullets.Range
	case req.DeleteParagraphBullets != nil:
		return req.DeleteParagraphBullets.Range
	default:
		return nil
	}
}
