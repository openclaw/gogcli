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

func TestSlidesStyleText(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
		"writeControl":   map[string]any{"requiredRevisionId": "rev-style"},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesStyleTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Range:          "2:8",
		Bold:           true,
		Italic:         true,
		TextColor:      "#f00",
		Size:           18,
		Font:           "Georgia",
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "ok | revisionId=rev-style | replies=1") {
		t.Fatalf("expected confirmation, got %q", out.String())
	}
	if len(captured) != 1 || captured[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %+v", captured)
	}
	got := captured[0].UpdateTextStyle
	if got.ObjectId != "shape_1" || got.TextRange == nil || *got.TextRange.StartIndex != 2 || *got.TextRange.EndIndex != 8 {
		t.Fatalf("unexpected target range: %#v", got)
	}
	if got.Fields != "bold,italic,foregroundColor,fontSize,fontFamily" {
		t.Fatalf("fields = %q", got.Fields)
	}
	if got.Style == nil || !got.Style.Bold || !got.Style.Italic || got.Style.FontFamily != "Georgia" {
		t.Fatalf("unexpected style: %#v", got.Style)
	}
	if got.Style.FontSize == nil || got.Style.FontSize.Magnitude != 18 || got.Style.FontSize.Unit != "PT" {
		t.Fatalf("unexpected font size: %#v", got.Style.FontSize)
	}
	rgb := got.Style.ForegroundColor.OpaqueColor.RgbColor
	if rgb.Red != 1 || rgb.Green != 0 || rgb.Blue != 0 {
		t.Fatalf("unexpected foreground color: %#v", rgb)
	}
}

func TestSlidesStyleText_ClearBooleanStyleUsesForceSendFields(t *testing.T) {
	rng, err := parseSlidesTextRange("1:3")
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	req, fields, err := buildSlidesStyleTextRequest("shape_1", rng, slidesStyleTextOptions{NoBold: true, NoItalic: true})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if fields != "bold,italic" {
		t.Fatalf("fields = %q", fields)
	}
	if req.Style.Bold || req.Style.Italic {
		t.Fatalf("clear style booleans should be false: %#v", req.Style)
	}
	if strings.Join(req.Style.ForceSendFields, ",") != "Bold,Italic" {
		t.Fatalf("force send fields = %#v", req.Style.ForceSendFields)
	}
}

func TestSlidesLink_DryRun(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesLinkCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Range:          "4:9",
		URL:            "https://example.com",
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out.String())
	}
	reqs := got.Request.BatchUpdate.Requests
	if len(reqs) != 1 || reqs[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %+v", reqs)
	}
	style := reqs[0].UpdateTextStyle.Style
	if style == nil || style.Link == nil || style.Link.Url != "https://example.com" {
		t.Fatalf("unexpected link style: %#v", style)
	}
}

func TestSlidesLink_Clear(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{"presentationId": "pres1"})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	cmd := &SlidesLinkCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Range:          "4:9",
		Clear:          true,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured) != 1 || captured[0].UpdateTextStyle == nil {
		t.Fatalf("expected one UpdateTextStyle request, got %+v", captured)
	}
	update := captured[0].UpdateTextStyle
	if update.Fields != "link" || update.Style == nil || update.Style.Link != nil {
		t.Fatalf("expected empty link style with link field mask, got %#v", update)
	}
}

func TestSlidesBulletsOnAndOff(t *testing.T) {
	rng, err := parseSlidesTextRange("0:12")
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if rng.Type != "FIXED_RANGE" || *rng.StartIndex != 0 || *rng.EndIndex != 12 {
		t.Fatalf("range = %#v", rng)
	}

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	on := &SlidesBulletsCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Range:          "0:12",
		On:             true,
		Preset:         "BULLET_CHECKBOX",
	}
	if err := on.Run(ctx, flags); err != nil {
		t.Fatalf("on.Run: %v", err)
	}
	if len(captured) != 1 || captured[0].CreateParagraphBullets == nil {
		t.Fatalf("expected CreateParagraphBullets, got %+v", captured)
	}
	if captured[0].CreateParagraphBullets.BulletPreset != "BULLET_CHECKBOX" {
		t.Fatalf("preset = %q", captured[0].CreateParagraphBullets.BulletPreset)
	}

	off := &SlidesBulletsCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Range:          "0:12",
		Off:            true,
	}
	if err := off.Run(ctx, flags); err != nil {
		t.Fatalf("off.Run: %v", err)
	}
	if len(captured) != 1 || captured[0].DeleteParagraphBullets == nil {
		t.Fatalf("expected DeleteParagraphBullets, got %+v", captured)
	}
}

func TestSlidesTextEditValidation(t *testing.T) {
	if _, err := parseSlidesTextRange("8:8"); err == nil || !strings.Contains(err.Error(), "end must be greater") {
		t.Fatalf("expected range order error, got %v", err)
	}
	rng, err := parseSlidesTextRange("1:2")
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if _, _, err := buildSlidesStyleTextRequest("shape_1", rng, slidesStyleTextOptions{}); err == nil ||
		!strings.Contains(err.Error(), "no text style options") {
		t.Fatalf("expected empty options error, got %v", err)
	}
	if _, _, err := buildSlidesStyleTextRequest("shape_1", rng, slidesStyleTextOptions{Color: "nope"}); err == nil ||
		!strings.Contains(err.Error(), "#RRGGBB") {
		t.Fatalf("expected color error, got %v", err)
	}
	for _, cmd := range []*SlidesLinkCmd{
		{PresentationID: "pres1", ObjectID: "shape_1", Range: "1:2"},
		{PresentationID: "pres1", ObjectID: "shape_1", Range: "1:2", URL: "https://example.com", Clear: true},
	} {
		if err := cmd.Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"}); err == nil ||
			!strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("expected link mode validation error, got %v", err)
		}
	}
}
