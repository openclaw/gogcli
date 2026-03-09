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

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
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
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "replacement.png")
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesReplaceSlideCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			Image:          imgPath,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Replaced image on slide 1") {
		t.Errorf("expected confirmation, got: %q", out)
	}
	if !strings.Contains(out, "link\thttps://docs.google.com/presentation/d/pres1/edit") {
		t.Errorf("expected link, got: %q", out)
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

func TestSlidesReplaceSlide_WithNotes(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "replacement.jpg")
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &SlidesReplaceSlideCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			Image:          imgPath,
			Notes:          ptrString("New notes for replaced slide"),
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated speaker notes") {
		t.Errorf("expected notes update confirmation, got: %q", out)
	}

	// Should have ReplaceImage + DeleteText + InsertText = 3 requests
	if len(capturedRequests) != 3 {
		t.Fatalf("expected 3 requests in batch, got %d", len(capturedRequests))
	}
	if capturedRequests[0].ReplaceImage == nil {
		t.Error("expected first request to be ReplaceImage")
	}
	if capturedRequests[1].DeleteText == nil {
		t.Error("expected second request to be DeleteText")
	}
	if capturedRequests[2].InsertText == nil {
		t.Error("expected third request to be InsertText")
	} else if capturedRequests[2].InsertText.Text != "New notes for replaced slide" {
		t.Errorf("expected notes text, got %q", capturedRequests[2].InsertText.Text)
	}
}

func TestSlidesReplaceSlide_JSON(t *testing.T) {
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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "test.png")
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &SlidesReplaceSlideCmd{
			PresentationID: "pres1",
			SlideID:        "slide_1",
			Image:          imgPath,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out)
	}
	if result["slideNumber"] != float64(1) {
		t.Errorf("expected slideNumber=1, got %v", result["slideNumber"])
	}
	if result["slideObjectId"] != "slide_1" {
		t.Errorf("expected slideObjectId=slide_1, got %v", result["slideObjectId"])
	}
}

func TestSlidesReplaceSlide_SlideNotFound(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "test.png")
	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

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
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "test.png")
	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

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

func TestSlidesReplaceSlide_ClearNotesWithEmptyFlag(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "replacement-clear.png")
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SlidesReplaceSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
		Image:          imgPath,
		Notes:          ptrString(""),
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(capturedRequests) != 2 {
		t.Fatalf("expected ReplaceImage + DeleteText (2 requests), got %d", len(capturedRequests))
	}
	if capturedRequests[0].ReplaceImage == nil {
		t.Fatal("expected first request to be ReplaceImage")
	}
	if capturedRequests[1].DeleteText == nil {
		t.Fatal("expected second request to be DeleteText")
	}
}

func TestSlidesReplaceSlide_WithNotes_MissingPlaceholderFails(t *testing.T) {
	origSlides := newSlidesService
	origDrive := newDriveService
	t.Cleanup(func() {
		newSlidesService = origSlides
		newDriveService = origDrive
	})

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
	newSlidesService = func(context.Context, string) (*slides.Service, error) { return slidesSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(driveSrv.Client()),
		option.WithEndpoint(driveSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	imgPath := newTestImage(t, "replacement-missing-notes.png")
	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

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
