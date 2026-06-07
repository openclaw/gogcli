package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestSlidesInsertImage_PlacesSizedImageOnExistingSlide(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

	var captured slides.BatchUpdatePresentationRequest
	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&captured)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies":        []any{map[string]any{}},
			})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(slidesPresGetResponse("", false))
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	var deleteCalled, permsCalled bool
	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "img_123", "webContentLink": "https://drive.google.com/uc?id=img_123"})
		case strings.Contains(r.URL.Path, "/files/img_123/permissions") && r.Method == http.MethodPost:
			permsCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/img_123") && r.Method == http.MethodDelete:
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(slidesSrv.Client()), option.WithEndpoint(slidesSrv.URL+"/"))
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(driveSrv.Client()), option.WithEndpoint(driveSrv.URL+"/"))
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "logo.png")
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		cmd := &SlidesInsertImageCmd{
			PresentationID: "pres1",
			SlideID:        "existing_slide_1",
			Image:          imgPath,
			X:              560,
			Y:              24,
			Width:          120,
			Height:         60,
			Unit:           "PT",
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !permsCalled {
		t.Errorf("expected a temporary public read permission to be set")
	}
	if !deleteCalled {
		t.Errorf("expected the temporary Drive file to be deleted")
	}
	if len(captured.Requests) != 1 || captured.Requests[0].CreateImage == nil {
		t.Fatalf("expected one createImage request, got %+v", captured.Requests)
	}
	ci := captured.Requests[0].CreateImage
	ep := ci.ElementProperties
	if ep == nil || ep.PageObjectId != "existing_slide_1" {
		t.Errorf("image not placed on the target slide: %+v", ep)
	}
	if ep.Size == nil || ep.Size.Width == nil || ep.Size.Width.Magnitude != 120 || ep.Size.Width.Unit != "PT" {
		t.Errorf("unexpected width: %+v", ep.Size)
	}
	if ep.Size.Height == nil || ep.Size.Height.Magnitude != 60 {
		t.Errorf("unexpected height: %+v", ep.Size)
	}
	if ep.Transform == nil || ep.Transform.TranslateX != 560 || ep.Transform.TranslateY != 24 || ep.Transform.Unit != "PT" {
		t.Errorf("unexpected transform: %+v", ep.Transform)
	}
	if !strings.Contains(out, "image\t") || !strings.Contains(out, "/presentation/d/pres1/edit") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestSlidesInsertImage_RejectsMissingSlide(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(slidesPresGetResponse("", false))
	}))
	defer slidesSrv.Close()
	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(slidesSrv.Client()), option.WithEndpoint(slidesSrv.URL+"/"))
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("Drive should not be called when the slide is missing")
		return nil, errors.New("unexpected Drive service call")
	}

	imgPath := newTestImage(t, "logo.png")
	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)
	cmd := &SlidesInsertImageCmd{PresentationID: "pres1", SlideID: "nope", Image: imgPath, Width: 100}
	if err := cmd.Run(ctx, flags); err == nil {
		t.Fatal("expected error for missing slide")
	}
}

func TestSlidesInsertImage_WarnsWhenCleanupFails(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"presentationId": "pres1", "replies": []any{map[string]any{}}})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(slidesPresGetResponse("", false))
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "img_123"})
		case strings.Contains(r.URL.Path, "/files/img_123/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/img_123") && r.Method == http.MethodDelete:
			// Simulate a cleanup failure.
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 500, "message": "boom"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(slidesSrv.Client()), option.WithEndpoint(slidesSrv.URL+"/"))
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }
	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(driveSrv.Client()), option.WithEndpoint(driveSrv.URL+"/"))
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "logo.png")
	flags := &RootFlags{Account: "a@b.com"}

	var stderr strings.Builder
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: &stderr, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	cmd := &SlidesInsertImageCmd{PresentationID: "pres1", SlideID: "existing_slide_1", Image: imgPath, Width: 100, Height: 100, Unit: "PT"}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stderr.String(), "failed to delete temporary Drive image") {
		t.Errorf("expected a cleanup-failure warning on stderr, got: %q", stderr.String())
	}
}

func TestImageAspectRatio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	img := image.NewRGBA(image.Rect(0, 0, 200, 100)) // 2:1, so height/width = 0.5
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if encodeErr := png.Encode(f, img); encodeErr != nil {
		t.Fatalf("encode: %v", encodeErr)
	}
	_ = f.Close()

	ar, err := imageAspectRatio(path)
	if err != nil {
		t.Fatalf("imageAspectRatio: %v", err)
	}
	if ar != 0.5 {
		t.Errorf("expected aspect ratio 0.5, got %v", ar)
	}
}
