package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestPhotosUploadCreatesItemsInAlbumWithDescription(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jfif")
	if err := os.WriteFile(a, []byte("jpeg-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("jpeg-b"), 0o600); err != nil {
		t.Fatal(err)
	}

	var uploads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/uploads":
			if r.Header.Get("X-Goog-Upload-Protocol") != "raw" || r.Header.Get("X-Goog-Upload-Content-Type") != "image/jpeg" {
				t.Errorf("upload headers = %v", r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			n := uploads.Add(1)
			_, _ = io.WriteString(w, "token-"+string(body)+"-"+string(rune('0'+n)))
		case r.Method == http.MethodPost && r.URL.Path == "/mediaItems:batchCreate":
			var body struct {
				AlbumID       string `json:"albumId"`
				NewMediaItems []struct {
					Description     string `json:"description"`
					SimpleMediaItem struct {
						UploadToken string `json:"uploadToken"`
						FileName    string `json:"fileName"`
					} `json:"simpleMediaItem"`
				} `json:"newMediaItems"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.AlbumID != "alb1" || len(body.NewMediaItems) != 2 {
				t.Fatalf("batchCreate body = %#v", body)
			}
			for _, it := range body.NewMediaItems {
				if it.Description != "Field trip" || !strings.HasPrefix(it.SimpleMediaItem.UploadToken, "token-") {
					t.Fatalf("item = %#v", it)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"newMediaItemResults": []map[string]any{
				{"uploadToken": body.NewMediaItems[0].SimpleMediaItem.UploadToken, "status": map[string]any{"message": "Success"}, "mediaItem": map[string]any{"id": "m1", "productUrl": "https://photos.example/m1"}},
				{"uploadToken": body.NewMediaItems[1].SimpleMediaItem.UploadToken, "status": map[string]any{"code": 3, "message": "Failed: bad image"}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := googleapi.NewPhotosClient(srv.Client(), googleapi.WithPhotosBaseURL(srv.URL))
	result := executeWithPhotosTestServices(t, []string{
		"--json", "--account", "a@example.com", "photos", "upload", a, b,
		"--album", "alb1", "--description", "Field trip",
	}, photosTestServices{Photos: fixedPhotosTestService(client)})
	if result.err == nil {
		t.Fatalf("expected an error for the failed item\nstdout=%s", result.stdout)
	}
	var parsed struct {
		Uploaded int `json:"uploaded"`
		Failed   int `json:"failed"`
		Results  []struct {
			File        string `json:"file"`
			MediaItemID string `json:"mediaItemId"`
			Status      string `json:"status"`
			Error       string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json: %v\n%s", err, result.stdout)
	}
	if parsed.Uploaded != 1 || parsed.Failed != 1 || uploads.Load() != 2 {
		t.Fatalf("unexpected: %#v uploads=%d", parsed, uploads.Load())
	}
	if parsed.Results[0].MediaItemID != "m1" || parsed.Results[0].Status != "ok" {
		t.Fatalf("first result = %#v", parsed.Results[0])
	}
	if parsed.Results[1].Status != "error" || !strings.Contains(parsed.Results[1].Error, "bad image") {
		t.Fatalf("second result = %#v", parsed.Results[1])
	}
}

func TestPhotosUploadDryRunMakesNoCalls(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.jpg")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := executeWithPhotosTestServices(t, []string{
		"--json", "--dry-run", "--account", "a@example.com", "photos", "upload", f,
	}, photosTestServices{Photos: unexpectedPhotosTestService(t, "dry run must not build a client")})
	if result.err != nil {
		t.Fatalf("Run: %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "photos.upload") {
		t.Fatalf("dry-run output = %s", result.stdout)
	}
}

func TestPhotosAlbumsCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/albums" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Album struct {
				Title string `json:"title"`
			} `json:"album"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "alb1", "title": body.Album.Title, "isWriteable": true})
	}))
	defer srv.Close()

	client := googleapi.NewPhotosClient(srv.Client(), googleapi.WithPhotosBaseURL(srv.URL))
	result := executeWithPhotosTestServices(t, []string{
		"--json", "--account", "a@example.com", "photos", "albums", "create", "School 2026",
	}, photosTestServices{Photos: fixedPhotosTestService(client)})
	if result.err != nil {
		t.Fatalf("Run: %v\nstderr=%s", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, `"id": "alb1"`) || !strings.Contains(result.stdout, "School 2026") {
		t.Fatalf("output = %s", result.stdout)
	}
}

func TestPhotosMimeType(t *testing.T) {
	cases := map[string]string{"a.JPG": "image/jpeg", "b.jfif": "image/jpeg", "c.png": "image/png", "d.heic": "image/heic"}
	for in, want := range cases {
		if got := photosMimeType(in); got != want {
			t.Errorf("photosMimeType(%q) = %q, want %q", in, got, want)
		}
	}
}
