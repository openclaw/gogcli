package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func testDriveService(t *testing.T, handler http.HandlerFunc) *drive.Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("drive.NewService: %v", err)
	}
	return svc
}

func trimDrivePrefix(path string) string {
	if strings.HasPrefix(path, "/upload/drive/v3") {
		return strings.TrimPrefix(path, "/upload/drive/v3")
	}
	return strings.TrimPrefix(path, "/drive/v3")
}

func TestExtractMarkdownImages_AngleBracketRefWithSpaces(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantRef string
	}{
		{
			name:    "no title",
			content: "before ![chart](<images/weekly chart.png>) after",
			wantRef: "images/weekly chart.png",
		},
		{
			name:    "double quoted title",
			content: "![chart](<images/weekly chart.png> \"Quarterly\")",
			wantRef: "images/weekly chart.png",
		},
		{
			name:    "single quoted title",
			content: "![chart](<images/weekly chart.png> 'Quarterly')",
			wantRef: "images/weekly chart.png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, images := extractMarkdownImages(tc.content)
			if len(images) != 1 {
				t.Fatalf("expected 1 image, got %d", len(images))
			}
			if images[0].originalRef != tc.wantRef {
				t.Fatalf("originalRef = %q, want %q", images[0].originalRef, tc.wantRef)
			}
			if !strings.Contains(cleaned, "<<IMG_0>>") {
				t.Fatalf("expected placeholder in cleaned content, got %q", cleaned)
			}
		})
	}
}

func TestResolveMarkdownImagePath(t *testing.T) {
	root := t.TempDir()
	mdDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mdFile := filepath.Join(mdDir, "note.md")
	if err := os.WriteFile(mdFile, []byte("# note"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	localImg := filepath.Join(mdDir, "image.png")
	if err := os.WriteFile(localImg, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	got, err := resolveMarkdownImagePath(mdFile, "image.png")
	if err != nil {
		t.Fatalf("resolveMarkdownImagePath (inside): %v", err)
	}
	want, err := filepath.EvalSymlinks(localImg)
	if err != nil {
		t.Fatalf("EvalSymlinks(local): %v", err)
	}
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}

	outsideImg := filepath.Join(root, "outside.png")
	outsideWriteErr := os.WriteFile(outsideImg, []byte("png"), 0o644)
	if outsideWriteErr != nil {
		t.Fatalf("write outside image: %v", outsideWriteErr)
	}
	_, err = resolveMarkdownImagePath(mdFile, "../outside.png")
	if err == nil || !strings.Contains(err.Error(), "outside markdown file directory") {
		t.Fatalf("expected traversal error, got %v", err)
	}

	linkPath := filepath.Join(mdDir, "link.png")
	if err := os.Symlink(outsideImg, linkPath); err == nil {
		_, err = resolveMarkdownImagePath(mdFile, "link.png")
		if err == nil || !strings.Contains(err.Error(), "outside markdown file directory") {
			t.Fatalf("expected symlink traversal error, got %v", err)
		}
	}
}

func TestPathWithinDir_RootDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("root path semantics differ on Windows")
	}
	if !pathWithinDir("/tmp/image.png", "/") {
		t.Fatalf("expected /tmp/image.png to be considered inside /")
	}
}

func TestCleanupDriveFileIDsBestEffort_DeletesAllNonEmptyIDs(t *testing.T) {
	var (
		mu      sync.Mutex
		deleted []string
	)

	svc := testDriveService(t, func(w http.ResponseWriter, r *http.Request) {
		drivePath := trimDrivePrefix(r.URL.Path)
		if r.Method != http.MethodDelete || !strings.HasPrefix(drivePath, "/files/") {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(drivePath, "/files/")
		mu.Lock()
		deleted = append(deleted, id)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	cleanupCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cleanupDriveFileIDsBestEffort(cleanupCtx, svc, []string{"alpha", "", "beta", "  "})

	mu.Lock()
	sort.Strings(deleted)
	got := strings.Join(deleted, ",")
	mu.Unlock()

	if got != "alpha,beta" {
		t.Fatalf("deleted IDs = %q, want %q", got, "alpha,beta")
	}
}

func TestUploadLocalImage_PermissionFailureDeletesUploadedFile(t *testing.T) {
	var (
		mu         sync.Mutex
		deleteHits int
	)

	svc := testDriveService(t, func(w http.ResponseWriter, r *http.Request) {
		drivePath := trimDrivePrefix(r.URL.Path)
		switch {
		case r.Method == http.MethodPost && drivePath == "/files":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "tmp-file",
				"webContentLink": "https://example.com/content",
			})
		case r.Method == http.MethodPost && drivePath == "/files/tmp-file/permissions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 403, "message": "forbidden"}})
		case r.Method == http.MethodDelete && drivePath == "/files/tmp-file":
			mu.Lock()
			deleteHits++
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})

	imgPath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(imgPath, []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	_, _, err := uploadLocalImage(context.Background(), svc, imgPath)
	if err == nil || !strings.Contains(err.Error(), "set image permissions") {
		t.Fatalf("expected permission error, got %v", err)
	}

	mu.Lock()
	hits := deleteHits
	mu.Unlock()
	if hits != 1 {
		t.Fatalf("expected exactly 1 cleanup delete, got %d", hits)
	}
}
