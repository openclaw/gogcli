package cmd

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesElementCmd struct {
	CreateShape SlidesElementCreateShapeCmd `cmd:"" name:"create-shape" help:"Create a native shape on a slide"`
	CreateLine  SlidesElementCreateLineCmd  `cmd:"" name:"create-line" help:"Create a native line on a slide"`
	Transform   SlidesElementTransformCmd   `cmd:"" name:"transform" aliases:"move,resize,rotate" help:"Move, resize, rotate, or replace an element transform"`
	Style       SlidesElementStyleCmd       `cmd:"" name:"style" help:"Style a shape fill/outline or a line"`
	ZOrder      SlidesElementZOrderCmd      `cmd:"" name:"z-order" help:"Change element stacking order"`
	Group       SlidesElementGroupCmd       `cmd:"" name:"group" help:"Group two or more elements"`
	Ungroup     SlidesElementUngroupCmd     `cmd:"" name:"ungroup" help:"Ungroup one or more element groups"`
	AltText     SlidesElementAltTextCmd     `cmd:"" name:"alt-text" help:"Set or clear element accessibility text"`
	Delete      SlidesElementDeleteCmd      `cmd:"" name:"delete" aliases:"rm" help:"Delete one page element"`
}

const (
	slidesElementKindShape = "shape"
	slidesElementKindLine  = "line"
)

type SlidesElementCreateShapeCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID"`
	Type           string  `name:"type" default:"RECTANGLE" help:"Slides shape type (for example RECTANGLE, TEXT_BOX, ELLIPSE)"`
	X              float64 `name:"x" default:"0" help:"Left position"`
	Y              float64 `name:"y" default:"0" help:"Top position"`
	Width          float64 `name:"width" default:"100" help:"Shape width"`
	Height         float64 `name:"height" default:"100" help:"Shape height"`
	Unit           string  `name:"unit" default:"PT" enum:"PT,EMU" help:"Geometry unit"`
	ObjectID       string  `name:"object-id" help:"Optional stable object ID (5-50 allowed characters)"`
}

func (c *SlidesElementCreateShapeCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, slideID, err := slidesElementPageTarget(c.PresentationID, c.SlideID)
	if err != nil {
		return err
	}
	shapeType := normalizeSlidesEnum(c.Type)
	if shapeType == "" {
		shapeType = "RECTANGLE"
	}
	if shapeType == "TYPE_UNSPECIFIED" {
		return usage("--type must name a concrete Slides shape type")
	}
	if c.Width <= 0 || c.Height <= 0 {
		return usage("--width and --height must be > 0")
	}
	objectID, err := slidesElementObjectID(c.ObjectID, "gogShape")
	if err != nil {
		return err
	}
	unit, err := slidesElementEnum(c.Unit, "PT", "PT", "EMU")
	if err != nil {
		return err
	}
	request := &slides.Request{CreateShape: &slides.CreateShapeRequest{
		ObjectId:          objectID,
		ShapeType:         shapeType,
		ElementProperties: slidesElementProperties(slideID, c.X, c.Y, c.Width, c.Height, unit),
	}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.create-shape",
		Action:         "create shape",
		PresentationID: presentationID,
		Request:        request,
		Payload: map[string]any{
			"slide_object_id": slideID,
			"object_id":       objectID,
			"shape_type":      shapeType,
		},
		Output: map[string]any{
			"presentationId": presentationID,
			"slideObjectId":  slideID,
			"objectId":       objectID,
			"shapeType":      shapeType,
		},
		Text: fmt.Sprintf("Created shape %s", objectID),
	})
}

type SlidesElementCreateLineCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID"`
	Category       string  `name:"category" default:"STRAIGHT" enum:"STRAIGHT,BENT,CURVED" help:"Line category"`
	X              float64 `name:"x" default:"0" help:"Start X position"`
	Y              float64 `name:"y" default:"0" help:"Start Y position"`
	Width          float64 `name:"width" default:"100" help:"Horizontal extent"`
	Height         float64 `name:"height" default:"0" help:"Vertical extent"`
	Unit           string  `name:"unit" default:"PT" enum:"PT,EMU" help:"Geometry unit"`
	ObjectID       string  `name:"object-id" help:"Optional stable object ID (5-50 allowed characters)"`
}

