package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"
)

var slidesTableAll = strings.ToUpper("all")

type SlidesTableCellCmd struct {
	Style SlidesTableCellStyleCmd `cmd:"" name:"style" help:"Style one zero-based table cell"`
}

type SlidesTableBorderCmd struct {
	Style SlidesTableBorderStyleCmd `cmd:"" name:"style" help:"Style borders around or within a table cell range"`
}

type SlidesTableRowSizeCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string  `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row            int64   `name:"row" required:"" help:"Zero-based row"`
	Height         float64 `name:"height" required:"" help:"Minimum row height in points (>=0)"`
}

type SlidesTableColumnSizeCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string  `arg:"" name:"tableObjectId" help:"Table object ID"`
	Col            int64   `name:"col" required:"" help:"Zero-based column"`
	Width          float64 `name:"width" required:"" help:"Column width in points (>=32)"`
}

type SlidesTableCellStyleCmd struct {
	PresentationID  string  `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID   string  `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row             int64   `name:"row" required:"" help:"Zero-based row"`
	Col             int64   `name:"col" required:"" help:"Zero-based column"`
	FillColor       string  `name:"fill-color" help:"Cell fill as #RGB or #RRGGBB"`
	FillTransparent bool    `name:"fill-transparent" help:"Remove the cell fill"`
	ContentAlign    *string `name:"content-align" enum:"TOP,MIDDLE,BOTTOM" help:"Vertical content alignment"`
	Range           string  `name:"range" help:"Optional UTF-16 text range as start:end; defaults to all cell text"`
	Bold            bool    `name:"bold" help:"Set cell text bold"`
	NoBold          bool    `name:"no-bold" help:"Clear cell text bold"`
	Italic          bool    `name:"italic" help:"Set cell text italic"`
	NoItalic        bool    `name:"no-italic" help:"Clear cell text italic"`
	Underline       bool    `name:"underline" help:"Set cell text underline"`
	NoUnderline     bool    `name:"no-underline" help:"Clear cell text underline"`
	TextColor       string  `name:"text-color" help:"Cell text color as #RGB or #RRGGBB"`
	Size            float64 `name:"size" help:"Cell text size in points"`
	Font            string  `name:"font" help:"Cell text font family"`
}

type SlidesTableBorderStyleCmd struct {
	PresentationID string   `arg:"" name:"presentationId" help:"Presentation ID"`
	TableObjectID  string   `arg:"" name:"tableObjectId" help:"Table object ID"`
	Row            int64    `name:"row" required:"" help:"Zero-based starting row"`
	Col            int64    `name:"col" required:"" help:"Zero-based starting column"`
	RowSpan        int64    `name:"row-span" default:"1" help:"Number of rows in the range"`
	ColSpan        int64    `name:"col-span" default:"1" help:"Number of columns in the range"`
	Position       string   `name:"position" default:"ALL" enum:"ALL,BOTTOM,INNER,INNER_HORIZONTAL,INNER_VERTICAL,LEFT,OUTER,RIGHT,TOP" help:"Borders within the selected range to update"`
	BorderColor    string   `name:"border-color" help:"Border color as #RGB or #RRGGBB"`
	Transparent    bool     `name:"transparent" help:"Make selected borders transparent"`
	Weight         *float64 `name:"weight" help:"Border weight in points"`
	Dash           *string  `name:"dash" enum:"SOLID,DOT,DASH,DASH_DOT,LONG_DASH,LONG_DASH_DOT" help:"Border dash style"`
}

func (c *SlidesTableRowSizeCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if c.Row < 0 {
		return usage("--row must be >= 0")
	}
	if c.Height < 0 {
		return usage("--height must be >= 0")
	}
	request := &slides.Request{UpdateTableRowProperties: &slides.UpdateTableRowPropertiesRequest{
		ObjectId:   tableID,
		RowIndices: []int64{c.Row},
		TableRowProperties: &slides.TableRowProperties{
			MinRowHeight: slidesElementDimension(c.Height, "PT"),
		},
		Fields: "minRowHeight",
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.row.size",
		Action:         "size table row",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"row": c.Row, "height": c.Height},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "row": c.Row, "minHeight": c.Height, "unit": "PT"},
		Text:           fmt.Sprintf("Set row %d minimum height to %.2f PT in table %s", c.Row, c.Height, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, c.Row, 0)
		},
	})
}

func (c *SlidesTableColumnSizeCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if c.Col < 0 {
		return usage("--col must be >= 0")
	}
	if c.Width < 32 {
		return usage("--width must be >= 32 points")
	}
	request := &slides.Request{UpdateTableColumnProperties: &slides.UpdateTableColumnPropertiesRequest{
		ObjectId:      tableID,
		ColumnIndices: []int64{c.Col},
		TableColumnProperties: &slides.TableColumnProperties{
			ColumnWidth: slidesElementDimension(c.Width, "PT"),
		},
		Fields: "columnWidth",
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.column.size",
		Action:         "size table column",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"col": c.Col, "width": c.Width},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "col": c.Col, "width": c.Width, "unit": "PT"},
		Text:           fmt.Sprintf("Set column %d width to %.2f PT in table %s", c.Col, c.Width, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, 0, c.Col)
		},
	})
}

func (c *SlidesTableCellStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if c.Row < 0 {
		return usage("--row must be >= 0")
	}
	if c.Col < 0 {
		return usage("--col must be >= 0")
	}
	if c.FillColor != "" && c.FillTransparent {
		return usage("--fill-color and --fill-transparent are mutually exclusive")
	}

	requests := make([]*slides.Request, 0, 2)
	fields := make([]string, 0, 8)
	properties := &slides.TableCellProperties{}
	cellFields := make([]string, 0, 4)
	if c.FillColor != "" || c.FillTransparent {
		if c.FillTransparent {
			properties.TableCellBackgroundFill = &slides.TableCellBackgroundFill{PropertyState: "NOT_RENDERED"}
			cellFields = append(cellFields, "tableCellBackgroundFill.propertyState")
		} else {
			fill, fillErr := slidesElementSolidFill(c.FillColor, false)
			if fillErr != nil {
				return usage("--fill-color must be a #RRGGBB or #RGB hex color")
			}
			properties.TableCellBackgroundFill = &slides.TableCellBackgroundFill{PropertyState: "RENDERED", SolidFill: fill}
			cellFields = append(cellFields, "tableCellBackgroundFill.propertyState", "tableCellBackgroundFill.solidFill.color", "tableCellBackgroundFill.solidFill.alpha")
		}
	}
	if c.ContentAlign != nil {
		alignment := normalizeSlidesEnum(*c.ContentAlign)
		if !slidesTableContentAlignments[alignment] {
			return usage("--content-align must be TOP, MIDDLE, or BOTTOM")
		}
		properties.ContentAlignment = alignment
		cellFields = append(cellFields, "contentAlignment")
	}
	if len(cellFields) > 0 {
		requests = append(requests, &slides.Request{UpdateTableCellProperties: &slides.UpdateTableCellPropertiesRequest{
			ObjectId:            tableID,
			TableRange:          slidesTableRange(c.Row, c.Col, 1, 1),
			TableCellProperties: properties,
			Fields:              strings.Join(cellFields, ","),
		}})
		fields = append(fields, cellFields...)
	}

	textOptions := slidesStyleTextOptions{
		Bold:        c.Bold,
		NoBold:      c.NoBold,
		Italic:      c.Italic,
		NoItalic:    c.NoItalic,
		Underline:   c.Underline,
		NoUnderline: c.NoUnderline,
		Color:       c.TextColor,
		Size:        c.Size,
		Font:        c.Font,
	}
	if slidesTextStyleOptionsProvided(textOptions) {
		textRange := &slides.Range{Type: slidesTableAll}
		if strings.TrimSpace(c.Range) != "" {
			textRange, err = parseSlidesTextRange(c.Range)
			if err != nil {
				return err
			}
		}
		textRequest, textFields, textErr := buildSlidesStyleTextRequest(tableID, textRange, textOptions)
		if textErr != nil {
			return textErr
		}
		textRequest.CellLocation = slidesTableCellLocation(c.Row, c.Col)
		requests = append(requests, &slides.Request{UpdateTextStyle: textRequest})
		for _, field := range strings.Split(textFields, ",") {
			fields = append(fields, "text."+field)
		}
	}
	if len(requests) == 0 {
		return usage("provide at least one cell or text style option")
	}

	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.cell.style",
		Action:         "style table cell",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Requests:       requests,
		Payload:        map[string]any{"row": c.Row, "col": c.Col, "fields": fields},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "row": c.Row, "col": c.Col, "fields": fields},
		Text:           fmt.Sprintf("Styled cell [%d,%d] in table %s", c.Row, c.Col, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableAnchor(table, c.Row, c.Col)
		},
	})
}

func (c *SlidesTableBorderStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, tableID, err := slidesTableTarget(c.PresentationID, c.TableObjectID)
	if err != nil {
		return err
	}
	if c.Row < 0 {
		return usage("--row must be >= 0")
	}
	if c.Col < 0 {
		return usage("--col must be >= 0")
	}
	if c.RowSpan < 1 {
		return usage("--row-span must be >= 1")
	}
	if c.ColSpan < 1 {
		return usage("--col-span must be >= 1")
	}
	if c.BorderColor != "" && c.Transparent {
		return usage("--border-color and --transparent are mutually exclusive")
	}
	if c.Weight != nil && *c.Weight <= 0 {
		return usage("--weight must be > 0")
	}
	position := normalizeSlidesEnum(c.Position)
	if !slidesTableBorderPositions[position] {
		return usage("invalid --position")
	}

	properties := &slides.TableBorderProperties{}
	fields := make([]string, 0, 4)
	if c.BorderColor != "" || c.Transparent {
		fill, fillErr := slidesElementSolidFill(c.BorderColor, c.Transparent)
		if fillErr != nil {
			return usage("--border-color must be a #RRGGBB or #RGB hex color")
		}
		properties.TableBorderFill = &slides.TableBorderFill{SolidFill: fill}
		fields = append(fields, "tableBorderFill.solidFill.color", "tableBorderFill.solidFill.alpha")
	}
	if c.Weight != nil {
		properties.Weight = slidesElementDimension(*c.Weight, "PT")
		fields = append(fields, "weight")
	}
	if c.Dash != nil {
		dash := normalizeSlidesEnum(*c.Dash)
		if !slidesTableBorderDashes[dash] {
			return usage("invalid --dash")
		}
		properties.DashStyle = dash
		fields = append(fields, "dashStyle")
	}
	if len(fields) == 0 {
		return usage("provide at least one border style option")
	}

	request := &slides.Request{UpdateTableBorderProperties: &slides.UpdateTableBorderPropertiesRequest{
		ObjectId:              tableID,
		TableRange:            slidesTableRange(c.Row, c.Col, c.RowSpan, c.ColSpan),
		BorderPosition:        position,
		TableBorderProperties: properties,
		Fields:                strings.Join(fields, ","),
	}}
	return runSlidesTableMutation(ctx, flags, slidesTableMutation{
		Op:             "slides.table.border.style",
		Action:         "style table borders",
		PresentationID: presentationID,
		TableObjectID:  tableID,
		Request:        request,
		Payload:        map[string]any{"row": c.Row, "col": c.Col, "row_span": c.RowSpan, "col_span": c.ColSpan, "position": position, "fields": fields},
		Output:         map[string]any{"presentationId": presentationID, "tableObjectId": tableID, "row": c.Row, "col": c.Col, "rowSpan": c.RowSpan, "colSpan": c.ColSpan, "position": position, "fields": fields},
		Text:           fmt.Sprintf("Styled %s borders in table %s", position, tableID),
		Validate: func(table *slides.Table) error {
			return validateSlidesTableRange(table, c.Row, c.Col, c.RowSpan, c.ColSpan)
		},
	})
}

func slidesTableRange(row, col, rowSpan, colSpan int64) *slides.TableRange {
	return &slides.TableRange{
		Location:   slidesTableCellLocation(row, col),
		RowSpan:    rowSpan,
		ColumnSpan: colSpan,
	}
}

func slidesTextStyleOptionsProvided(options slidesStyleTextOptions) bool {
	return options.Bold || options.NoBold || options.Italic || options.NoItalic || options.Underline || options.NoUnderline ||
		options.Color != "" || options.Size != 0 || strings.TrimSpace(options.Font) != ""
}

var slidesTableContentAlignments = map[string]bool{
	"TOP":    true,
	"MIDDLE": true,
	"BOTTOM": true,
}

var slidesTableBorderPositions = map[string]bool{
	slidesTableAll:     true,
	"BOTTOM":           true,
	"INNER":            true,
	"INNER_HORIZONTAL": true,
	"INNER_VERTICAL":   true,
	"LEFT":             true,
	"OUTER":            true,
	"RIGHT":            true,
	"TOP":              true,
}

var slidesTableBorderDashes = map[string]bool{
	"SOLID":         true,
	"DOT":           true,
	"DASH":          true,
	"DASH_DOT":      true,
	"LONG_DASH":     true,
	"LONG_DASH_DOT": true,
}
