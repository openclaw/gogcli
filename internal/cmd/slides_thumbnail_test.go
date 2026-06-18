package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
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

	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "presentationId\tpres1") {
		t.Errorf("expected presentationId in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "slideId\tslide_1") {
		t.Errorf("expected slideId in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "url\thttps://example.com/thumb.png") {
		t.Errorf("expected thumbnail URL in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "width\t1600") {
		t.Errorf("expected width in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "height\t900") {
		t.Errorf("expected height in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "size\tlarge") {
		t.Errorf("expected size in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "format\tpng") {
		t.Errorf("expected format in output, got: %q", out.String())
	}
}

func TestSlidesThumbnail_JSON(t *testing.T) {
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

	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Size:           "medium",
		Format:         "jpeg",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out.String())
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

	flags := &RootFlags{Account: "a@b.com"}
	outputPath := filepath.Join(t.TempDir(), "slide.png")
	if err := os.WriteFile(outputPath, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Output:         outputPath,
	}
	if err := cmd.Run(ctx, flags); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected existing-file error, got %v", err)
	}
	if gotBytes, err := os.ReadFile(outputPath); err != nil || string(gotBytes) != "original" {
		t.Fatalf("existing file changed: data=%q err=%v", gotBytes, err)
	}

	cmd.Overwrite = true
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run overwrite: %v", err)
	}

	gotBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(gotBytes) != string(imageBytes) {
		t.Fatalf("expected downloaded bytes %q, got %q", string(imageBytes), string(gotBytes))
	}

	if !strings.Contains(out.String(), "output\t"+outputPath) {
		t.Errorf("expected output path in output, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "bytes\t16") {
		t.Errorf("expected byte count in output, got: %q", out.String())
	}
}

func TestSlidesThumbnail_InvalidSize(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Size:           "giant",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `invalid thumbnail size "giant"`) {
		t.Fatalf("expected invalid size error, got: %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestSlidesThumbnail_InvalidFormat(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

	cmd := &SlidesThumbnailCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Format:         "gif",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `invalid thumbnail format "gif"`) {
		t.Fatalf("expected invalid format error, got: %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

func TestSlidesThumbnail_MissingSlideID(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

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
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

	cmd := &SlidesThumbnailCmd{
		SlideID: "slide_1",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty presentationId") {
		t.Fatalf("expected empty presentationId error, got: %v", err)
	}
}

func TestSlidesThumbnail_APIFailure(t *testing.T) {
	svc := newSlidesThumbnailTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	}))

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

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

	flags := &RootFlags{Account: "a@b.com"}
	outputPath := filepath.Join(t.TempDir(), "slide.png")
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

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