func (c *SlidesElementCreateLineCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, slideID, err := slidesElementPageTarget(c.PresentationID, c.SlideID)
	if err != nil {
		return err
	}
	if c.Width < 0 || c.Height < 0 || (c.Width == 0 && c.Height == 0) {
		return usage("--width and --height must be >= 0, with at least one > 0")
	}
	objectID, err := slidesElementObjectID(c.ObjectID, "gogLine")
	if err != nil {
		return err
	}
	category, err := slidesElementEnum(c.Category, "STRAIGHT", "STRAIGHT", "BENT", "CURVED")
	if err != nil {
		return err
	}
	unit, err := slidesElementEnum(c.Unit, "PT", "PT", "EMU")
	if err != nil {
		return err
	}
	request := &slides.Request{CreateLine: &slides.CreateLineRequest{
		ObjectId:          objectID,
		Category:          category,
		ElementProperties: slidesElementProperties(slideID, c.X, c.Y, c.Width, c.Height, unit),
	}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.create-line",
		Action:         "create line",
		PresentationID: presentationID,
		Request:        request,
		Payload: map[string]any{
			"slide_object_id": slideID,
			"object_id":       objectID,
			"category":        category,
		},
		Output: map[string]any{
			"presentationId": presentationID,
			"slideObjectId":  slideID,
			"objectId":       objectID,
			"category":       category,
		},
		Text: fmt.Sprintf("Created line %s", objectID),
	})
}

type SlidesElementTransformCmd struct {
	PresentationID string   `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string   `arg:"" name:"objectId" help:"Page element object ID"`
	ScaleX         *float64 `name:"scale-x" help:"X scale; omitted axis defaults to 1"`
	ScaleY         *float64 `name:"scale-y" help:"Y scale; omitted axis defaults to 1"`
	ShearX         *float64 `name:"shear-x" help:"X shear"`
	ShearY         *float64 `name:"shear-y" help:"Y shear"`
	TranslateX     *float64 `name:"translate-x" help:"X translation"`
	TranslateY     *float64 `name:"translate-y" help:"Y translation"`
	Rotate         *float64 `name:"rotate" help:"Clockwise rotation in degrees around the element origin"`
	Unit           string   `name:"unit" default:"PT" enum:"PT,EMU" help:"Translation unit"`
	ApplyMode      string   `name:"apply-mode" default:"RELATIVE" enum:"RELATIVE,ABSOLUTE" help:"Compose with or replace the existing transform"`
}

func (c *SlidesElementTransformCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, err := slidesElementTarget(c.PresentationID, c.ObjectID)
	if err != nil {
		return err
	}
	if c.ScaleX == nil && c.ScaleY == nil && c.ShearX == nil && c.ShearY == nil && c.TranslateX == nil && c.TranslateY == nil && c.Rotate == nil {
		return usage("provide at least one transform option")
	}
	if c.Rotate != nil && (c.ScaleX != nil || c.ScaleY != nil || c.ShearX != nil || c.ShearY != nil) {
		return usage("--rotate is mutually exclusive with scale and shear options")
	}

	var scaleX, scaleY, shearX, shearY float64
	if c.Rotate != nil {
		radians := *c.Rotate * math.Pi / 180
		scaleX, scaleY = math.Cos(radians), math.Cos(radians)
		shearX, shearY = -math.Sin(radians), math.Sin(radians)
	} else {
		scaleX = slidesElementFloat(c.ScaleX, 1)
		scaleY = slidesElementFloat(c.ScaleY, 1)
		shearX = slidesElementFloat(c.ShearX, 0)
		shearY = slidesElementFloat(c.ShearY, 0)
	}
	unit, err := slidesElementEnum(c.Unit, "PT", "PT", "EMU")
	if err != nil {
		return err
	}
	applyMode, err := slidesElementEnum(c.ApplyMode, "RELATIVE", "RELATIVE", "ABSOLUTE")
	if err != nil {
		return err
	}
	transform := &slides.AffineTransform{
		ScaleX:          scaleX,
		ScaleY:          scaleY,
		ShearX:          shearX,
		ShearY:          shearY,
		TranslateX:      slidesElementFloat(c.TranslateX, 0),
		TranslateY:      slidesElementFloat(c.TranslateY, 0),
		Unit:            unit,
		ForceSendFields: []string{"ScaleX", "ScaleY", "ShearX", "ShearY", "TranslateX", "TranslateY"},
	}
	request := &slides.Request{UpdatePageElementTransform: &slides.UpdatePageElementTransformRequest{
		ObjectId:  objectID,
		ApplyMode: applyMode,
		Transform: transform,
	}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.transform",
		Action:         "transform element",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"object_id": objectID, "apply_mode": applyMode},
		Output:         map[string]any{"presentationId": presentationID, "objectId": objectID, "applyMode": applyMode, "transform": transform},
		Text:           fmt.Sprintf("Transformed element %s", objectID),
	})
}

type SlidesElementStyleCmd struct {
	PresentationID     string   `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID           string   `arg:"" name:"objectId" help:"Shape or line object ID"`
	Kind               string   `name:"kind" default:"shape" enum:"shape,line" help:"Element kind"`
	FillColor          string   `name:"fill-color" help:"Shape fill as #RGB or #RRGGBB"`
	FillTransparent    bool     `name:"fill-transparent" help:"Remove the shape fill"`
	OutlineColor       string   `name:"outline-color" help:"Shape outline or line color as #RGB or #RRGGBB"`
	OutlineTransparent bool     `name:"outline-transparent" help:"Remove the shape outline or make the line transparent"`
	OutlineWeight      *float64 `name:"outline-weight" help:"Shape outline or line weight in points"`
	OutlineDash        *string  `name:"outline-dash" enum:"SOLID,DOT,DASH,DASH_DOT,LONG_DASH,LONG_DASH_DOT" help:"Shape outline or line dash style"`
}

