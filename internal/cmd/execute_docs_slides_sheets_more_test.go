package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/app"
)

func TestExecute_DocsSlidesSheets_CopyCreateInfoCat_JSON(t *testing.T) {
	var createCalls int32
	var copyCalls int32
	var exportCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"title":      "Doc 1",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"textRun": map[string]any{
											"content": "hello",
										},
									},
								},
							},
						},
					},
				},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/presentations/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"presentationId": "p1",
				"title":          "Slides 1",
				"slides":         []any{map[string]any{"objectId": "slide1"}},
			})
			return
		case r.Method == http.MethodGet && strings.Contains(drivePath, "/files/d1") && !strings.HasSuffix(drivePath, "/copy"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "d1",
				"name":     "Doc 1",
				"mimeType": "application/vnd.google-apps.document",
			})
			return
		case r.Method == http.MethodGet && strings.Contains(drivePath, "/files/p1") && !strings.HasSuffix(drivePath, "/copy"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "p1",
				"name":     "Slides 1",
				"mimeType": "application/vnd.google-apps.presentation",
			})
			return
		case r.Method == http.MethodGet && strings.Contains(drivePath, "/files/s1") && !strings.HasSuffix(drivePath, "/copy"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "s1",
				"name":     "Sheet 1",
				"mimeType": "application/vnd.google-apps.spreadsheet",
			})
			return
		case r.Method == http.MethodPost && strings.HasSuffix(drivePath, "/copy"):
			atomic.AddInt32(&copyCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			id := "copy-unknown"
			switch {
			case strings.Contains(drivePath, "/files/d1/copy"):
				id = "d1-copy"
			case strings.Contains(drivePath, "/files/p1/copy"):
				id = "p1-copy"
			case strings.Contains(drivePath, "/files/s1/copy"):
				id = "s1-copy"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           id,
				"name":         "Copy",
				"mimeType":     "application/octet-stream",
				"webViewLink":  "https://example.com/" + id,
				"modifiedTime": "2025-12-26T00:00:00Z",
			})
			return
		case r.Method == http.MethodPost && (strings.HasSuffix(drivePath, "/files")):
			atomic.AddInt32(&createCalls, 1)
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mt, _ := req["mimeType"].(string)
			name, _ := req["name"].(string)
			id := "created"
			switch mt {
			case "application/vnd.google-apps.document":
				id = "doc-created"
			case "application/vnd.google-apps.presentation":
				id = "slides-created"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"name":        name,
				"mimeType":    mt,
				"webViewLink": "https://example.com/" + id,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	slidesSvc, err := slides.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewSlidesService: %v", err)
	}

	export := func(_ context.Context, _ *drive.Service, fileID, mimeType string) (*http.Response, error) {
		if fileID == "" || mimeType == "" {
			return nil, fmt.Errorf("invalid export request: file=%q mime=%q", fileID, mimeType)
		}
		atomic.AddInt32(&exportCalls, 1)
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("hello")),
		}, nil
	}

	runtime := &app.Runtime{Services: app.Services{
		Docs: func(context.Context, string) (*docs.Service, error) {
			return docSvc, nil
		},
		Slides: func(context.Context, string) (*slides.Service, error) {
			return slidesSvc, nil
		},
		Drive:       stubDriveService(svc),
		DriveExport: export,
	}}
	run := func(args ...string) map[string]any {
		result := executeWithTestRuntime(t, append([]string{"--json", "--account", "a@b.com"}, args...), runtime)
		if result.err != nil {
			t.Fatalf("Execute(%v): %v", args, result.err)
		}
		var parsed map[string]any
		if unmarshalErr := json.Unmarshal([]byte(result.stdout), &parsed); unmarshalErr != nil {
			t.Fatalf("json parse: %v\nout=%q", unmarshalErr, result.stdout)
		}
		return parsed
	}

	_ = run("docs", "create", "T")
	_ = run("docs", "info", "d1")
	_ = run("docs", "copy", "d1", "T2")
	gotCat := run("docs", "cat", "d1")
	if gotCat["text"] != "hello" {
		t.Fatalf("unexpected docs cat text=%v", gotCat["text"])
	}

	_ = run("slides", "create", "T")
	_ = run("slides", "info", "p1")
	_ = run("slides", "copy", "p1", "T2")

	_ = run("sheets", "copy", "s1", "T2")
	_ = run("drive", "copy", "d1", "T2")

	if atomic.LoadInt32(&createCalls) != 2 {
		t.Fatalf("createCalls=%d", createCalls)
	}
	if atomic.LoadInt32(&copyCalls) < 4 {
		t.Fatalf("copyCalls=%d", copyCalls)
	}
	if atomic.LoadInt32(&exportCalls) != 0 {
		t.Fatalf("exportCalls=%d", exportCalls)
	}
}

func TestExecute_DocsCat_WrongMime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result := executeWithTestRuntime(t,
		[]string{"--account", "a@b.com", "docs", "cat", "x1"},
		&app.Runtime{Services: app.Services{
			Docs: func(context.Context, string) (*docs.Service, error) {
				return docSvc, nil
			},
		}},
	)
	if result.err == nil {
		t.Fatal("expected error, got nil")
	}
}
