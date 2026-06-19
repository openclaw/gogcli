package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesTableRowCmd struct {
	Insert SlidesTableRowInsertCmd `cmd:"" name:"insert" aliases:"add" help:"Insert rows above or below a zero-based row"`
	Delete SlidesTableRowDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete the row containing a zero-based table cell"`
	Size   SlidesTableRowSizeCmd   `cmd:"" name:"size" help:"Set a row's minimum height"`
}

type SlidesTableColumnCmd struct {
	Insert SlidesTableColumnInsertCmd `cmd:"" name:"insert" aliases:"add" help:"Insert columns left or right of a zero-based column"`
	Delete SlidesTableColumnDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete the column containing a zero-based table cell"`
	Size   SlidesTableColumnSizeCmd   `cmd:"" name:"size" help:"Set a column's width"`
}

type SlidesTableRowInsertCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row            int64  `name:"row" required:"" help:"Zero-based reference row"`
	Count          int64  `name:"count" default:"1" help:"Number of rows to insert (1-20)"`
	Below          bool   `name:"below" help:"Insert below the reference row instead of above"`
}

type SlidesTableRowDeleteCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row            int64  `name:"row" required:"" help:"Zero-based row to delete"`
}

type SlidesTableColumnInsertCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string `arg:"" name:"tableObjectId" help:"Table object ID"`
	Col            int64  `name:"col" required:"" help:"Zero-based reference column"`
	Count          int64  `name:"count" default:"1" help:"Number of columns to insert (1-20)"`
	Right          bool   `name:"right" help:"Insert right of the reference column instead of left"`
}

type SlidesTableColumnDeleteCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string `arg:"" name:"tableObjectId" help:"Table object ID"`
	Col            int64  `name:"col" required:"" help:"Zero-based column to delete"`
}

type SlidesTableMergeCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row            int64  `name:"row" required:"" help:"Zero-based starting row"`
	Col            int64  `name:"col" required:"" help:"Zero-based starting column"`
	RowSpan        int64  `name:"row-span" default:"1" help:"Number of rows in the range"`
	ColSpan        int64  `name:"col-span" default:"1" help:"Number of columns in the range"`
}

type SlidesTableUnmergeCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row            int64  `name:"row" required:"" help:"Zero-based starting row"`
	Col            int64  `name:"col" required:"" help:"Zero-based starting column"`
	RowSpan        int64  `name:"row-span" default:"1" help:"Number of rows in the range"`
	ColSpan        int64  `name:"col-span" default:"1" help:"Number of columns in the range"`
}

func (c *SlidesTableRowInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if err := validateSlidesTableInsert(c.Row, c.Count, "row"); err != nil {
		return err
	}
	request := &slides.Request{InsertTableRows: &slides.InsertTableRowsRequest{
		TableObjectId:   tableID,
		CellLocation:    slidesTableCellLocation(c.Row, 0),
		InsertBelow:     c.Below,
		Number:          c.Count,
		ForceSendFields: []string{"InsertBelow"},
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.row.insert",
		Action:         "insert table rows",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"row": c.Row, "count": c.Count, "below": c.Below},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "row": c.Row, "count": c.Count, "below": c.Below},
		Text:           fmt.Sprintf("Inserted %d row(s) in table %s", c.Count, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, c.Row, 0)
		},
	})
}

func (c *SlidesTableRowDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if c.Row < 0 {
		return usage("--row must be >= 0")
	}
	request := &slides.Request{DeleteTableRow: &slides.DeleteTableRowRequest{
		TableObjectId: tableID,
		CellLocation:  slidesTableCellLocation(c.Row, 0),
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.row.delete",
		Action:         "delete table row",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"row": c.Row},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "row": c.Row, "deleted": true},
		Text:           fmt.Sprintf("Deleted row %d from table %s", c.Row, tableID),
		Destructive:    fmt.Sprintf("delete the row containing cell [%d,0] from table %s (a merged cell may span multiple rows)", c.Row, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, c.Row, 0)
		},
	})
}

