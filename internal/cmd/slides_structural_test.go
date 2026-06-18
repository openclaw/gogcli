package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

func int64TestPtr(v int64) *int64    { return &v }
func stringTestPtr(v string) *string { return &v }

func TestSlidesNewSlide(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesNewSlideCmd{
		PresentationID: "pres1",
		Layout:         stringTestPtr("TITLE_AND_BODY"),
		Index:          int64TestPtr(0),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected 1 request, got %d", len(captured))
	}
	create := captured[0].CreateSlide
	if create == nil {
		t.Fatalf("expected CreateSlide request, got %+v", captured[0])
	}
	if create.ObjectId == "" || !strings.HasPrefix(create.ObjectId, "gogSlide") {
		t.Fatalf("unexpected generated slide ID: %q", create.ObjectId)
	}
	if create.SlideLayoutReference == nil || create.SlideLayoutReference.PredefinedLayout != "TITLE_AND_BODY" {
		t.Fatalf("unexpected layout reference: %+v", create.SlideLayoutReference)
	}
	if create.InsertionIndex != 0 {
		t.Fatalf("InsertionIndex = %d, want 0", create.InsertionIndex)
	}
	if got := strings.TrimSpace(out.String()); !strings.Contains(got, "slideObjectId\tgogSlide") || !strings.Contains(got, "index\t0") {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}

func TestSlidesNewSlideJSON(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesNewSlideCmd{PresentationID: "pres1", Layout: stringTestPtr("BLANK")}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		PresentationID string `json:"presentationId"`
		SlideObjectID  string `json:"slideObjectId"`
		Layout         string `json:"layout"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, out.String())
	}
	if got.PresentationID != "pres1" || got.Layout != "BLANK" || !strings.HasPrefix(got.SlideObjectID, "gogSlide") {
		t.Fatalf("unexpected JSON output: %#v", got)
	}
}

func TestSlidesNewSlideDefaultsToBlank(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), newSlidesServiceFromServer(t, srv))
	if err := (&SlidesNewSlideCmd{PresentationID: "pres1"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	create := captured[0].CreateSlide
	if create == nil || create.SlideLayoutReference != nil {
		t.Fatalf("default slide should omit layout reference, got %+v", create)
	}
	if !strings.Contains(out.String(), `"layout": "BLANK"`) {
		t.Fatalf("default output should report BLANK: %s", out.String())
	}
}

func TestSlidesNewSlideUsesExactLayoutID(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), newSlidesServiceFromServer(t, srv))
	cmd := &SlidesNewSlideCmd{PresentationID: "pres1", LayoutID: "layout_123"}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	create := captured[0].CreateSlide
	if create == nil || create.SlideLayoutReference == nil || create.SlideLayoutReference.LayoutId != "layout_123" {
		t.Fatalf("unexpected layout reference: %+v", create)
	}
	if !strings.Contains(out.String(), `"layoutId": "layout_123"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestSlidesNewSlideDryRunSkipsService(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesNewSlideCmd{
		PresentationID: "pres1",
		Layout:         stringTestPtr("TITLE_AND_BODY"),
		Index:          int64TestPtr(2),
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out.String())
	}
	if !got.DryRun || got.Op != "slides.new-slide" {
		t.Fatalf("unexpected dry-run envelope: %#v", got)
	}
	if len(got.Request.BatchUpdate.Requests) != 1 || got.Request.BatchUpdate.Requests[0].CreateSlide == nil {
		t.Fatalf("expected CreateSlide dry-run request, got %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesNewSlideValidation(t *testing.T) {
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	tests := []struct {
		name string
		cmd  SlidesNewSlideCmd
		want string
	}{
		{"empty presentation", SlidesNewSlideCmd{PresentationID: " ", Layout: stringTestPtr("BLANK")}, "empty presentationId"},
		{"layout conflict", SlidesNewSlideCmd{PresentationID: "pres1", Layout: stringTestPtr("BLANK"), LayoutID: "layout_123"}, "--layout and --layout-id are mutually exclusive"},
		{"negative index", SlidesNewSlideCmd{PresentationID: "pres1", Layout: stringTestPtr("BLANK"), Index: int64TestPtr(-1)}, "--index must be >= 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
	}
}

func TestSlidesDuplicateSlide(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}, map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesDuplicateSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		ToIndex:        int64TestPtr(0),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("expected duplicate and move requests, got %d", len(captured))
	}
	dup := captured[0].DuplicateObject
	if dup == nil {
		t.Fatalf("expected DuplicateObject request, got %+v", captured[0])
	}
	duplicateID := dup.ObjectIds["slide_1"]
	if dup.ObjectId != "slide_1" || !strings.HasPrefix(duplicateID, "gogDup") {
		t.Fatalf("unexpected duplicate request: %+v", dup)
	}
	move := captured[1].UpdateSlidesPosition
	if move == nil {
		t.Fatalf("expected UpdateSlidesPosition request, got %+v", captured[1])
	}
	if len(move.SlideObjectIds) != 1 || move.SlideObjectIds[0] != duplicateID || move.InsertionIndex != 0 {
		t.Fatalf("unexpected move request: %+v", move)
	}
	if got := out.String(); !strings.Contains(got, "slideObjectId\t"+duplicateID) || !strings.Contains(got, "toIndex\t0") {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestSlidesDuplicateSlideWithoutMove(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &SlidesDuplicateSlideCmd{PresentationID: "pres1", SlideID: "slide_1"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured) != 1 || captured[0].DuplicateObject == nil {
		t.Fatalf("expected one DuplicateObject request, got %+v", captured)
	}
}

func TestSlidesDuplicateSlideDryRunSkipsService(t *testing.T) {
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)
	cmd := &SlidesDuplicateSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		ToIndex:        int64TestPtr(0),
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true}); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		Op      string `json:"op"`
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, out.String())
	}
	if got.Op != "slides.duplicate-slide" || len(got.Request.BatchUpdate.Requests) != 2 {
		t.Fatalf("unexpected dry-run output: %+v", got)
	}
	if got.Request.BatchUpdate.Requests[0].DuplicateObject == nil || got.Request.BatchUpdate.Requests[1].UpdateSlidesPosition == nil {
		t.Fatalf("unexpected dry-run requests: %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesDuplicateSlideValidation(t *testing.T) {
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	tests := []struct {
		name string
		cmd  SlidesDuplicateSlideCmd
		want string
	}{
		{"empty presentation", SlidesDuplicateSlideCmd{PresentationID: " ", SlideID: "slide_1"}, "empty presentationId"},
		{"empty slide", SlidesDuplicateSlideCmd{PresentationID: "pres1", SlideID: " "}, "empty slideId"},
		{"negative to index", SlidesDuplicateSlideCmd{PresentationID: "pres1", SlideID: "slide_1", ToIndex: int64TestPtr(-1)}, "--to-index must be >= 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
	}
}

func TestSlidesMoveSlide(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesMoveSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		ToIndex:        int64TestPtr(3),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected 1 request, got %d", len(captured))
	}
	move := captured[0].UpdateSlidesPosition
	if move == nil {
		t.Fatalf("expected UpdateSlidesPosition request, got %+v", captured[0])
	}
	if len(move.SlideObjectIds) != 1 || move.SlideObjectIds[0] != "slide_1" || move.InsertionIndex != 3 {
		t.Fatalf("unexpected move request: %+v", move)
	}
	if got := out.String(); !strings.Contains(got, "slideObjectId\tslide_1") || !strings.Contains(got, "toIndex\t3") {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestSlidesMoveSlideJSON(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesMoveSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		ToIndex:        int64TestPtr(2),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		PresentationID string `json:"presentationId"`
		SlideObjectID  string `json:"slideObjectId"`
		ToIndex        int64  `json:"toIndex"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, out.String())
	}
	if got.PresentationID != "pres1" || got.SlideObjectID != "slide_1" || got.ToIndex != 2 {
		t.Fatalf("unexpected JSON output: %#v", got)
	}
}

func TestSlidesMoveSlideDryRunSkipsService(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesMoveSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		ToIndex:        int64TestPtr(0),
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out.String())
	}
	if !got.DryRun || got.Op != "slides.move-slide" {
		t.Fatalf("unexpected dry-run envelope: %#v", got)
	}
	if len(got.Request.BatchUpdate.Requests) != 1 || got.Request.BatchUpdate.Requests[0].UpdateSlidesPosition == nil {
		t.Fatalf("expected UpdateSlidesPosition dry-run request, got %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesMoveSlideValidation(t *testing.T) {
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	tests := []struct {
		name string
		cmd  SlidesMoveSlideCmd
		want string
	}{
		{"empty presentation", SlidesMoveSlideCmd{PresentationID: " ", SlideID: "slide_1", ToIndex: int64TestPtr(0)}, "empty presentationId"},
		{"empty slide", SlidesMoveSlideCmd{PresentationID: "pres1", SlideID: " ", ToIndex: int64TestPtr(0)}, "empty slideId"},
		{"missing to index", SlidesMoveSlideCmd{PresentationID: "pres1", SlideID: "slide_1"}, "--to-index is required"},
		{"negative to index", SlidesMoveSlideCmd{PresentationID: "pres1", SlideID: "slide_1", ToIndex: int64TestPtr(-1)}, "--to-index must be >= 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
	}
}
