package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

type slidesElementTestRunner interface {
	Run(context.Context, *RootFlags) error
}

func float64ElementTestPtr(value float64) *float64 { return &value }

func captureSlidesElementRequest(t *testing.T, runner slidesElementTestRunner, flags *RootFlags) *slides.Request {
	t.Helper()
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{"presentationId": "pres1", "replies": []any{map[string]any{}}})
	defer srv.Close()

	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), newSlidesServiceFromServer(t, srv))
	if err := runner.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured %d requests, want 1", len(captured))
	}
	return captured[0]
}

func TestSlidesElementCreateShape(t *testing.T) {
	request := captureSlidesElementRequest(t, &SlidesElementCreateShapeCmd{
		PresentationID: "pres1",
		SlideID:        "slide1",
		Type:           "round-rectangle",
		X:              12,
		Y:              24,
		Width:          200,
		Height:         80,
		Unit:           "PT",
		ObjectID:       "shape_123",
	}, &RootFlags{Account: "a@b.com"})

	create := request.CreateShape
	if create == nil {
		t.Fatalf("expected createShape, got %+v", request)
	}
	if create.ObjectId != "shape_123" || create.ShapeType != "ROUND_RECTANGLE" {
		t.Fatalf("unexpected createShape: %+v", create)
	}
	properties := create.ElementProperties
	if properties.PageObjectId != "slide1" || properties.Size.Width.Magnitude != 200 || properties.Size.Height.Magnitude != 80 {
		t.Fatalf("unexpected element properties: %+v", properties)
	}
	if properties.Transform.TranslateX != 12 || properties.Transform.TranslateY != 24 || properties.Transform.ScaleX != 1 || properties.Transform.ScaleY != 1 {
		t.Fatalf("unexpected transform: %+v", properties.Transform)
	}
}

