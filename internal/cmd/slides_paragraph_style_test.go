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

func TestSlidesParagraphStyleShape(t *testing.T) {
	var captured []*slides.Request
	server := mockSlidesBatchUpdateServer(t, &captured, map[string]any{})
	defer server.Close()
	var output bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), newSlidesServiceFromServer(t, server))
	args := []string{"pres1", "shape1", "--align", "center", "--direction", "right-to-left", "--line-spacing", "125", "--space-above", "0", "--indent-start", "18", "--indent-first-line", "0", "--range", "0:8"}
	if err := runKong(t, &SlidesParagraphStyleCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 || captured[0].UpdateParagraphStyle == nil {
		t.Fatalf("requests=%+v", captured)
	}
	r := captured[0].UpdateParagraphStyle
	if r.Style.Alignment != "CENTER" || r.Style.Direction != "RIGHT_TO_LEFT" || r.Style.LineSpacing != 125 {
		t.Fatalf("style=%+v", r.Style)
	}
	if r.Fields != "alignment,direction,lineSpacing,spaceAbove,indentStart,indentFirstLine" {
		t.Fatalf("fields=%s", r.Fields)
	}
	if r.TextRange.Type != "FIXED_RANGE" || r.TextRange.StartIndex == nil || *r.TextRange.StartIndex != 0 || r.TextRange.EndIndex == nil || *r.TextRange.EndIndex != 8 {
		t.Fatalf("range=%+v", r.TextRange)
	}
	if !strings.Contains(output.String(), `"objectId": "shape1"`) {
		t.Fatal(output.String())
	}
}

func TestSlidesParagraphStyleZeroFields(t *testing.T) {
	zero := float64(0)
	c := SlidesParagraphStyleCmd{SpaceAbove: &zero, SpaceBelow: &zero, IndentStart: &zero, IndentEnd: &zero, IndentFirstLine: &zero}
	style, fields, err := c.paragraphStyle()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(style)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 5 {
		t.Fatal(fields)
	}
	for _, field := range fields {
		if v, ok := obj[field]["magnitude"]; !ok || v != float64(0) {
			t.Fatalf("explicit zero missing: %s", raw)
		}
	}
}

func TestSlidesParagraphStyleCell(t *testing.T) {
	var captured slides.BatchUpdatePresentationRequest
	pres := slidesPresentationWithTableCellText(0, 0, "cell\n")
	server := mockSlidesPresentationBatchUpdateServer(t, &captured, pres, map[string]any{})
	defer server.Close()
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), newSlidesServiceFromServer(t, server))
	args := []string{"pres1", "table_1", "--row", "0", "--col", "0", "--align", "END"}
	if err := runKong(t, &SlidesParagraphStyleCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	r := captured.Requests[0].UpdateParagraphStyle
	if r.CellLocation == nil || r.CellLocation.RowIndex != 0 || r.CellLocation.ColumnIndex != 0 || r.TextRange.Type != "ALL" {
		t.Fatalf("request=%+v", r)
	}
	if captured.WriteControl == nil || captured.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatal("missing revision protection")
	}
	captured = slides.BatchUpdatePresentationRequest{}
	args = []string{"pres1", "table_1", "--row", "9", "--col", "0", "--align", "END"}
	if err := runKong(t, &SlidesParagraphStyleCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err == nil || len(captured.Requests) != 0 {
		t.Fatalf("invalid cell changed table: err=%v", err)
	}
}

func TestSlidesParagraphStyleValidationAndDryRun(t *testing.T) {
	ctx := withSlidesTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(context.Context, string) (*slides.Service, error) {
		t.Fatal("service created")
		return nil, context.Canceled
	})
	for _, opts := range [][]string{{}, {"--align", "bad"}, {"--direction", "bad"}, {"--line-spacing", "0"}, {"--space-above", "-1"}, {"--row", "0", "--align", "START"}, {"--row", "-1", "--col", "0", "--align", "START"}, {"--range", "8:0", "--align", "START"}} {
		args := append([]string{"pres1", "shape1"}, opts...)
		if err := runKong(t, &SlidesParagraphStyleCmd{}, args, ctx, &RootFlags{}); err == nil {
			t.Fatalf("accepted invalid options %v", opts)
		}
	}
	nan := math.NaN()
	if _, _, err := (&SlidesParagraphStyleCmd{IndentStart: &nan}).paragraphStyle(); err == nil {
		t.Fatal("accepted NaN")
	}
	if err := runKong(t, &SlidesParagraphStyleCmd{}, []string{"pres1", "shape1", "--align", "START"}, ctx, &RootFlags{DryRun: true}); err != nil && ExitCode(err) != 0 {
		t.Fatal(err)
	}
}

func TestSlidesParagraphStyleDryRunShowsValuesAndRange(t *testing.T) {
	for _, tc := range []struct {
		align, span, rangeType string
		cell                   bool
	}{{"START", "", "ALL", false}, {"END", "0:8", "FIXED_RANGE", true}} {
		t.Run(tc.align, func(t *testing.T) {
			var output bytes.Buffer
			ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)
			args := []string{"pres1", "target", "--align", tc.align, "--space-above", "0"}
			if tc.span != "" {
				args = append(args, "--range", tc.span)
			}
			if tc.cell {
				args = append(args, "--row", "0", "--col", "0")
			}
			err := runKong(t, &SlidesParagraphStyleCmd{}, args, ctx, &RootFlags{DryRun: true})
			if err != nil && ExitCode(err) != 0 {
				t.Fatal(err)
			}
			var result struct {
				Request struct {
					BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
				} `json:"request"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Request.BatchUpdate.Requests) != 1 {
				t.Fatalf("missing dry-run requests: %s", output.String())
			}
			request := result.Request.BatchUpdate.Requests[0].UpdateParagraphStyle
			if request == nil || request.Style.Alignment != tc.align || request.TextRange.Type != tc.rangeType || request.Style.SpaceAbove == nil {
				t.Fatalf("incomplete plan: %s", output.String())
			}
			if tc.cell && request.CellLocation == nil {
				t.Fatal("missing cell target")
			}
		})
	}
}
