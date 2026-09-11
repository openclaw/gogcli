package cmd

import (
	"context"
	"fmt"
	"math"
	"strings"

	"google.golang.org/api/slides/v1"
)

type SlidesParagraphStyleCmd struct {
	PresentationID  string   `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID        string   `arg:"" name:"objectId" help:"Shape or table object ID"`
	Range           string   `name:"range" help:"UTF-16 range as start:end (default: all text); styles every intersecting paragraph"`
	Row             *int64   `name:"row" help:"Zero-based table row; requires --col"`
	Col             *int64   `name:"col" help:"Zero-based table column; requires --row"`
	Align           string   `name:"align" help:"Paragraph alignment: START, CENTER, END, JUSTIFIED"`
	Direction       string   `name:"direction" help:"Text direction: LEFT_TO_RIGHT or RIGHT_TO_LEFT"`
	LineSpacing     *float64 `name:"line-spacing" help:"Line spacing percent (100 is normal)"`
	SpaceAbove      *float64 `name:"space-above" help:"Space above each paragraph in points"`
	SpaceBelow      *float64 `name:"space-below" help:"Space below each paragraph in points"`
	IndentStart     *float64 `name:"indent-start" help:"Paragraph start indentation in points"`
	IndentEnd       *float64 `name:"indent-end" help:"Paragraph end indentation in points"`
	IndentFirstLine *float64 `name:"indent-first-line" help:"First-line indentation in points"`
}

func (c *SlidesParagraphStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, err := slidesElementTarget(c.PresentationID, c.ObjectID)
	if err != nil {
		return err
	}
	if (c.Row == nil) != (c.Col == nil) {
		return usage("--row and --col must be provided together")
	}
	if c.Row != nil && (*c.Row < 0 || *c.Col < 0) {
		return usage("--row and --col must be >= 0")
	}
	textRange := &slides.Range{Type: "ALL"}
	if strings.TrimSpace(c.Range) != "" {
		textRange, err = parseSlidesTextRange(c.Range)
		if err != nil {
			return err
		}
	}
	style, fields, err := c.paragraphStyle()
	if err != nil {
		return err
	}
	request := &slides.Request{UpdateParagraphStyle: &slides.UpdateParagraphStyleRequest{
		ObjectId: objectID, TextRange: textRange, Style: style, Fields: strings.Join(fields, ","),
	}}
	payload := map[string]any{"object_id": objectID, "fields": fields}
	output := map[string]any{"presentationId": presentationID, "objectId": objectID, "fields": fields}
	message := fmt.Sprintf("Styled paragraphs in %s", objectID)
	if c.Row != nil {
		request.UpdateParagraphStyle.CellLocation = slidesTableCellLocation(*c.Row, *c.Col)
		payload["row"], payload["col"] = *c.Row, *c.Col
		output["row"], output["col"] = *c.Row, *c.Col
		return runSlidesTableMutation(ctx, flags, slidesTableMutation{
			Op: "slides.paragraph-style", Action: "style paragraphs", PresentationID: presentationID,
			TableObjectID: objectID, Request: request, Payload: payload, Output: output, Text: message,
			Validate: func(table *slides.Table) error { return validateSlidesTableAnchor(table, *c.Row, *c.Col) },
		})
	}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op: "slides.paragraph-style", Action: "style paragraphs", PresentationID: presentationID,
		Request: request, Payload: payload, Output: output, Text: message,
	})
}

func (c *SlidesParagraphStyleCmd) paragraphStyle() (*slides.ParagraphStyle, []string, error) {
	style := &slides.ParagraphStyle{}
	var fields []string
	if c.Align != "" {
		align, err := slidesElementEnum(c.Align, "", "START", "CENTER", "END", "JUSTIFIED")
		if err != nil {
			return nil, nil, usage("--align must be START, CENTER, END, or JUSTIFIED")
		}
		style.Alignment = align
		fields = append(fields, "alignment")
	}
	if c.Direction != "" {
		direction, err := slidesElementEnum(c.Direction, "", "LEFT_TO_RIGHT", "RIGHT_TO_LEFT")
		if err != nil {
			return nil, nil, usage("--direction must be LEFT_TO_RIGHT or RIGHT_TO_LEFT")
		}
		style.Direction = direction
		fields = append(fields, "direction")
	}
	if c.LineSpacing != nil {
		if math.IsNaN(*c.LineSpacing) || math.IsInf(*c.LineSpacing, 0) || *c.LineSpacing <= 0 {
			return nil, nil, usage("--line-spacing must be finite and > 0")
		}
		style.LineSpacing = *c.LineSpacing
		fields = append(fields, "lineSpacing")
	}
	for _, dimension := range []struct {
		value       *float64
		target      **slides.Dimension
		field       string
		nonnegative bool
	}{
		{c.SpaceAbove, &style.SpaceAbove, "spaceAbove", true},
		{c.SpaceBelow, &style.SpaceBelow, "spaceBelow", true},
		{c.IndentStart, &style.IndentStart, "indentStart", false},
		{c.IndentEnd, &style.IndentEnd, "indentEnd", false},
		{c.IndentFirstLine, &style.IndentFirstLine, "indentFirstLine", false},
	} {
		if dimension.value == nil {
			continue
		}
		value := *dimension.value
		if math.IsNaN(value) || math.IsInf(value, 0) || (dimension.nonnegative && value < 0) {
			return nil, nil, usagef("invalid %s value", dimension.field)
		}
		*dimension.target = &slides.Dimension{Magnitude: value, Unit: "PT", ForceSendFields: []string{"Magnitude"}}
		fields = append(fields, dimension.field)
	}
	if len(fields) == 0 {
		return nil, nil, usage("provide at least one paragraph style option")
	}
	return style, fields, nil
}
