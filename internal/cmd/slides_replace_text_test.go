package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

func TestSlidesReplaceText(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies": []any{
			map[string]any{
				"replaceAllText": map[string]any{"occurrencesChanged": 4},
			},
		},
		"writeControl": map[string]any{"requiredRevisionId": "rev-abc"},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "oldName",
		Replacement:    "newName",
		All:            true,
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "ok | revisionId=rev-abc | replaced=4") {
		t.Errorf("expected confirmation with revisionId + replaced count, got: %q", out.String())
	}
	if len(captured) != 1 {
		t.Fatalf("expected 1 request in batch, got %d", len(captured))
	}
	if captured[0].ReplaceAllText == nil {
		t.Fatal("expected ReplaceAllText request")
	}
	if captured[0].ReplaceAllText.ContainsText == nil {
		t.Fatal("expected ContainsText set on request")
	}
	if captured[0].ReplaceAllText.ContainsText.Text != "oldName" {
		t.Errorf("expected find text %q, got %q", "oldName", captured[0].ReplaceAllText.ContainsText.Text)
	}
	if captured[0].ReplaceAllText.ContainsText.MatchCase {
		t.Error("expected MatchCase=false by default")
	}
	if captured[0].ReplaceAllText.ReplaceText != "newName" {
		t.Errorf("expected replacement %q, got %q", "newName", captured[0].ReplaceAllText.ReplaceText)
	}
	if len(captured[0].ReplaceAllText.PageObjectIds) != 0 {
		t.Errorf("expected no PageObjectIds, got %+v", captured[0].ReplaceAllText.PageObjectIds)
	}
}

