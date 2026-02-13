package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestSlidesDeleteSlide(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	var deletedObjectID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				for _, rr := range req.Requests {
					if rr.DeleteObject != nil {
						deletedObjectID = rr.DeleteObject.ObjectId
					}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies":        []any{},
			})
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
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesDeleteSlideCmd{
			PresentationID: "pres1",
			SlideID:        "slide_abc",
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	_ = out
	if deletedObjectID != "slide_abc" {
		t.Errorf("expected delete of slide_abc, got %q", deletedObjectID)
	}
}

func TestSlidesDeleteSlide_EmptyID(t *testing.T) {
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

	cmd := &SlidesDeleteSlideCmd{
		PresentationID: "pres1",
		SlideID:        "  ",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty slideId") {
		t.Fatalf("expected empty slideId error, got: %v", err)
	}
}
