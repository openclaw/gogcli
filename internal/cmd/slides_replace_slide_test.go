package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
)

func replaceSlidePresResponse() map[string]any {
	return map[string]any{
		"presentationId": "pres1",
		"pageSize": map[string]any{
			"width":  map[string]any{"magnitude": 9144000, "unit": "EMU"},
			"height": map[string]any{"magnitude": 5143500, "unit": "EMU"},
		},
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
				"pageElements": []any{
					map[string]any{
						"objectId": "img_on_slide",
						"image": map[string]any{
							"contentUrl": "https://example.com/old.png",
						},
					},
				},
			},
			map[string]any{
				"objectId": "slide_2",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{},
				},
				"pageElements": []any{},
			},
		},
	}
}

func TestSlidesReplaceSlide(t *testing.T) {
	var capturedRequests []*slides.Request

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_ = json.NewEncoder(w).Encode(replaceSlidePresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	var deleteCalled bool
	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "new_img_123",
				"webContentLink": "https://drive.google.com/uc?id=new_img_123",
			})
		case strings.Contains(r.URL.Path, "/files/new_img_123/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/new_img_123") && r.Method == http.MethodDelete:
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "replacement.png")
	flags := &RootFlags{Account: "a@b.com"}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, &stdout, &stderr), slidesSvc, driveSvc)
	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(stdout.String(), "Replaced image on slide 1") {
		t.Errorf("expected confirmation, got: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "link\thttps://docs.google.com/presentation/d/pres1/edit") {
		t.Errorf("expected link, got: %q", stdout.String())
	}

	// Should use ReplaceImage request (only 1 request, no notes update)
	if len(capturedRequests) != 1 {
		t.Fatalf("expected 1 request in batch, got %d", len(capturedRequests))
	}
	if capturedRequests[0].ReplaceImage == nil {
		t.Error("expected ReplaceImage request")
	} else if capturedRequests[0].ReplaceImage.ImageObjectId != "img_on_slide" {
		t.Errorf("expected image object ID img_on_slide, got %q", capturedRequests[0].ReplaceImage.ImageObjectId)
	}

	if !deleteCalled {
		t.Error("expected Drive file cleanup")
	}
}

func TestSlidesReplaceSlide_URLSkipsDrive(t *testing.T) {
	t.Parallel()

	var capturedRequests []*slides.Request
	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			var req slides.BatchUpdatePresentationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			capturedRequests = req.Requests
			_ = json.NewEncoder(w).Encode(map[string]any{"presentationId": "pres1", "replies": []any{}})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(replaceSlidePresResponse())
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
		t.Fatal("URL replacement must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesTestService(
		withDriveTestServiceFactory(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveFactory),
		slidesSvc,
	)
	runErr := runKong(t, &SlidesReplaceSlideCmd{}, []string{
		"pres1",
		"slide_1",
		"--url", "https://example.com/replacement.png?sig=abc",
	}, ctx, &RootFlags{Account: "a@b.com"})
	if runErr != nil {
		t.Fatalf("slides replace-slide --url: %v", runErr)
	}
	if len(capturedRequests) != 1 || capturedRequests[0].ReplaceImage == nil {
		t.Fatalf("unexpected requests: %#v", capturedRequests)
	}
	replace := capturedRequests[0].ReplaceImage
	if replace.ImageObjectId != "img_on_slide" || replace.Url != "https://example.com/replacement.png?sig=abc" {
		t.Fatalf("unexpected ReplaceImage request: %#v", replace)
	}
}

func TestSlidesReplaceSlide_URLDryRunSkipsServices(t *testing.T) {
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
	err := runKong(t, &SlidesReplaceSlideCmd{}, []string{
		"pres1",
		"slide_1",
		"--url", "https://example.com/replacement.png",
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
	if payload.Op != "slides.replace-slide" || payload.Request.URL != "https://example.com/replacement.png" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}

func TestSlidesReplaceSlide_WithNotes(t *testing.T) {
	var capturedRequests []*slides.Request

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_ = json.NewEncoder(w).Encode(replaceSlidePresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "new_img_456",
				"webContentLink": "https://drive.google.com/uc?id=new_img_456",
			})
		case strings.Contains(r.URL.Path, "/files/new_img_456/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/new_img_456") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "replacement.jpg")
	flags := &RootFlags{Account: "a@b.com"}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, &stdout, &stderr), slidesSvc, driveSvc)
	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
		Notes:          ptrString("New notes for replaced slide"),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(stdout.String(), "Updated speaker notes") {
		t.Errorf("expected notes update confirmation, got: %q", stdout.String())
	}

	// Blank notes placeholders must not be cleared before insertion; Google
	// rejects DeleteText{ALL} on an empty notes box.
	if len(capturedRequests) != 2 {
		t.Fatalf("expected ReplaceImage + InsertText (2 requests), got %d", len(capturedRequests))
	}
	if capturedRequests[0].ReplaceImage == nil {
		t.Error("expected first request to be ReplaceImage")
	}
	if capturedRequests[1].InsertText == nil {
		t.Error("expected second request to be InsertText")
	} else if capturedRequests[1].InsertText.Text != "New notes for replaced slide" {
		t.Errorf("expected notes text, got %q", capturedRequests[1].InsertText.Text)
	}
}