func (c *SlidesTableColumnInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if err := validateSlidesTableInsert(c.Col, c.Count, "col"); err != nil {
		return err
	}
	request := &slides.Request{InsertTableColumns: &slides.InsertTableColumnsRequest{
		TableObjectId:   tableID,
		CellLocation:    slidesTableCellLocation(0, c.Col),
		InsertRight:     c.Right,
		Number:          c.Count,
		ForceSendFields: []string{"InsertRight"},
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.column.insert",
		Action:         "insert table columns",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"col": c.Col, "count": c.Count, "right": c.Right},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "col": c.Col, "count": c.Count, "right": c.Right},
		Text:           fmt.Sprintf("Inserted %d column(s) in table %s", c.Count, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, 0, c.Col)
		},
	})
}

func (c *SlidesTableColumnDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if c.Col < 0 {
		return usage("--col must be >= 0")
	}
	request := &slides.Request{DeleteTableColumn: &slides.DeleteTableColumnRequest{
		TableObjectId: tableID,
		CellLocation:  slidesTableCellLocation(0, c.Col),
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.column.delete",
		Action:         "delete table column",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"col": c.Col},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "col": c.Col, "deleted": true},
		Text:           fmt.Sprintf("Deleted column %d from table %s", c.Col, tableID),
		Destructive:    fmt.Sprintf("delete the column containing cell [0,%d] from table %s (a merged cell may span multiple columns)", c.Col, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, 0, c.Col)
		},
	})
}

func (c *SlidesTableMergeCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.RowSpan == 1 && c.ColSpan == 1 {
		return usage("merge range must span more than one cell")
	}
	return runSlidesTableRangeCommand(ctx, flags, slidesTableRangeCommand{
		presentationID: c.PresentationID,
		tableObjectID:  c.TableObjectID,
		row:            c.Row,
		col:            c.Col,
		rowSpan:        c.RowSpan,
		colSpan:        c.ColSpan,
		merge:          true,
	})
}

func (c *SlidesTableUnmergeCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSlidesTableRangeCommand(ctx, flags, slidesTableRangeCommand{
		presentationID: c.PresentationID,
		tableObjectID:  c.TableObjectID,
		row:            c.Row,
		col:            c.Col,
		rowSpan:        c.RowSpan,
		colSpan:        c.ColSpan,
	})
}

type slidesTableRangeCommand struct {
	presentationID string
	tableObjectID  string
	row            int64
	col            int64
	rowSpan        int64
	colSpan        int64
	merge          bool
}

func runSlidesTableRangeCommand(ctx context.Context, flags *RootFlags, command slidesTableRangeCommand) error {
	presentationID, tableID, err := slidesTableTarget(command.presentationID, command.tableObjectID)
	if err != nil {
		return err
	}
	if command.row < 0 {
		return usage("--row must be >= 0")
	}
	if command.col < 0 {
		return usage("--col must be >= 0")
	}
	if command.rowSpan < 1 {
		return usage("--row-span must be >= 1")
	}
	if command.colSpan < 1 {
		return usage("--col-span must be >= 1")
	}
	tableRange := &slides.TableRange{
		Location:   slidesTableCellLocation(command.row, command.col),
		RowSpan:    command.rowSpan,
		ColumnSpan: command.colSpan,
	}
	op := unmergeOp
	action := "unmerge table cells"
	request := &slides.Request{UnmergeTableCells: &slides.UnmergeTableCellsRequest{ObjectId: tableID, TableRange: tableRange}}
	if command.merge {
		op = mergeOp
		action = "merge table cells"
		request = &slides.Request{MergeTableCells: &slides.MergeTableCellsRequest{ObjectId: tableID, TableRange: tableRange}}
	}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table." + op,
		Action:         action,
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"row": command.row, "col": command.col, "row_span": command.rowSpan, "col_span": command.colSpan},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "row": command.row, "col": command.col, "rowSpan": command.rowSpan, "colSpan": command.colSpan, "operation": op},
		Text:           fmt.Sprintf("%s cells in table %s", strings.ToUpper(op[:1])+op[1:], tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableRange(table, command.row, command.col, command.rowSpan, command.colSpan)
		},
	})
}

type slidesTableMutation struct {
	Op             string
	Action         string
	PresentationID string
	TableObjectID  string
	Request        *slides.Request
	Requests       []*slides.Request
	Payload        map[string]any
	Output         map[string]any
	Text           string
	Destructive    string
	Validate       func(*slides.Table) error
}