func (c *SlidesElementStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, err := slidesElementTarget(c.PresentationID, c.ObjectID)
	if err != nil {
		return err
	}
	if c.FillColor != "" && c.FillTransparent {
		return usage("--fill-color and --fill-transparent are mutually exclusive")
	}
	if c.OutlineColor != "" && c.OutlineTransparent {
		return usage("--outline-color and --outline-transparent are mutually exclusive")
	}
	if c.OutlineWeight != nil && *c.OutlineWeight <= 0 {
		return usage("--outline-weight must be > 0")
	}
	kind := strings.ToLower(strings.TrimSpace(c.Kind))
	if kind == "" {
		kind = slidesElementKindShape
	}
	if kind != slidesElementKindShape && kind != slidesElementKindLine {
		return usage("--kind must be shape or line")
	}
	if kind == slidesElementKindLine && (c.FillColor != "" || c.FillTransparent) {
		return usage("fill options apply only to shapes")
	}
	if kind == slidesElementKindShape && c.OutlineTransparent && (c.OutlineWeight != nil || c.OutlineDash != nil) {
		return usage("--outline-transparent cannot be combined with outline weight or dash")
	}
	if c.FillColor == "" && !c.FillTransparent && c.OutlineColor == "" && !c.OutlineTransparent && c.OutlineWeight == nil && c.OutlineDash == nil {
		return usage("provide at least one style option")
	}

	request, fields, err := slidesElementStyleRequest(c, objectID, kind)
	if err != nil {
		return err
	}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.style",
		Action:         "style element",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"object_id": objectID, "kind": kind, "fields": fields},
		Output:         map[string]any{"presentationId": presentationID, "objectId": objectID, "kind": kind, "fields": fields},
		Text:           fmt.Sprintf("Styled %s %s", kind, objectID),
	})
}