func TestSlidesReplaceText_ObjectScopedShapeText(t *testing.T) {
	var captured []*slides.Request
	var capturedWriteControl *slides.WriteControl
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/presentations/pres1"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"revisionId":     "rev-read",
				"slides": []any{
					map[string]any{
						"objectId": "slide_1",
						"pageElements": []any{
							map[string]any{
								"objectId": "shape_1",
								"shape": map[string]any{
									"text": map[string]any{
										"textElements": []any{
											map[string]any{
												"startIndex": 0,
												"endIndex":   11,
												"textRun": map[string]any{
													"content": "🙂 " + "old " + "old\n",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				captured = req.Requests
				capturedWriteControl = req.WriteControl
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"writeControl":   map[string]any{"requiredRevisionId": "rev-object"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "fresh",
		ObjectID:       "shape_1",
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "ok | revisionId=rev-object | replaced=2") {
		t.Fatalf("expected object-scoped confirmation, got %q", out.String())
	}
	if len(captured) != 4 {
		t.Fatalf("expected delete+insert for 2 matches, got %d: %+v", len(captured), captured)
	}
	if capturedWriteControl == nil || capturedWriteControl.RequiredRevisionId != "rev-read" {
		t.Fatalf("expected write control requiredRevisionId=rev-read, got %#v", capturedWriteControl)
	}
	firstDelete := captured[0].DeleteText
	if firstDelete == nil || firstDelete.ObjectId != "shape_1" ||
		firstDelete.TextRange == nil || *firstDelete.TextRange.StartIndex != 7 || *firstDelete.TextRange.EndIndex != 10 {
		t.Fatalf("expected second match deleted first at UTF-16 range 7:10, got %#v", firstDelete)
	}
	firstInsert := captured[1].InsertText
	if firstInsert == nil || firstInsert.ObjectId != "shape_1" || firstInsert.InsertionIndex != 7 || firstInsert.Text != "fresh" {
		t.Fatalf("unexpected first insert: %#v", firstInsert)
	}
	secondDelete := captured[2].DeleteText
	if secondDelete == nil || secondDelete.TextRange == nil ||
		*secondDelete.TextRange.StartIndex != 3 || *secondDelete.TextRange.EndIndex != 6 {
		t.Fatalf("expected first match deleted second at UTF-16 range 3:6, got %#v", secondDelete)
	}
}

func TestSlidesReplaceText_ObjectScopedPreservesTextElementIndexes(t *testing.T) {
	pres := &slides.Presentation{
		Slides: []*slides.Page{{
			PageElements: []*slides.PageElement{{
				ObjectId: "shape_1",
				Shape: &slides.Shape{Text: &slides.TextContent{
					TextElements: []*slides.TextElement{
						{StartIndex: 4, EndIndex: 10, TextRun: &slides.TextRun{Content: "prefix"}},
						{StartIndex: 20, EndIndex: 23, TextRun: &slides.TextRun{Content: "old"}},
					},
				}},
			}},
		}},
	}

	requests, replaced, err := buildSlidesObjectReplaceTextRequests(pres, "shape_1", "old", "fresh", false)
	if err != nil {
		t.Fatalf("build requests: %v", err)
	}
	if replaced != 1 {
		t.Fatalf("replaced = %d, want 1", replaced)
	}
	if len(requests) != 2 {
		t.Fatalf("expected delete+insert request, got %d: %+v", len(requests), requests)
	}
	del := requests[0].DeleteText
	if del == nil || del.TextRange == nil ||
		*del.TextRange.StartIndex != 20 || *del.TextRange.EndIndex != 23 {
		t.Fatalf("delete should use Slides text element indexes 20:23, got %#v", del)
	}
	ins := requests[1].InsertText
	if ins == nil || ins.InsertionIndex != 20 || ins.Text != "fresh" {
		t.Fatalf("insert should use Slides text element index 20, got %#v", ins)
	}
	if _, _, err := buildSlidesObjectReplaceTextRequests(pres, "shape_1", "prefixold", "x", false); err == nil ||
		!strings.Contains(err.Error(), "no matching text") {
		t.Fatalf("discontinuous text runs must not be matched across an API index gap, got %v", err)
	}
}

func TestSlidesReplaceText_ObjectScopedFindsGroupedShapeText(t *testing.T) {
	pres := &slides.Presentation{
		Slides: []*slides.Page{{
			PageElements: []*slides.PageElement{{
				ObjectId: "group_1",
				ElementGroup: &slides.Group{Children: []*slides.PageElement{
					{
						ObjectId: "shape_1",
						Shape: &slides.Shape{Text: &slides.TextContent{
							TextElements: []*slides.TextElement{
								{StartIndex: 5, EndIndex: 14, TextRun: &slides.TextRun{Content: "old value"}},
							},
						}},
					},
					{
						ObjectId: "shape_2",
						Shape: &slides.Shape{Text: &slides.TextContent{
							TextElements: []*slides.TextElement{
								{StartIndex: 1, EndIndex: 10, TextRun: &slides.TextRun{Content: "untouched"}},
							},
						}},
					},
				}},
			}},
		}},
	}

	requests, replaced, err := buildSlidesObjectReplaceTextRequests(pres, "shape_1", "old", "fresh", false)
	if err != nil {
		t.Fatalf("build requests: %v", err)
	}
	if replaced != 1 {
		t.Fatalf("replaced = %d, want 1", replaced)
	}
	if len(requests) != 2 {
		t.Fatalf("expected delete+insert request, got %d: %+v", len(requests), requests)
	}
	del := requests[0].DeleteText
	if del == nil || del.ObjectId != "shape_1" || del.TextRange == nil ||
		*del.TextRange.StartIndex != 5 || *del.TextRange.EndIndex != 8 {
		t.Fatalf("delete should target grouped shape text range 5:8, got %#v", del)
	}
	ins := requests[1].InsertText
	if ins == nil || ins.ObjectId != "shape_1" || ins.InsertionIndex != 5 || ins.Text != "fresh" {
		t.Fatalf("insert should target grouped shape index 5, got %#v", ins)
	}
}

func TestSlidesReplaceText_ObjectAndPageMutuallyExclusive(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "fresh",
		ObjectID:       "shape_1",
		Pages:          []string{"slide_1"},
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}
}

func TestSlidesReplaceText_RequiresExplicitScope(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "fresh",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "explicit scope required") {
		t.Fatalf("expected explicit scope error, got: %v", err)
	}
}

func TestSlidesReplaceText_AllAndPageMutuallyExclusive(t *testing.T) {
	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "fresh",
		Pages:          []string{"slide_1"},
		All:            true,
	}
	err := cmd.Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}
}

func TestSlidesReplaceText_ObjectScopedDryRunNoAPICall(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during object-scoped dry-run")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "fresh",
		ObjectID:       "shape_1",
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			ObjectID                 string                                `json:"object_id"`
			ObjectScope              bool                                  `json:"object_scope"`
			RequiresPresentationRead bool                                  `json:"requires_presentation_read"`
			BatchUpdate              slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out.String())
	}
	if !got.DryRun {
		t.Fatalf("expected dry_run=true, got %#v", got)
	}
	if got.Request.ObjectID != "shape_1" || !got.Request.ObjectScope || !got.Request.RequiresPresentationRead {
		t.Fatalf("unexpected object-scoped dry-run payload: %#v", got.Request)
	}
	if len(got.Request.BatchUpdate.Requests) != 0 {
		t.Fatalf("object-scoped dry-run should not invent ranges without reading the presentation: %+v", got.Request.BatchUpdate.Requests)
	}
}

func TestSlidesReplaceText_MatchCaseAndPages(t *testing.T) {
	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies": []any{
			map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 1}},
		},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "FooBar",
		Replacement:    "BazQux",
		MatchCase:      true,
		Pages:          []string{"slide_1", "slide_2"},
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	if len(captured) != 1 || captured[0].ReplaceAllText == nil {
		t.Fatalf("expected 1 ReplaceAllText request, got %+v", captured)
	}
	if !captured[0].ReplaceAllText.ContainsText.MatchCase {
		t.Error("expected MatchCase=true")
	}
	got := captured[0].ReplaceAllText.PageObjectIds
	want := []string{"slide_1", "slide_2"}
	if len(got) != len(want) {
		t.Fatalf("expected %d pageObjectIds, got %d (%+v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pageObjectIds[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSlidesReplaceText_BlankPageID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "fresh",
		Pages:          []string{"   "},
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty page object ID") {
		t.Fatalf("expected empty page object ID error, got: %v", err)
	}
}

func TestSlidesReplaceText_DryRunNoAPICall(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	var out bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &out, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created during dry-run")
			return nil, context.Canceled
		},
	)
	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "needle",
		Replacement:    "thread",
		MatchCase:      true,
		Pages:          []string{"p1"},
	}
	if err := cmd.Run(ctx, flags); err != nil && ExitCode(err) != 0 {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			BatchUpdate slides.BatchUpdatePresentationRequest `json:"batch_update"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out.String())
	}
	if !got.DryRun {
		t.Fatalf("expected dry_run=true, got %#v", got)
	}
	body := got.Request.BatchUpdate
	if len(body.Requests) != 1 || body.Requests[0].ReplaceAllText == nil {
		t.Fatalf("expected single ReplaceAllText request in dry-run, got %+v", body.Requests)
	}
	rr := body.Requests[0].ReplaceAllText
	if rr.ReplaceText != "thread" || rr.ContainsText.Text != "needle" || !rr.ContainsText.MatchCase {
		t.Errorf("unexpected dry-run request body: %+v", rr)
	}
	if len(rr.PageObjectIds) != 1 || rr.PageObjectIds[0] != "p1" {
		t.Errorf("expected pageObjectIds=[p1], got %+v", rr.PageObjectIds)
	}
}

func TestSlidesReplaceText_EmptyFind(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*slides.Service, error) {
			t.Fatal("slides service should not be created")
			return nil, context.Canceled
		},
	)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "",
		Replacement:    "anything",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty find") {
		t.Fatalf("expected empty find error, got: %v", err)
	}
}
