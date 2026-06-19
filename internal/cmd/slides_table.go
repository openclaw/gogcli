package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesTableCmd struct {
	Create  SlidesTableCreateCmd  `cmd:"" name:"create" aliases:"add" help:"Create an auto-sized native table on a slide"`
	Row     SlidesTableRowCmd     `cmd:"" name:"row" help:"Insert, delete, or size table rows"`
	Column  SlidesTableColumnCmd  `cmd:"" name:"column" aliases:"col" help:"Insert, delete, or size table columns"`
	Cell    SlidesTableCellCmd    `cmd:"" name:"cell" help:"Style table cells"`
	Border  SlidesTableBorderCmd  `cmd:"" name:"border" help:"Style table borders"`
	Merge   SlidesTableMergeCmd   `cmd:"" name:"merge" help:"Merge a rectangular table cell range"`
	Unmerge SlidesTableUnmergeCmd `cmd:"" name:"unmerge" aliases:"split" help:"Unmerge cells in a rectangular table range"`
}

type SlidesTableCreateCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID to place the table on"`
	ObjectID       string `name:"object-id" help:"Optional table object ID to assign"`
	Rows           int64  `name:"rows" required:"" help:"Number of table rows (>=1)"`
	Cols           int64  `name:"cols" required:"" help:"Number of table columns (>=1)"`
}

func (c *SlidesTableCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}
	objectID := strings.TrimSpace(c.ObjectID)
	if c.Rows < 1 {
		return usage("--rows must be >= 1")
	}
	if c.Cols < 1 {
		return usage("--cols must be >= 1")
	}
	req := buildSlidesCreateTableRequest(slideID, objectID, c.Rows, c.Cols)
	body := &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{{CreateTable: req}},
	}
	if err := dryRunExit(ctx, flags, "slides.table.create", map[string]any{
		"presentation_id": presentationID,
		"slide_id":        slideID,
		"object_id":       objectID,
		"rows":            c.Rows,
		"cols":            c.Cols,
		"batch_update":    body,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}

	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}
	if _, idx := findSlidesPageByID(pres, slideID); idx == -1 {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}
	if pres.RevisionId != "" {
		body.WriteControl = &slides.WriteControl{RequiredRevisionId: pres.RevisionId}
	}

	resp, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	tableID := objectID
	if resp != nil {
		for _, reply := range resp.Replies {
			if reply != nil && reply.CreateTable != nil && reply.CreateTable.ObjectId != "" {
				tableID = reply.CreateTable.ObjectId
				break
			}
		}
	}
	link := fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", presentationID)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"presentationId": presentationID,
			"slideObjectId":  slideID,
			"tableObjectId":  tableID,
			"rows":           c.Rows,
			"cols":           c.Cols,
			"link":           link,
		})
	}

	u.Out().Linef("table\t%s", tableID)
	u.Out().Linef("link\t%s", link)
	return nil
}

func buildSlidesCreateTableRequest(slideID, objectID string, rows, cols int64) *slides.CreateTableRequest {
	// Slides ignores size and transform during table creation and chooses both.
	// https://developers.google.com/workspace/slides/api/samples/tables#create_a_table
	return &slides.CreateTableRequest{
		ObjectId: objectID,
		Rows:     rows,
		Columns:  cols,
		ElementProperties: &slides.PageElementProperties{
			PageObjectId: slideID,
		},
	}
}
