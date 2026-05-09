package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestSlidesReplaceText(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesReplaceTextCmd{
			PresentationID: "pres1",
			Find:           "oldName",
			Replacement:    "newName",
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ok | revisionId=rev-abc | replaced=4") {
		t.Errorf("expected confirmation with revisionId + replaced count, got: %q", out)
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

func TestSlidesReplaceText_MatchCaseAndPages(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies": []any{
			map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 1}},
		},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	_ = captureStdout(t, func() {
		cmd := &SlidesReplaceTextCmd{
			PresentationID: "pres1",
			Find:           "FooBar",
			Replacement:    "BazQux",
			MatchCase:      true,
			Pages:          []string{"slide_1", "slide_2"},
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

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
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	newSlidesService = func(context.Context, string) (*slides.Service, error) {
		t.Fatal("slides service should not be created")
		return nil, context.Canceled
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesReplaceTextCmd{
		PresentationID: "pres1",
		Find:           "old",
		Replacement:    "new",
		Pages:          []string{"   "},
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty page object ID") {
		t.Fatalf("expected empty page object ID error, got: %v", err)
	}
}

func TestSlidesReplaceText_DryRunNoAPICall(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	newSlidesService = func(context.Context, string) (*slides.Service, error) {
		t.Fatal("slides service should not be created during dry-run")
		return nil, context.Canceled
	}

	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	out := captureStdout(t, func() {
		cmd := &SlidesReplaceTextCmd{
			PresentationID: "pres1",
			Find:           "needle",
			Replacement:    "thread",
			MatchCase:      true,
			Pages:          []string{"p1"},
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var body slides.BatchUpdatePresentationRequest
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("dry-run output should be valid JSON: %v\nout=%s", err, out)
	}
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
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	newSlidesService = func(context.Context, string) (*slides.Service, error) {
		t.Fatal("slides service should not be created")
		return nil, context.Canceled
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

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