func TestSlidesReplaceSlide_JSON(t *testing.T) {
	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "pres1",
				"replies":        []any{},
			})
		case strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(replaceSlidePresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "new_img_json",
				"webContentLink": "https://drive.google.com/uc?id=new_img_json",
			})
		case strings.Contains(r.URL.Path, "/files/new_img_json/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/new_img_json") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "test.png")
	flags := &RootFlags{Account: "a@b.com"}

	var stdout, stderr bytes.Buffer
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), slidesSvc, driveSvc)
	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, stdout.String())
	}
	if result["slideNumber"] != float64(1) {
		t.Errorf("expected slideNumber=1, got %v", result["slideNumber"])
	}
	if result["slideObjectId"] != "slide_1" {
		t.Errorf("expected slideObjectId=slide_1, got %v", result["slideObjectId"])
	}
}

func TestSlidesReplaceSlide_SlideNotFound(t *testing.T) {
	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(replaceSlidePresResponse())
			return
		}
		http.NotFound(w, r)
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "img_nf",
				"webContentLink": "https://drive.google.com/uc?id=img_nf",
			})
		case strings.Contains(r.URL.Path, "/files/img_nf/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/img_nf") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "test.png")
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), slidesSvc, driveSvc)

	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "nonexistent",
		Image:          imgPath,
	}
	err = cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `slide "nonexistent" not found`) {
		t.Fatalf("expected slide-not-found error, got: %v", err)
	}
}

func TestSlidesReplaceSlide_NoImage(t *testing.T) {
	// Slide with no image element
	presResp := map[string]any{
		"presentationId": "pres1",
		"pageSize": map[string]any{
			"width":  map[string]any{"magnitude": 9144000, "unit": "EMU"},
			"height": map[string]any{"magnitude": 5143500, "unit": "EMU"},
		},
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{},
				},
				"pageElements": []any{},
			},
		},
	}

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(presResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "img_noimg",
				"webContentLink": "https://drive.google.com/uc?id=img_noimg",
			})
		case strings.Contains(r.URL.Path, "/files/img_noimg/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/img_noimg") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "test.png")
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), slidesSvc, driveSvc)

	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
	}
	err = cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "no image found on slide") {
		t.Fatalf("expected no-image error, got: %v", err)
	}
}

func TestSlidesReplaceSlide_UnsupportedFormat(t *testing.T) {
	imgPath := newTestImage(t, "replacement.bmp")
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
	}
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "unsupported image format") {
		t.Fatalf("expected unsupported format error, got: %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestSlidesReplaceSlide_ClearNotesWithEmptyFlag(t *testing.T) {
	var capturedRequests []*slides.Request

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_ = json.NewEncoder(w).Encode(replaceSlidePresResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "new_img_clear",
				"webContentLink": "https://drive.google.com/uc?id=new_img_clear",
			})
		case strings.Contains(r.URL.Path, "/files/new_img_clear/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/new_img_clear") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "replacement-clear.png")
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), slidesSvc, driveSvc)

	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
		Notes:          ptrString(""),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(capturedRequests) != 1 {
		t.Fatalf("expected ReplaceImage only for already-empty notes, got %d", len(capturedRequests))
	}
	if capturedRequests[0].ReplaceImage == nil {
		t.Fatal("expected first request to be ReplaceImage")
	}
}

func TestSlidesReplaceSlide_WithNotes_MissingPlaceholderFails(t *testing.T) {
	presResp := map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{},
				},
				"pageElements": []any{
					map[string]any{
						"objectId": "img_on_slide",
						"image":    map[string]any{"contentUrl": "https://example.com/old.png"},
					},
				},
			},
		},
	}

	slidesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") && r.Method == http.MethodPost {
			t.Fatal("batchUpdate should not be called when notes placeholder is missing")
		}
		if strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(presResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer slidesSrv.Close()

	driveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "new_img_missing_notes",
				"webContentLink": "https://drive.google.com/uc?id=new_img_missing_notes",
			})
		case strings.Contains(r.URL.Path, "/files/new_img_missing_notes/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/new_img_missing_notes") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer driveSrv.Close()

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(slidesSrv.Client()),
		option.WithEndpoint(slidesSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("slides.NewService: %v", err)
	}

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}

	imgPath := newTestImage(t, "replacement-missing-notes.png")
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesAndDriveTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), slidesSvc, driveSvc)

	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
		Notes:          ptrString("new notes"),
	}
	err = cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "could not find speaker notes placeholder") {
		t.Fatalf("expected missing-notes-placeholder error, got: %v", err)
	}
}
