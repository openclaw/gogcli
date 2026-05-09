package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveGetCmd_TextWithDetailsAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(r.URL.Path, "/files/file1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "file1",
			"name":          "File",
			"mimeType":      "text/plain",
			"size":          "5",
			"modifiedTime":  "2025-12-12T14:37:47Z",
			"createdTime":   "2025-12-11T00:00:00Z",
			"description":   "desc",
			"starred":       true,
			"webViewLink":   "http://example.com/file",
			"hasThumbnail":  true,
			"thumbnailLink": "https://thumb.example/file",
		})
	}))
	t.Cleanup(srv.Close)

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	u, uiErr := ui.New(ui.Options{Stdout: &outBuf, Stderr: &errBuf, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveGetCmd{}
	if execErr := runKong(t, cmd, []string{"file1"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	textOut := outBuf.String()
	if !strings.Contains(textOut, "description") || !strings.Contains(textOut, "link") {
		t.Fatalf("missing details: %q", textOut)
	}

	jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	jsonOut := captureStdout(t, func() {
		cmd := &DriveGetCmd{}
		if execErr := runKong(t, cmd, []string{"file1"}, jsonCtx, flags); execErr != nil {
			t.Fatalf("execute json: %v", execErr)
		}
	})
	if !strings.Contains(jsonOut, "\"file\"") {
		t.Fatalf("unexpected json: %q", jsonOut)
	}
	var parsed struct {
		File *drive.File `json:"file"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, jsonOut)
	}
	if parsed.File == nil || !parsed.File.HasThumbnail || parsed.File.ThumbnailLink != "https://thumb.example/file" {
		t.Fatalf("missing thumbnail fields in json: %#v", parsed.File)
	}
}

func TestDriveDownloadCmd_GoogleDoc_JSON(t *testing.T) {
	origNew := newDriveService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveExportDownload = origExport
	})

	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("docdata")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(r.URL.Path, "/files/doc1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "doc1",
			"name":     "Doc",
			"mimeType": driveMimeGoogleDoc,
		})
	}))
	t.Cleanup(srv.Close)

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	dest := filepath.Join(t.TempDir(), "out.bin")
	out := captureStdout(t, func() {
		cmd := &DriveDownloadCmd{}
		if execErr := runKong(t, cmd, []string{"doc1", "--out", dest}, ctx, flags); execErr != nil {
			t.Fatalf("download: %v", execErr)
		}
	})
	if !strings.Contains(out, "\"path\"") || !strings.Contains(out, "\"size\"") {
		t.Fatalf("unexpected json: %q", out)
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if payload.Path == "" {
		t.Fatalf("expected path in json")
	}
	if _, err := os.Stat(payload.Path); err != nil {
		t.Fatalf("expected file created: %v", err)
	}
}