func TestSlidesElementCreateLinePreservesZeroExtent(t *testing.T) {
	properties := slidesElementProperties("slide1", 0, 0, 120, 0, "PT")
	encoded, err := json.Marshal(properties)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"magnitude":0`)) || !bytes.Contains(encoded, []byte(`"translateX":0`)) {
		t.Fatalf("zero geometry omitted from request: %s", encoded)
	}

	request := captureSlidesElementRequest(t, &SlidesElementCreateLineCmd{
		PresentationID: "pres1",
		SlideID:        "slide1",
		Category:       "curved",
		Width:          120,
		Height:         0,
		Unit:           "PT",
		ObjectID:       "line_1234",
	}, &RootFlags{Account: "a@b.com"})
	if request.CreateLine == nil || request.CreateLine.Category != "CURVED" || request.CreateLine.ObjectId != "line_1234" {
		t.Fatalf("unexpected createLine: %+v", request.CreateLine)
	}
}

func TestSlidesElementTransformRotation(t *testing.T) {
	request := captureSlidesElementRequest(t, &SlidesElementTransformCmd{
		PresentationID: "pres1",
		ObjectID:       "shape1",
		Rotate:         float64ElementTestPtr(90),
		TranslateX:     float64ElementTestPtr(10),
		Unit:           "PT",
		ApplyMode:      "RELATIVE",
	}, &RootFlags{Account: "a@b.com"})

	update := request.UpdatePageElementTransform
	if update == nil || update.ObjectId != "shape1" || update.ApplyMode != "RELATIVE" {
		t.Fatalf("unexpected transform update: %+v", update)
	}
	if math.Abs(update.Transform.ScaleX) > 1e-9 || math.Abs(update.Transform.ScaleY) > 1e-9 || update.Transform.ShearX != -1 || update.Transform.ShearY != 1 {
		t.Fatalf("unexpected rotation matrix: %+v", update.Transform)
	}
	if update.Transform.TranslateX != 10 || update.Transform.TranslateY != 0 {
		t.Fatalf("unexpected translation: %+v", update.Transform)
	}
}

func TestSlidesElementStyleShape(t *testing.T) {
	dash := "DASH"
	request := captureSlidesElementRequest(t, &SlidesElementStyleCmd{
		PresentationID: "pres1",
		ObjectID:       "shape1",
		Kind:           "shape",
		FillColor:      "#123",
		OutlineColor:   "#abcdef",
		OutlineWeight:  float64ElementTestPtr(2.5),
		OutlineDash:    &dash,
	}, &RootFlags{Account: "a@b.com"})

	update := request.UpdateShapeProperties
	if update == nil || update.ObjectId != "shape1" {
		t.Fatalf("unexpected shape style request: %+v", update)
	}
	for _, field := range []string{"shapeBackgroundFill.solidFill.color", "outline.outlineFill.solidFill.color", "outline.weight", "outline.dashStyle"} {
		if !strings.Contains(update.Fields, field) {
			t.Errorf("field mask %q missing %q", update.Fields, field)
		}
	}
	fill := update.ShapeProperties.ShapeBackgroundFill.SolidFill
	if fill.Color.RgbColor.Red != 1.0/15 || fill.Color.RgbColor.Green != 2.0/15 || fill.Color.RgbColor.Blue != 3.0/15 {
		t.Fatalf("unexpected #123 expansion: %+v", fill.Color.RgbColor)
	}
	if update.ShapeProperties.Outline.Weight.Magnitude != 2.5 || update.ShapeProperties.Outline.DashStyle != "DASH" {
		t.Fatalf("unexpected outline: %+v", update.ShapeProperties.Outline)
	}
}

func TestSlidesElementStyleLineTransparent(t *testing.T) {
	request := captureSlidesElementRequest(t, &SlidesElementStyleCmd{
		PresentationID:     "pres1",
		ObjectID:           "line1",
		Kind:               "line",
		OutlineTransparent: true,
	}, &RootFlags{Account: "a@b.com"})

	update := request.UpdateLineProperties
	if update == nil || update.LineProperties.LineFill == nil {
		t.Fatalf("unexpected line style request: %+v", update)
	}
	if update.LineProperties.LineFill.SolidFill.Alpha != 0 || !strings.Contains(update.Fields, "lineFill.solidFill.alpha") {
		t.Fatalf("line transparency not encoded: %+v", update)
	}
}

func TestSlidesElementStructuralRequests(t *testing.T) {
	t.Run("z-order", func(t *testing.T) {
		request := captureSlidesElementRequest(t, &SlidesElementZOrderCmd{
			PresentationID: "pres1",
			ObjectIDs:      []string{"shape1", "line1"},
			Operation:      "BRING_TO_FRONT",
		}, &RootFlags{Account: "a@b.com"})
		if request.UpdatePageElementsZOrder == nil || len(request.UpdatePageElementsZOrder.PageElementObjectIds) != 2 {
			t.Fatalf("unexpected z-order request: %+v", request)
		}
	})

	t.Run("group", func(t *testing.T) {
		request := captureSlidesElementRequest(t, &SlidesElementGroupCmd{
			PresentationID: "pres1",
			ObjectIDs:      []string{"shape1", "line1"},
			GroupID:        "group_123",
		}, &RootFlags{Account: "a@b.com"})
		if request.GroupObjects == nil || request.GroupObjects.GroupObjectId != "group_123" || len(request.GroupObjects.ChildrenObjectIds) != 2 {
			t.Fatalf("unexpected group request: %+v", request)
		}
	})

	t.Run("ungroup", func(t *testing.T) {
		request := captureSlidesElementRequest(t, &SlidesElementUngroupCmd{
			PresentationID: "pres1",
			GroupIDs:       []string{"group_123"},
		}, &RootFlags{Account: "a@b.com"})
		if request.UngroupObjects == nil || len(request.UngroupObjects.ObjectIds) != 1 {
			t.Fatalf("unexpected ungroup request: %+v", request)
		}
	})

	t.Run("alt-text-clear", func(t *testing.T) {
		empty := ""
		request := captureSlidesElementRequest(t, &SlidesElementAltTextCmd{
			PresentationID: "pres1",
			ObjectID:       "shape1",
			Title:          &empty,
		}, &RootFlags{Account: "a@b.com"})
		if request.UpdatePageElementAltText == nil || request.UpdatePageElementAltText.Title != "" {
			t.Fatalf("unexpected alt text request: %+v", request)
		}
	})

	t.Run("delete", func(t *testing.T) {
		request := captureSlidesElementRequest(t, &SlidesElementDeleteCmd{
			PresentationID: "pres1",
			ObjectID:       "shape1",
		}, &RootFlags{Account: "a@b.com", Force: true})
		if request.DeleteObject == nil || request.DeleteObject.ObjectId != "shape1" {
			t.Fatalf("unexpected delete request: %+v", request)
		}
	})
}

func TestSlidesElementDryRunSkipsService(t *testing.T) {
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)
	cmd := &SlidesElementCreateShapeCmd{
		PresentationID: "pres1",
		SlideID:        "slide1",
		Width:          100,
		Height:         50,
		ObjectID:       "shape_123",
	}
	if err := cmd.Run(ctx, &RootFlags{DryRun: true}); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), `"op": "slides.element.create-shape"`) || !strings.Contains(out.String(), `"createShape"`) {
		t.Fatalf("unexpected dry-run output: %s", out.String())
	}
}

func TestSlidesElementValidation(t *testing.T) {
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)
	tests := []struct {
		name string
		cmd  slidesElementTestRunner
		want string
	}{
		{"shape size", &SlidesElementCreateShapeCmd{PresentationID: "p", SlideID: "s", Width: 0, Height: 10}, "width and --height"},
		{"line size", &SlidesElementCreateLineCmd{PresentationID: "p", SlideID: "s"}, "at least one > 0"},
		{"object ID", &SlidesElementCreateShapeCmd{PresentationID: "p", SlideID: "s", Width: 10, Height: 10, ObjectID: "bad!"}, "object ID"},
		{"empty transform", &SlidesElementTransformCmd{PresentationID: "p", ObjectID: "o"}, "at least one transform"},
		{"rotate conflict", &SlidesElementTransformCmd{PresentationID: "p", ObjectID: "o", Rotate: float64ElementTestPtr(1), ScaleX: float64ElementTestPtr(1)}, "mutually exclusive"},
		{"shape transparent outline conflict", &SlidesElementStyleCmd{PresentationID: "p", ObjectID: "o", Kind: "shape", OutlineTransparent: true, OutlineWeight: float64ElementTestPtr(1)}, "cannot be combined"},
		{"line fill", &SlidesElementStyleCmd{PresentationID: "p", ObjectID: "o", Kind: "line", FillColor: "#fff"}, "only to shapes"},
		{"group count", &SlidesElementGroupCmd{PresentationID: "p", ObjectIDs: []string{"one"}}, "at least 2"},
		{"duplicate IDs", &SlidesElementZOrderCmd{PresentationID: "p", ObjectIDs: []string{"one", "one"}, Operation: "SEND_TO_BACK"}, "duplicate"},
		{"alt text missing", &SlidesElementAltTextCmd{PresentationID: "p", ObjectID: "o"}, "provide --title"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if ExitCode(err) != 2 {
				t.Fatalf("ExitCode = %d, want 2", ExitCode(err))
			}
		})
	}
}
