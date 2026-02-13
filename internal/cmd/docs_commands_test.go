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

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestDocsCreateCopyCat_JSON(t *testing.T) {
	origNew := newDriveService
	origDocs := newDocsService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		newDocsService = origDocs
		driveExportDownload = origExport
	})

	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("doc text")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		switch {
		case strings.HasPrefix(path, "/v1/documents/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"title":      "Doc",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"textRun": map[string]any{
											"content": "doc text",
										},
									},
								},
							},
						},
					},
				},
			})
			return
		case strings.HasPrefix(drivePath, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "doc1",
				"mimeType": "application/vnd.google-apps.document",
			})
			return
		case drivePath == "/files" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc1",
			})
			return
		case strings.Contains(drivePath, "/files/doc1/copy") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc2",
				"name":        "Copy",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc2",
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
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		cmd := &DocsCreateCmd{}
		if err := runKong(t, cmd, []string{"Doc"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &DocsCopyCmd{}
		if err := runKong(t, cmd, []string{"doc1", "Copy"}, ctx, flags); err != nil {
			t.Fatalf("copy: %v", err)
		}
	})

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if !strings.Contains(out, "doc text") {
		t.Fatalf("unexpected cat output: %q", out)
	}
}

func TestDocsCreateCopyCat_Text(t *testing.T) {
	origNew := newDriveService
	origDocs := newDocsService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		newDocsService = origDocs
		driveExportDownload = origExport
	})

	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("doc text")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		switch {
		case strings.HasPrefix(path, "/v1/documents/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"title":      "Doc",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"textRun": map[string]any{
											"content": "doc text",
										},
									},
								},
							},
						},
					},
				},
			})
			return
		case strings.HasPrefix(drivePath, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "doc1",
				"mimeType": "application/vnd.google-apps.document",
			})
			return
		case drivePath == "/files" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc1",
			})
			return
		case strings.Contains(drivePath, "/files/doc1/copy") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc2",
				"name":        "Copy",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc2",
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
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		createCmd := &DocsCreateCmd{}
		if err := runKong(t, createCmd, []string{"Doc"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}

		copyCmd := &DocsCopyCmd{}
		if err := runKong(t, copyCmd, []string{"doc1", "Copy"}, ctx, flags); err != nil {
			t.Fatalf("copy: %v", err)
		}

		catCmd := &DocsCatCmd{}
		if err := runKong(t, catCmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if !strings.Contains(out, "doc text") || !strings.Contains(out, "id\tdoc1") {
		t.Fatalf("unexpected output: %q", out)
	}
}