type SlidesElementZOrderCmd struct {
	PresentationID string   `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectIDs      []string `arg:"" name:"objectId" help:"One or more page element object IDs"`
	Operation      string   `name:"operation" required:"" enum:"BRING_TO_FRONT,BRING_FORWARD,SEND_BACKWARD,SEND_TO_BACK" help:"Stacking operation"`
}

func (c *SlidesElementZOrderCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectIDs, err := slidesElementTargets(c.PresentationID, c.ObjectIDs, 1)
	if err != nil {
		return err
	}
	operation := normalizeSlidesEnum(c.Operation)
	request := &slides.Request{UpdatePageElementsZOrder: &slides.UpdatePageElementsZOrderRequest{
		PageElementObjectIds: objectIDs,
		Operation:            operation,
	}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.z-order",
		Action:         "change element z-order",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"object_ids": objectIDs, "operation": operation},
		Output:         map[string]any{"presentationId": presentationID, "objectIds": objectIDs, "operation": operation},
		Text:           fmt.Sprintf("Changed z-order for %d element(s)", len(objectIDs)),
	})
}

type SlidesElementGroupCmd struct {
	PresentationID string   `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectIDs      []string `arg:"" name:"objectId" help:"Two or more page element object IDs"`
	GroupID        string   `name:"group-id" help:"Optional stable group object ID"`
}

func (c *SlidesElementGroupCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectIDs, err := slidesElementTargets(c.PresentationID, c.ObjectIDs, 2)
	if err != nil {
		return err
	}
	groupID, err := slidesElementObjectID(c.GroupID, "gogGroup")
	if err != nil {
		return err
	}
	request := &slides.Request{GroupObjects: &slides.GroupObjectsRequest{
		ChildrenObjectIds: objectIDs,
		GroupObjectId:     groupID,
	}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.group",
		Action:         "group elements",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"object_ids": objectIDs, "group_object_id": groupID},
		Output:         map[string]any{"presentationId": presentationID, "objectIds": objectIDs, "groupObjectId": groupID},
		Text:           fmt.Sprintf("Created group %s", groupID),
	})
}

type SlidesElementUngroupCmd struct {
	PresentationID string   `arg:"" name:"presentationId" help:"Presentation ID"`
	GroupIDs       []string `arg:"" name:"groupId" help:"One or more top-level group object IDs"`
}

func (c *SlidesElementUngroupCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, groupIDs, err := slidesElementTargets(c.PresentationID, c.GroupIDs, 1)
	if err != nil {
		return err
	}
	request := &slides.Request{UngroupObjects: &slides.UngroupObjectsRequest{ObjectIds: groupIDs}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.ungroup",
		Action:         "ungroup elements",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"group_object_ids": groupIDs},
		Output:         map[string]any{"presentationId": presentationID, "groupObjectIds": groupIDs, "ungrouped": true},
		Text:           fmt.Sprintf("Ungrouped %d group(s)", len(groupIDs)),
	})
}

type SlidesElementAltTextCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string  `arg:"" name:"objectId" help:"Page element object ID"`
	Title          *string `name:"title" help:"Accessibility title; pass an empty value to clear"`
	Description    *string `name:"description" help:"Accessibility description; pass an empty value to clear"`
}

func (c *SlidesElementAltTextCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, err := slidesElementTarget(c.PresentationID, c.ObjectID)
	if err != nil {
		return err
	}
	if c.Title == nil && c.Description == nil {
		return usage("provide --title and/or --description")
	}
	update := &slides.UpdatePageElementAltTextRequest{ObjectId: objectID}
	fields := make([]string, 0, 2)
	if c.Title != nil {
		update.Title = *c.Title
		update.ForceSendFields = append(update.ForceSendFields, "Title")
		fields = append(fields, "title")
	}
	if c.Description != nil {
		update.Description = *c.Description
		update.ForceSendFields = append(update.ForceSendFields, "Description")
		fields = append(fields, "description")
	}
	request := &slides.Request{UpdatePageElementAltText: update}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.alt-text",
		Action:         "update element alt text",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"object_id": objectID, "fields": fields},
		Output:         map[string]any{"presentationId": presentationID, "objectId": objectID, "fields": fields},
		Text:           fmt.Sprintf("Updated alt text for element %s", objectID),
	})
}

type SlidesElementDeleteCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string `arg:"" name:"objectId" help:"Page element object ID"`
}

func (c *SlidesElementDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID, objectID, err := slidesElementTarget(c.PresentationID, c.ObjectID)
	if err != nil {
		return err
	}
	request := &slides.Request{DeleteObject: &slides.DeleteObjectRequest{ObjectId: objectID}}
	return runSlidesElementMutation(ctx, flags, slidesElementMutation{
		Op:             "slides.element.delete",
		Action:         "delete element",
		PresentationID: presentationID,
		Request:        request,
		Payload:        map[string]any{"object_id": objectID},
		Output:         map[string]any{"presentationId": presentationID, "objectId": objectID, "deleted": true},
		Text:           fmt.Sprintf("Deleted element %s", objectID),
		Destructive:    fmt.Sprintf("delete element %s from presentation %s", objectID, presentationID),
	})
}

