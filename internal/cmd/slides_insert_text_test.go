package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

// mockSlidesBatchUpdateServer spins up an httptest.Server that captures the
// batchUpdate request body and returns a canned BatchUpdatePresentationResponse.
// Tests can inspect captured requests via the returned pointer.
func mockSlidesBatchUpdateServer(
	t *testing.T,
	captured *[]*slides.Request,
	response map[string]any,
) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost {
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				*captured = req.Requests
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	return srv
}

func newSlidesServiceFromServer(t *testing.T, srv *httptest.Server) *slides.Service {
	t.Helper()
	svc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	return svc
}

func TestSlidesInsertText(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
		"writeControl":   map[string]any{"requiredRevisionId": "rev-123"},
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

		cmd := &SlidesInsertTextCmd{
			PresentationID: "pres1",
			ObjectID:       "shape_1",
			Text:           "hello world",
			InsertionIndex: 3,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ok | revisionId=rev-123 | replies=1") {
		t.Errorf("expected plain confirmation with revisionId and replies, got: %q", out)
	}
	if len(captured) != 1 {
		t.Fatalf("expected 1 request in batch, got %d", len(captured))
	}
	if captured[0].InsertText == nil {
		t.Fatal("expected InsertText request")
	}
	if captured[0].InsertText.Text != "hello world" {
		t.Errorf("expected text %q, got %q", "hello world", captured[0].InsertText.Text)
	}
	if captured[0].InsertText.ObjectId != "shape_1" {
		t.Errorf("expected objectId shape_1, got %q", captured[0].InsertText.ObjectId)
	}
	if captured[0].InsertText.InsertionIndex != 3 {
		t.Errorf("expected insertionIndex 3, got %d", captured[0].InsertText.InsertionIndex)
	}
}

func TestSlidesInsertText_ReplaceEmitsDeleteThenInsert(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}, map[string]any{}},
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
		cmd := &SlidesInsertTextCmd{
			PresentationID: "pres1",
			ObjectID:       "shape_1",
			Text:           "replacement",
			Replace:        true,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if len(captured) != 2 {
		t.Fatalf("expected 2 requests (DeleteText + InsertText), got %d", len(captured))
	}
	if captured[0].DeleteText == nil {
		t.Error("expected first request to be DeleteText")
	} else if captured[0].DeleteText.TextRange == nil || captured[0].DeleteText.TextRange.Type != "ALL" {
		t.Errorf("expected DeleteText TextRange.Type=ALL, got %+v", captured[0].DeleteText.TextRange)
	}
	if captured[1].InsertText == nil {
		t.Error("expected second request to be InsertText")
	} else if captured[1].InsertText.Text != "replacement" {
		t.Errorf("expected inserted text %q, got %q", "replacement", captured[1].InsertText.Text)
	}
}

func TestSlidesInsertText_ReplaceEmptyClearsOnly(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
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
		cmd := &SlidesInsertTextCmd{
			PresentationID: "pres1",
			ObjectID:       "shape_1",
			Text:           "",
			Replace:        true,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if len(captured) != 1 {
		t.Fatalf("expected 1 DeleteText request, got %d", len(captured))
	}
	if captured[0].DeleteText == nil {
		t.Fatalf("expected DeleteText request, got %+v", captured[0])
	}
}

func TestSlidesInsertText_StdinDash(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var captured []*slides.Request
	srv := mockSlidesBatchUpdateServer(t, &captured, map[string]any{
		"presentationId": "pres1",
		"replies":        []any{map[string]any{}},
	})
	defer srv.Close()

	svc := newSlidesServiceFromServer(t, srv)
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	// Pipe text into stdin.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	const piped = "from-stdin content\nline 2\n"
	go func() {
		_, _ = w.Write([]byte(piped))
		_ = w.Close()
	}()

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	_ = captureStdout(t, func() {
		cmd := &SlidesInsertTextCmd{
			PresentationID: "pres1",
			ObjectID:       "shape_1",
			Text:           "-",
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if len(captured) != 1 || captured[0].InsertText == nil {
		t.Fatalf("expected single InsertText request, got %+v", captured)
	}
	if captured[0].InsertText.Text != piped {
		t.Errorf("expected piped text %q, got %q", piped, captured[0].InsertText.Text)
	}
}

func TestSlidesInsertText_DryRunNoAPICall(t *testing.T) {
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
		cmd := &SlidesInsertTextCmd{
			PresentationID: "pres1",
			ObjectID:       "shape_1",
			Text:           "dry",
			Replace:        true,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var body slides.BatchUpdatePresentationRequest
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("dry-run output should be valid JSON BatchUpdatePresentationRequest: %v\nout=%s", err, out)
	}
	if len(body.Requests) != 2 {
		t.Fatalf("expected 2 requests in dry-run body, got %d", len(body.Requests))
	}
	if body.Requests[0].DeleteText == nil || body.Requests[1].InsertText == nil {
		t.Errorf("expected DeleteText then InsertText in dry-run body, got %+v", body.Requests)
	}
}

func TestSlidesInsertText_EmptyTextWithoutReplace(t *testing.T) {
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

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "shape_1",
		Text:           "",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty text") {
		t.Fatalf("expected empty text error, got: %v", err)
	}
}

func TestSlidesInsertText_EmptyObjectID(t *testing.T) {
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

	cmd := &SlidesInsertTextCmd{
		PresentationID: "pres1",
		ObjectID:       "",
		Text:           "something",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty objectId") {
		t.Fatalf("expected empty objectId error, got: %v", err)
	}
}
