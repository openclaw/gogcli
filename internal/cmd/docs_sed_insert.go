package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func (c *DocsSedCmd) doPositionalInsert(ctx context.Context, docsSvc *docs.Service, u *ui.UI, id string, idx int64, replacement string) error {
	// Check for image syntax first
	imgSpec := parseImageSyntax(replacement)

	// Check for table creation (explicit |RxC| or pipe-table syntax)
	tableSpec := parseTableCreate(replacement)
	if tableSpec == nil {
		tableSpec = parseTableFromPipes(replacement)
	}

	// Parse markdown formatting
	plainText, formats := parseMarkdownReplacement(replacement)

	var requests []*docs.Request

	switch {
	case tableSpec != nil:
		// Insert a table at the position
		requests = append(requests, &docs.Request{
			InsertTable: &docs.InsertTableRequest{
				Location: &docs.Location{Index: idx},
				Rows:     int64(tableSpec.rows),
				Columns:  int64(tableSpec.cols),
			},
		})
	case imgSpec != nil:
		// Insert an image
		imgReq := &docs.InsertInlineImageRequest{
			Uri:        imgSpec.URL,
			Location:   &docs.Location{Index: idx},
			ObjectSize: buildImageSizeSpec(imgSpec),
		}
		requests = append(requests, &docs.Request{InsertInlineImage: imgReq})
	default:
		// Insert plain text
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: idx},
				Text:     plainText,
			},
		})

		// Apply text formatting if any
		if len(formats) > 0 {
			end := idx + int64(len(plainText))
			requests = append(requests, buildTextStyleRequests(formats, idx, end)...)
		}
	}

	// After inserting text, reset paragraph styles and apply heading/bullet if requested
	if tableSpec == nil && imgSpec == nil && plainText != "" {
		end := idx + int64(len(plainText))

		// Reset paragraph to NORMAL_TEXT (removes inherited heading styles)
		requests = append(requests, &docs.Request{
			UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
				Range: &docs.Range{StartIndex: idx, EndIndex: end},
				ParagraphStyle: &docs.ParagraphStyle{
					NamedStyleType: "NORMAL_TEXT",
				},
				Fields: "namedStyleType",
			},
		})
		// Remove inherited bullets
		requests = append(requests, &docs.Request{
			DeleteParagraphBullets: &docs.DeleteParagraphBulletsRequest{
				Range: &docs.Range{StartIndex: idx, EndIndex: end},
			},
		})

		// Apply heading/bullet formats from markdown
		requests = append(requests, buildParagraphStyleRequests(formats, idx, end)...)
	}

	err := retryOnQuota(ctx, func() error {
		_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("batch update (positional insert): %w", err)
	}

	// Fill pipe-table cells if content was provided
	if tableSpec != nil && len(tableSpec.cells) > 0 {
		if err := c.fillTableCells(ctx, docsSvc, id, idx, tableSpec); err != nil {
			return fmt.Errorf("fill table cells: %w", err)
		}
	}

	label := fmt.Sprintf("%d chars", len(plainText))
	if tableSpec != nil {
		label = fmt.Sprintf("%dx%d table", tableSpec.rows, tableSpec.cols)
		if len(tableSpec.cells) > 0 {
			label += " (filled)"
		}
	} else if imgSpec != nil {
		label = "image"
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"inserted", label})
}