type slidesElementMutation struct {
	Op             string
	Action         string
	PresentationID string
	Request        *slides.Request
	Payload        map[string]any
	Output         map[string]any
	Text           string
	Destructive    string
}

func runSlidesElementMutation(ctx context.Context, flags *RootFlags, mutation slidesElementMutation) error {
	body := &slides.BatchUpdatePresentationRequest{Requests: []*slides.Request{mutation.Request}}
	payload := mutation.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["presentation_id"] = mutation.PresentationID
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
	if _, err := svc.Presentations.BatchUpdate(mutation.PresentationID, body).Context(ctx).Do(); err != nil {
		return fmt.Errorf("%s: %w", mutation.Action, err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), mutation.Output)
	}
	ui.FromContext(ctx).Out().Linef("%s", mutation.Text)
	return nil
}

func slidesElementProperties(slideID string, x, y, width, height float64, unit string) *slides.PageElementProperties {
	return &slides.PageElementProperties{
		PageObjectId: slideID,
		Size: &slides.Size{
			Width:  slidesElementDimension(width, unit),
			Height: slidesElementDimension(height, unit),
		},
		Transform: &slides.AffineTransform{
			ScaleX:          1,
			ScaleY:          1,
			TranslateX:      x,
			TranslateY:      y,
			Unit:            unit,
			ForceSendFields: []string{"ScaleX", "ScaleY", "ShearX", "ShearY", "TranslateX", "TranslateY"},
		},
	}
}

func slidesElementDimension(value float64, unit string) *slides.Dimension {
	return &slides.Dimension{Magnitude: value, Unit: unit, ForceSendFields: []string{"Magnitude"}}
}

func slidesElementStyleRequest(c *SlidesElementStyleCmd, objectID, kind string) (*slides.Request, []string, error) {
	if kind == slidesElementKindLine {
		properties := &slides.LineProperties{}
		fields := make([]string, 0, 3)
		if c.OutlineColor != "" || c.OutlineTransparent {
			fill, err := slidesElementSolidFill(c.OutlineColor, c.OutlineTransparent)
			if err != nil {
				return nil, nil, usage("--outline-color must be a #RRGGBB or #RGB hex color")
			}
			properties.LineFill = &slides.LineFill{SolidFill: fill}
			fields = append(fields, "lineFill.solidFill.color", "lineFill.solidFill.alpha")
		}
		if c.OutlineWeight != nil {
			properties.Weight = slidesElementDimension(*c.OutlineWeight, "PT")
			fields = append(fields, "weight")
		}
		if c.OutlineDash != nil {
			properties.DashStyle = normalizeSlidesEnum(*c.OutlineDash)
			fields = append(fields, "dashStyle")
		}
		return &slides.Request{UpdateLineProperties: &slides.UpdateLinePropertiesRequest{
			ObjectId:       objectID,
			LineProperties: properties,
			Fields:         strings.Join(fields, ","),
		}}, fields, nil
	}

	properties := &slides.ShapeProperties{}
	fields := make([]string, 0, 8)
	if c.FillColor != "" || c.FillTransparent {
		if c.FillTransparent {
			properties.ShapeBackgroundFill = &slides.ShapeBackgroundFill{PropertyState: "NOT_RENDERED"}
			fields = append(fields, "shapeBackgroundFill.propertyState")
		} else {
			fill, err := slidesElementSolidFill(c.FillColor, false)
			if err != nil {
				return nil, nil, usage("--fill-color must be a #RRGGBB or #RGB hex color")
			}
			properties.ShapeBackgroundFill = &slides.ShapeBackgroundFill{PropertyState: "RENDERED", SolidFill: fill}
			fields = append(fields, "shapeBackgroundFill.propertyState", "shapeBackgroundFill.solidFill.color", "shapeBackgroundFill.solidFill.alpha")
		}
	}
	if c.OutlineColor != "" || c.OutlineTransparent || c.OutlineWeight != nil || c.OutlineDash != nil {
		outline := &slides.Outline{}
		if c.OutlineTransparent {
			outline.PropertyState = "NOT_RENDERED"
			fields = append(fields, "outline.propertyState")
		} else {
			outline.PropertyState = "RENDERED"
			fields = append(fields, "outline.propertyState")
			if c.OutlineColor != "" {
				fill, err := slidesElementSolidFill(c.OutlineColor, false)
				if err != nil {
					return nil, nil, usage("--outline-color must be a #RRGGBB or #RGB hex color")
				}
				outline.OutlineFill = &slides.OutlineFill{SolidFill: fill}
				fields = append(fields, "outline.outlineFill.solidFill.color", "outline.outlineFill.solidFill.alpha")
			}
			if c.OutlineWeight != nil {
				outline.Weight = slidesElementDimension(*c.OutlineWeight, "PT")
				fields = append(fields, "outline.weight")
			}
			if c.OutlineDash != nil {
				outline.DashStyle = normalizeSlidesEnum(*c.OutlineDash)
				fields = append(fields, "outline.dashStyle")
			}
		}
		properties.Outline = outline
	}
	return &slides.Request{UpdateShapeProperties: &slides.UpdateShapePropertiesRequest{
		ObjectId:        objectID,
		ShapeProperties: properties,
		Fields:          strings.Join(fields, ","),
	}}, fields, nil
}