func runSlidesTableMutation(ctx context.Context, flags *RootFlags, mutation slidesTableMutation) error {
	requests := mutation.Requests
	if len(requests) == 0 && mutation.Request != nil {
		requests = []*slides.Request{mutation.Request}
	}
	if len(requests) == 0 {
		return fmt.Errorf("%s: no Slides requests provided", mutation.Action)
	}
	body := &slides.BatchUpdatePresentationRequest{Requests: requests}
	payload := mutation.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["presentation_id"] = mutation.PresentationID
	payload["table_object_id"] = mutation.TableObjectID
	payload["batch_update"] = body
	var err error
	if mutation.Destructive != "" {
		err = dryRunAndConfirmDestructive(ctx, flags, mutation.Op, payload, mutation.Destructive)
	} else {
		err = dryRunExit(ctx, flags, mutation.Op, payload)
	}
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	pres, err := svc.Presentations.Get(mutation.PresentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}
	table := findSlidesTableByID(pres, mutation.TableObjectID)
	if table == nil {
		return fmt.Errorf("table %q not found in presentation", mutation.TableObjectID)
	}
	if mutation.Validate != nil {
		if err := mutation.Validate(table); err != nil {
			return err
		}
	}
	if pres.RevisionId != "" {
		body.WriteControl = &slides.WriteControl{RequiredRevisionId: pres.RevisionId}
	}
	if _, err := svc.Presentations.BatchUpdate(mutation.PresentationID, body).Context(ctx).Do(); err != nil {
		return fmt.Errorf("%s: %w", mutation.Action, err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), mutation.Output)
	}
	ui.FromContext(ctx).Out().Linef("%s", mutation.Text)
	return nil
}

func slidesTableTarget(presentationID, tableObjectID string) (string, string, error) {
	presentationID = strings.TrimSpace(presentationID)
	if presentationID == "" {
		return "", "", usage("empty presentationId")
	}
	tableObjectID = strings.TrimSpace(tableObjectID)
	if tableObjectID == "" {
		return "", "", usage("empty tableObjectId")
	}
	return presentationID, tableObjectID, nil
}

func validateSlidesTableInsert(index, count int64, dimension string) error {
	if index < 0 {
		return usagef("--%s must be >= 0", dimension)
	}
	if count < 1 || count > 20 {
		return usage("--count must be between 1 and 20")
	}
	return nil
}

func validateSlidesTableAnchor(table *slides.Table, row, col int64) error {
	if table.Rows < 1 || table.Columns < 1 {
		return fmt.Errorf("provider returned invalid table dimensions %dx%d", table.Rows, table.Columns)
	}
	if row < 0 || row >= table.Rows {
		return usagef("--row must be between 0 and %d", table.Rows-1)
	}
	if col < 0 || col >= table.Columns {
		return usagef("--col must be between 0 and %d", table.Columns-1)
	}
	return nil
}

func validateSlidesTableRange(table *slides.Table, row, col, rowSpan, colSpan int64) error {
	if err := validateSlidesTableAnchor(table, row, col); err != nil {
		return err
	}
	if rowSpan > table.Rows-row {
		return usagef("--row-span exceeds table row count %d from row %d", table.Rows, row)
	}
	if colSpan > table.Columns-col {
		return usagef("--col-span exceeds table column count %d from column %d", table.Columns, col)
	}
	return nil
}

func findSlidesTableByID(pres *slides.Presentation, objectID string) *slides.Table {
	if pres == nil {
		return nil
	}
	for _, page := range pres.Slides {
		if page == nil {
			continue
		}
		if table := findSlidesTableInElements(page.PageElements, objectID); table != nil {
			return table
		}
	}
	return nil
}

func findSlidesTableInElements(elements []*slides.PageElement, objectID string) *slides.Table {
	for _, element := range elements {
		if element == nil {
			continue
		}
		if element.ObjectId == objectID && element.Table != nil {
			return element.Table
		}
		if element.ElementGroup != nil {
			if table := findSlidesTableInElements(element.ElementGroup.Children, objectID); table != nil {
				return table
			}
		}
	}
	return nil
}
