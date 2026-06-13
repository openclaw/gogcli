package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/googleapi"
)

func TestPhotosSearchBuildsReadOnlyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/mediaItems:search" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["pageSize"].(float64) != 10 {
			t.Fatalf("unexpected body: %#v", body)
		}
		filters := body["filters"].(map[string]any)
		mt := filters["mediaTypeFilter"].(map[string]any)
		if got := mt["mediaTypes"].([]any)[0]; got != "PHOTO" {
			t.Fatalf("media type = %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mediaItems": []map[string]any{{
				"id":         "m1",
				"filename":   "photo.jpg",
				"mimeType":   "image/jpeg",
				"productUrl": "https://photos.example/m1",
				"mediaMetadata": map[string]any{
					"creationTime": "2026-01-01T00:00:00Z",
				},
			}},
		})
	}))
	defer srv.Close()

	client := googleapi.NewPhotosClient(srv.Client(), googleapi.WithPhotosBaseURL(srv.URL))

	result := executeWithPhotosTestServices(t, []string{
		"--json", "--account", "a@example.com", "photos", "search",
		"--media-type", "PHOTO", "--max", "10", "--from", "2026-01-01", "--to", "2026-01-02",
	}, photosTestServices{Photos: fixedPhotosTestService(client)})
	if result.err != nil {
		t.Fatalf("Run: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed struct {
		MediaItemCount int `json:"mediaItemCount"`
		MediaItems     []struct {
			ID string `json:"id"`
		} `json:"mediaItems"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\n%s", err, result.stdout)
	}
	if parsed.MediaItemCount != 1 || len(parsed.MediaItems) != 1 || parsed.MediaItems[0].ID != "m1" {
		t.Fatalf("unexpected output: %#v", parsed)
	}
}

func TestPhotosDownloadStreamsToRuntimeOutput(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mediaItems/m1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"filename": "photo.jpg",
				"baseUrl":  srv.URL + "/media/photo",
			})
		case "/media/photo=d":
			_, _ = io.WriteString(w, "photo-bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := googleapi.NewPhotosClient(srv.Client(), googleapi.WithPhotosBaseURL(srv.URL))
	result := runWithPhotosTestServices(t, photosTestServices{
		Photos: fixedPhotosTestService(client),
	}, func(ctx context.Context) error {
		return (&PhotosDownloadCmd{MediaItemID: "m1", Out: "-"}).Run(ctx, &RootFlags{Account: "a@example.com"})
	})
	if result.err != nil {
		t.Fatalf("Run: %v\nstderr=%q", result.err, result.stderr)
	}
	if result.stdout != "photo-bytes" {
		t.Fatalf("stdout = %q, want photo bytes", result.stdout)
	}
}

func TestPhotosDownloadNotFoundExitCode(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mediaItems/missing":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "missing",
				"filename": "missing.jpg",
				"baseUrl":  srv.URL + "/media/missing",
			})
		case "/media/missing=d":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "expired")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := googleapi.NewPhotosClient(srv.Client(), googleapi.WithPhotosBaseURL(srv.URL))
	result := runWithPhotosTestServices(t, photosTestServices{
		Photos: fixedPhotosTestService(client),
	}, func(ctx context.Context) error {
		return (&PhotosDownloadCmd{MediaItemID: "missing", Out: "-"}).Run(
			ctx,
			&RootFlags{Account: "a@example.com"},
		)
	})

	if got := ExitCode(stableExitCode(result.err)); got != exitCodeNotFound {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, exitCodeNotFound, result.err)
	}
	if !strings.Contains(result.err.Error(), "HTTP 404: expired") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestPhotosValidationFailsBeforeClient(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{name: "list zero max", args: []string{"--account", "a@b.com", "photos", "list", "--max", "0"}, want: "max must be > 0"},
		{name: "list negative max", args: []string{"--account", "a@b.com", "photos", "list", "--max=-1"}, want: "max must be > 0"},
		{name: "list max above api limit", args: []string{"--account", "a@b.com", "photos", "list", "--max", "101"}, want: "max must be <= 100"},
		{name: "search zero max", args: []string{"--account", "a@b.com", "photos", "search", "--max", "0"}, want: "max must be > 0"},
		{name: "search negative max", args: []string{"--account", "a@b.com", "photos", "search", "--max=-1"}, want: "max must be > 0"},
		{name: "search max above api limit", args: []string{"--account", "a@b.com", "photos", "search", "--max", "101"}, want: "max must be <= 100"},
		{name: "search bad from", args: []string{"--account", "a@b.com", "photos", "search", "--from", "nope"}, want: "--from must be YYYY-MM-DD"},
		{name: "search bad to", args: []string{"--account", "a@b.com", "photos", "search", "--to", "nope"}, want: "--to must be YYYY-MM-DD"},
		{name: "get empty id", args: []string{"--account", "a@b.com", "photos", "get", ""}, want: "empty mediaItemId"},
		{name: "download empty id", args: []string{"--account", "a@b.com", "photos", "download", ""}, want: "empty mediaItemId"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := executeWithPhotosTestServices(t, tc.args, photosTestServices{
				Photos: unexpectedPhotosTestService(t, "expected local validation to fail before creating Photos client"),
			})
			if result.err == nil || ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), tc.want) {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}