func slidesElementSolidFill(color string, transparent bool) (*slides.SolidFill, error) {
	alpha := 1.0
	if transparent {
		alpha = 0
		color = "#000000"
	}
	r, g, b, ok := parseHexColor(color)
	if !ok {
		return nil, fmt.Errorf("invalid color")
	}
	return &slides.SolidFill{
		Alpha:           alpha,
		Color:           &slides.OpaqueColor{RgbColor: &slides.RgbColor{Red: r, Green: g, Blue: b}},
		ForceSendFields: []string{"Alpha"},
	}, nil
}

var slidesElementObjectIDPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_:-]{4,49}$`)

func slidesElementObjectID(value, prefix string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return newSlidesStructuralObjectID(prefix), nil
	}
	if !slidesElementObjectIDPattern.MatchString(value) {
		return "", usage("object ID must be 5-50 characters and contain only letters, digits, _, -, or :")
	}
	return value, nil
}

func slidesElementPageTarget(presentationID, slideID string) (string, string, error) {
	presentationID = strings.TrimSpace(presentationID)
	if presentationID == "" {
		return "", "", usage("empty presentationId")
	}
	slideID = strings.TrimSpace(slideID)
	if slideID == "" {
		return "", "", usage("empty slideId")
	}
	return presentationID, slideID, nil
}

func slidesElementTarget(presentationID, objectID string) (string, string, error) {
	presentationID = strings.TrimSpace(presentationID)
	if presentationID == "" {
		return "", "", usage("empty presentationId")
	}
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return "", "", usage("empty objectId")
	}
	return presentationID, objectID, nil
}

func slidesElementTargets(presentationID string, objectIDs []string, minimum int) (string, []string, error) {
	presentationID = strings.TrimSpace(presentationID)
	if presentationID == "" {
		return "", nil, usage("empty presentationId")
	}
	clean := make([]string, 0, len(objectIDs))
	seen := make(map[string]bool, len(objectIDs))
	for _, objectID := range objectIDs {
		objectID = strings.TrimSpace(objectID)
		if objectID == "" {
			return "", nil, usage("empty objectId")
		}
		if seen[objectID] {
			return "", nil, usagef("duplicate objectId %q", objectID)
		}
		seen[objectID] = true
		clean = append(clean, objectID)
	}
	if len(clean) < minimum {
		return "", nil, usagef("at least %d objectId value(s) required", minimum)
	}
	return presentationID, clean, nil
}

func slidesElementFloat(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeSlidesEnum(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	return value
}

func slidesElementEnum(value, fallback string, allowed ...string) (string, error) {
	value = normalizeSlidesEnum(value)
	if value == "" {
		value = fallback
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", usagef("invalid value %q; expected one of %s", value, strings.Join(allowed, ", "))
}
