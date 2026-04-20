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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newSlidesThumbnailTestService(t *testing.T, handler http.Handler) *slides.Service {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

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

func TestSlidesThumbnail(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	svc := newSlidesThumbnailTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1/pages/slide_1/thumbnail") && r.Method == http.MethodGet {
			if got := r.URL.Query().Get("thumbnailProperties.thumbnailSize"); got != "LARGE" {
				t.Fatalf("expected thumbnail size LARGE, got %q", got)
			}
			if got := r.URL.Query().Get("thumbnailProperties.mimeType"); got != "PNG" {
				t.Fatalf("expected mime type PNG, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contentUrl": "https://example.com/thumb.png",
				"width":      1600,
				"height":     900,
			})
			return
		}
		http.NotFound(w, r)
	}))
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesThumbnailCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "presentationId\tpres1") {
		t.Errorf("expected presentationId in output, got: %q", out)
	}
	if !strings.Contains(out, "slideId\tslide_1") {
		t.Errorf("expected slideId in output, got: %q", out)
	}
	if !strings.Contains(out, "url\thttps://example.com/thumb.png") {
		t.Errorf("expected thumbnail URL in output, got: %q", out)
	}
	if !strings.Contains(out, "width\t1600") {
		t.Errorf("expected width in output, got: %q", out)
	}
	if !strings.Contains(out, "height\t900") {
		t.Errorf("expected height in output, got: %q", out)
	}
	if !strings.Contains(out, "size\tlarge") {
		t.Errorf("expected size in output, got: %q", out)
	}
	if !strings.Contains(out, "format\tpng") {
		t.Errorf("expected format in output, got: %q", out)
	}
}

func TestSlidesThumbnail_JSON(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	svc := newSlidesThumbnailTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1/pages/slide_1/thumbnail") && r.Method == http.MethodGet {
			if got := r.URL.Query().Get("thumbnailProperties.thumbnailSize"); got != "MEDIUM" {
				t.Fatalf("expected thumbnail size MEDIUM, got %q", got)
			}
			if got := r.URL.Query().Get("thumbnailProperties.mimeType"); got != "JPEG" {
				t.Fatalf("expected mime type JPEG, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contentUrl": "https://example.com/thumb.jpg",
				"width":      800,
				"height":     450,
			})
			return
		}
		http.NotFound(w, r)
	}))
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &SlidesThumbnailCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			Size:           "medium",
			Format:         "jpeg",
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out)
	}

	if got := result["presentationId"]; got != "pres1" {
		t.Errorf("expected presentationId=pres1, got %v", got)
	}
	if got := result["slideId"]; got != "slide_1" {
		t.Errorf("expected slideId=slide_1, got %v", got)
	}
	if got := result["contentUrl"]; got != "https://example.com/thumb.jpg" {
		t.Errorf("expected contentUrl, got %v", got)
	}
	if got := result["width"]; got != float64(800) {
		t.Errorf("expected width=800, got %v", got)
	}
	if got := result["height"]; got != float64(450) {
		t.Errorf("expected height=450, got %v", got)
	}
	if got := result["size"]; got != "medium" {
		t.Errorf("expected size=medium, got %v", got)
	}
	if got := result["format"]; got != "jpeg" {
		t.Errorf("expected format=jpeg, got %v", got)
	}
}

func TestSlidesThumbnail_Download(t *testing.T) {
	origSlides := newSlidesService
	origHTTPClient := slidesThumbnailHTTPClient
	t.Cleanup(func() {
		newSlidesService = origSlides
		slidesThumbnailHTTPClient = origHTTPClient
	})

	imageBytes := []byte("fake-image-bytes")

	downloadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	t.Cleanup(downloadSrv.Close)

	svc := newSlidesThumbnailTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1/pages/slide_1/thumbnail") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contentUrl": downloadSrv.URL + "/thumb.png",
				"width":      1600,
				"height":     900,
			})
			return
		}
		http.NotFound(w, r)
	}))
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }
	slidesThumbnailHTTPClient = downloadSrv.Client()

	flags := &RootFlags{Account: "a@b.com"}
	outputPath := filepath.Join(t.TempDir(), "slide.png")

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesThumbnailCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			Output:         outputPath,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	gotBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(gotBytes) != string(imageBytes) {
		t.Fatalf("expected downloaded bytes %q, got %q", string(imageBytes), string(gotBytes))
	}

	if !strings.Contains(out, "output\t"+outputPath) {
		t.Errorf("expected output path in output, got: %q", out)
	}
	if !strings.Contains(out, "bytes\t16") {
		t.Errorf("expected byte count in output, got: %q", out)
	}
}

func TestSlidesThumbnail_InvalidSize(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Size:           "giant",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `invalid thumbnail size "giant"`) {
		t.Fatalf("expected invalid size error, got: %v", err)
	}
}

func TestSlidesThumbnail_InvalidFormat(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Format:         "gif",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `invalid thumbnail format "gif"`) {
		t.Fatalf("expected invalid format error, got: %v", err)
	}
}

func TestSlidesThumbnail_MissingSlideID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty slideId") {
		t.Fatalf("expected empty slideId error, got: %v", err)
	}
}

func TestSlidesThumbnail_MissingPresentationID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesThumbnailCmd{
		SlideID: "slide_1",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty presentationId") {
		t.Fatalf("expected empty presentationId error, got: %v", err)
	}
}

func TestSlidesThumbnail_APIFailure(t *testing.T) {
	origSlides := newSlidesService
	t.Cleanup(func() { newSlidesService = origSlides })

	svc := newSlidesThumbnailTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	}))
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "get thumbnail:") {
		t.Fatalf("expected get thumbnail error, got: %v", err)
	}
}

func TestSlidesThumbnail_DownloadFailure(t *testing.T) {
	origSlides := newSlidesService
	origHTTPClient := slidesThumbnailHTTPClient
	t.Cleanup(func() {
		newSlidesService = origSlides
		slidesThumbnailHTTPClient = origHTTPClient
	})

	downloadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(downloadSrv.Close)

	svc := newSlidesThumbnailTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1/pages/slide_1/thumbnail") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"contentUrl": downloadSrv.URL + "/thumb.png",
				"width":      1600,
				"height":     900,
			})
			return
		}
		http.NotFound(w, r)
	}))
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return svc, nil }
	slidesThumbnailHTTPClient = downloadSrv.Client()

	flags := &RootFlags{Account: "a@b.com"}
	outputPath := filepath.Join(t.TempDir(), "slide.png")

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Output:         outputPath,
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "download thumbnail: unexpected status 404 Not Found") {
		t.Fatalf("expected download failure, got: %v", err)
	}
}
