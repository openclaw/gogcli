package cmd

import (
	"bytes"
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
)

func TestSlidesInsertImage_PlacesSizedImageOnExistingSlide(t *testing.T) {
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

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(driveSrv.Client()), option.WithEndpoint(driveSrv.URL+"/"))
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "logo.png")
	flags := &RootFlags{Account: "a@b.com"}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, &stdout, &stderr), slidesSvc, driveSvc)
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
	if !strings.Contains(stdout.String(), "image\t") || !strings.Contains(stdout.String(), "/presentation/d/pres1/edit") {
		t.Errorf("unexpected output: %q", stdout.String())
	}
}

func TestResolveSlidesImageSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		url  string
		want string
	}{
		{name: "missing", want: "required: image argument or --url"},
		{name: "both", file: "image.png", url: "https://example.com/image.png", want: "mutually exclusive"},
		{name: "http", url: "http://example.com/image.png", want: "public HTTPS"},
		{name: "relative", url: "image.png", want: "public HTTPS"},
		{name: "credentials", url: "https://user:pass@example.com/image.png", want: "without embedded credentials"},
		{name: "unsupported file", file: "image.bmp", want: "unsupported image format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveSlidesImageSource(tt.file, tt.url)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("resolveSlidesImageSource() error = %v, want %q", err, tt.want)
			}
		})
	}

	source, err := resolveSlidesImageSource("", "https://example.com/image.png?sig=abc")
	if err != nil {
		t.Fatalf("resolve URL source: %v", err)
	}
	if source.imageURL != "https://example.com/image.png?sig=abc" || source.localPath != "" {
		t.Fatalf("unexpected URL source: %#v", source)
	}

	source, err = resolveSlidesImageSource(" image.png", "")
	if err != nil {
		t.Fatalf("resolve local source: %v", err)
	}
	if source.localPath != " image.png" || source.mimeType != mimePNG {
		t.Fatalf("local source path was changed: %#v", source)
	}
}

func TestSlidesInsertImage_URLSkipsDrive(t *testing.T) {
	t.Parallel()

	var captured slides.BatchUpdatePresentationRequest
	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"presentationId": "pres1", "replies": []any{map[string]any{}}})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(slidesPresGetResponse("", false))
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(slidesSrv.Client()), option.WithEndpoint(slidesSrv.URL+"/"))
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}
	driveFactory := func(context.Context, string) (*drive.Service, error) {
		t.Fatal("URL insertion must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesTestService(
		withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveFactory),
		slidesSvc,
	)
	runErr := runKong(t, &SlidesInsertImageCmd{}, []string{
		"pres1",
		"existing_slide_1",
		"--url", "https://example.com/image.png?sig=abc",
		"--width", "120",
		"--height", "60",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if runErr != nil {
		t.Fatalf("slides insert-image --url: %v", runErr)
	}
	if len(captured.Requests) != 1 || captured.Requests[0].CreateImage == nil {
		t.Fatalf("unexpected requests: %#v", captured.Requests)
	}
	create := captured.Requests[0].CreateImage
	if create.Url != "https://example.com/image.png?sig=abc" {
		t.Fatalf("CreateImage URL = %q", create.Url)
	}
	if create.ElementProperties.Size.Width.Magnitude != 120 || create.ElementProperties.Size.Height.Magnitude != 60 {
		t.Fatalf("unexpected image size: %#v", create.ElementProperties.Size)
	}
}

func TestSlidesInsertImage_URLRequiresHeight(t *testing.T) {
	t.Parallel()

	err := (&SlidesInsertImageCmd{
		PresentationID: "pres1",
		SlideID:        "slide1",
		URL:            "https://example.com/image.png",
		Width:          120,
	}).Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "--height is required with --url") {
		t.Fatalf("expected URL height error, got %v", err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected usage exit code 2, got %d", ExitCode(err))
	}
}

func TestSlidesInsertImage_URLDryRunSkipsServices(t *testing.T) {
	t.Parallel()

	slidesFactory := func(context.Context, string) (*slides.Service, error) {
		t.Fatal("dry-run must not create a Slides service")
		return nil, errors.New("unexpected Slides service call")
	}
	driveFactory := func(context.Context, string) (*drive.Service, error) {
		t.Fatal("dry-run must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesTestServiceFactory(
		withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveFactory),
		slidesFactory,
	)
	err := runKong(t, &SlidesInsertImageCmd{}, []string{
		"pres1",
		"slide1",
		"--url", "https://example.com/image.png",
		"--width", "120",
		"--height", "60",
	}, ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("dry-run error = %v", err)
	}
	var payload struct {
		Op      string `json:"op"`
		Request struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if payload.Op != "slides.insert-image" || payload.Request.URL != "https://example.com/image.png" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}

func TestSlidesInsertImage_RejectsMissingSlide(t *testing.T) {
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
	driveFactory := func(context.Context, string) (*drive.Service, error) {
		t.Fatal("Drive should not be called when the slide is missing")
		return nil, errors.New("unexpected Drive service call")
	}

	imgPath := newTestImage(t, "logo.png")
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(
		withDriveTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), driveFactory),
		slidesSvc,
	)
	cmd := &SlidesInsertImageCmd{PresentationID: "pres1", SlideID: "nope", Image: imgPath, Width: 100}
	if err := cmd.Run(ctx, flags); err == nil {
		t.Fatal("expected error for missing slide")
	}
}

func TestSlidesInsertImage_WarnsWhenCleanupFails(t *testing.T) {
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
	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(), option.WithHTTPClient(driveSrv.Client()), option.WithEndpoint(driveSrv.URL+"/"))
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "logo.png")
	flags := &RootFlags{Account: "a@b.com"}

	var stderr strings.Builder
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, io.Discard, &stderr), slidesSvc, driveSvc)
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
