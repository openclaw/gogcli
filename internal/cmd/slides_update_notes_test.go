package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func updateNotesPresResponse() map[string]any {
	return map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{
						"notesProperties": map[string]any{
							"speakerNotesObjectId": "notes_body_1",
						},
						"pageElements": []any{
							map[string]any{
								"objectId": "notes_body_1",
								"shape": map[string]any{
									"placeholder": map[string]any{"type": "BODY"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func ptrString(v string) *string { return &v }

func TestSlidesUpdateNotes(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var capturedRequests []*slides.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				capturedRequests = req.Requests
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies":        []any{},
			})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(updateNotesPresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesUpdateNotesCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			Notes:          ptrString("Updated notes content"),
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated notes on slide slide_1") {
		t.Errorf("expected confirmation message, got: %q", out)
	}

	// Verify batch contained DeleteText + InsertText
	if len(capturedRequests) != 2 {
		t.Fatalf("expected 2 requests in batch, got %d", len(capturedRequests))
	}
	if capturedRequests[0].DeleteText == nil {
		t.Error("expected first request to be DeleteText")
	}
	if capturedRequests[1].InsertText == nil {
		t.Error("expected second request to be InsertText")
	} else if capturedRequests[1].InsertText.Text != "Updated notes content" {
		t.Errorf("expected inserted text to be 'Updated notes content', got %q", capturedRequests[1].InsertText.Text)
	}
}

func TestSlidesUpdateNotes_NotesFile(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var insertedText string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				for _, rr := range req.Requests {
					if rr.InsertText != nil {
						insertedText = rr.InsertText.Text
					}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies":        []any{},
			})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(updateNotesPresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	notesPath := filepath.Join(t.TempDir(), "notes.md")
	notesContent := "# Updated Notes\n\nFrom a file.\n"
	if err := os.WriteFile(notesPath, []byte(notesContent), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	_ = captureStdout(t, func() {
		cmd := &SlidesUpdateNotesCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			NotesFile:      notesPath,
			Notes:          ptrString("this should be ignored"),
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if insertedText != notesContent {
		t.Errorf("expected notes from file, got: %q", insertedText)
	}
}

func TestSlidesUpdateNotes_SlideNotFound(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(updateNotesPresResponse())
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesUpdateNotesCmd{
		PresentationID: "pres1",
		SlideID:        "nonexistent",
		Notes:          ptrString("some notes"),
	}
	err = cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `slide "nonexistent" not found`) {
		t.Fatalf("expected slide-not-found error, got: %v", err)
	}
}

func TestSlidesUpdateNotes_EmptyNotes(t *testing.T) {
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

	cmd := &SlidesUpdateNotesCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "provide --notes or --notes-file") {
		t.Fatalf("expected empty notes error, got: %v", err)
	}
}

func TestSlidesUpdateNotes_ClearWithEmptyNotesFlag(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var capturedRequests []*slides.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				capturedRequests = req.Requests
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies":        []any{},
			})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(updateNotesPresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesUpdateNotesCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Notes:          ptrString(""),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(capturedRequests) != 1 {
		t.Fatalf("expected 1 request in batch for clear, got %d", len(capturedRequests))
	}
	if capturedRequests[0].DeleteText == nil {
		t.Fatal("expected DeleteText request when clearing notes")
	}
}
