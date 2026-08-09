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

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestDriveGetCmd_TextWithDetailsAndJSON(t *testing.T) {
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

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, &outBuf, &errBuf), svc)

	cmd := &DriveGetCmd{}
	if execErr := runKong(t, cmd, []string{"file1"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	textOut := outBuf.String()
	if !strings.Contains(textOut, "description") || !strings.Contains(textOut, "link") {
		t.Fatalf("missing details: %q", textOut)
	}

	var jsonOut bytes.Buffer
	jsonCtx := outfmt.WithMode(withDriveTestService(newCmdRuntimeOutputContext(t, &jsonOut, io.Discard), svc), outfmt.Mode{JSON: true})
	cmd = &DriveGetCmd{}
	if execErr := runKong(t, cmd, []string{"file1"}, jsonCtx, flags); execErr != nil {
		t.Fatalf("execute json: %v", execErr)
	}
	if !strings.Contains(jsonOut.String(), "\"file\"") {
		t.Fatalf("unexpected json: %q", jsonOut.String())
	}
	var parsed struct {
		File *drive.File `json:"file"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, jsonOut.String())
	}
	if parsed.File == nil || !parsed.File.HasThumbnail || parsed.File.ThumbnailLink != "https://thumb.example/file" {
		t.Fatalf("missing thumbnail fields in json: %#v", parsed.File)
	}
}

func TestDriveDownloadCmd_GoogleDoc_JSON(t *testing.T) {
	export := func(context.Context, *drive.Service, string, string) (*http.Response, error) {
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

	flags := &RootFlags{Account: "a@b.com"}
	dest := filepath.Join(t.TempDir(), "out.bin")
	var out bytes.Buffer
	ctx := withDriveTestOperations(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc, nil, export)
	cmd := &DriveDownloadCmd{}
	if execErr := runKong(t, cmd, []string{"doc1", "--out", dest}, ctx, flags); execErr != nil {
		t.Fatalf("download: %v", execErr)
	}
	if !strings.Contains(out.String(), "\"path\"") || !strings.Contains(out.String(), "\"size\"") {
		t.Fatalf("unexpected json: %q", out.String())
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if payload.Path == "" {
		t.Fatalf("expected path in json")
	}
	if _, err := os.Stat(payload.Path); err != nil {
		t.Fatalf("expected file created: %v", err)
	}
}
