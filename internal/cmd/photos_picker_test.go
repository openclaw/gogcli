package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/googleapi"
)

func TestPhotosPickerCommandWorkflow(t *testing.T) {
	var sessionGets atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sessions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create: %v", err)
			}
			if body["pickingConfig"].(map[string]any)["maxItemCount"] != "2" {
				t.Fatalf("create body = %#v", body)
			}
			_, _ = io.WriteString(w, `{
				"id":"session-1",
				"pickerUri":"https://photos.google.com/picker/session-1",
				"expireTime":"2026-06-11T13:00:00Z",
				"pollingConfig":{"pollInterval":"0.001s","timeoutIn":"1s"},
				"pickingConfig":{"maxItemCount":"2"}
			}`)
		case r.Method == http.MethodGet && r.URL.Path == "/sessions/session-1":
			ready := sessionGets.Add(1) > 1
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "session-1",
				"mediaItemsSet": ready,
				"pollingConfig": map[string]any{
					"pollInterval": "0.001s",
					"timeoutIn":    "1s",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/mediaItems":
			if r.URL.Query().Get("sessionId") != "session-1" {
				t.Fatalf("session query = %q", r.URL.Query().Get("sessionId"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mediaItems": []map[string]any{{
					"id":         "photo-1",
					"type":       "PHOTO",
					"createTime": "2026-06-10T12:00:00Z",
					"mediaFile": map[string]any{
						"baseUrl":  srv.URL + "/media/photo",
						"mimeType": "image/jpeg",
						"filename": "picked.jpg",
						"mediaFileMetadata": map[string]any{
							"width":  1200,
							"height": 800,
						},
					},
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/media/photo=d":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = io.WriteString(w, "picked-photo")
		case r.Method == http.MethodDelete && r.URL.Path == "/sessions/session-1":
			_, _ = io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL))
	openedURI := ""
	services := photosTestServices{
		PhotosPicker: fixedPhotosPickerTestService(client),
		OpenURL: func(_ context.Context, uri string) error {
			openedURI = uri
			return nil
		},
	}
	flags := &RootFlags{Account: "a@example.com"}

	createResult := runWithPhotosTestServices(t, services, func(ctx context.Context) error {
		cmd := &PhotosPickerCreateCmd{MaxItems: 2, Open: true}
		return cmd.Run(ctx, flags)
	})
	if createResult.err != nil {
		t.Fatalf("create: %v\nstderr=%q", createResult.err, createResult.stderr)
	}
	if !strings.Contains(createResult.stdout, `"id": "session-1"`) {
		t.Fatalf("create output = %s", createResult.stdout)
	}
	if openedURI != "https://photos.google.com/picker/session-1" {
		t.Fatalf("opened URI = %q", openedURI)
	}

	waitResult := runWithPhotosTestServices(t, services, func(ctx context.Context) error {
		cmd := &PhotosPickerWaitCmd{SessionID: "session-1", Timeout: time.Second}
		return cmd.Run(ctx, flags)
	})
	if waitResult.err != nil {
		t.Fatalf("wait: %v\nstderr=%q", waitResult.err, waitResult.stderr)
	}
	if !strings.Contains(waitResult.stdout, `"mediaItemsSet": true`) {
		t.Fatalf("wait output = %s", waitResult.stdout)
	}

	listResult := runWithPhotosTestServices(t, services, func(ctx context.Context) error {
		cmd := &PhotosPickerListCmd{SessionID: "session-1", Max: 50}
		return cmd.Run(ctx, flags)
	})
	if listResult.err != nil {
		t.Fatalf("list: %v\nstderr=%q", listResult.err, listResult.stderr)
	}
	if !strings.Contains(listResult.stdout, `"id": "photo-1"`) {
		t.Fatalf("list output = %s", listResult.stdout)
	}

	stdoutDownload := runWithPhotosTestServices(t, services, func(ctx context.Context) error {
		return (&PhotosPickerDownloadCmd{
			SessionID:   "session-1",
			MediaItemID: "photo-1",
			Out:         "-",
		}).Run(ctx, flags)
	})
	if stdoutDownload.err != nil {
		t.Fatalf("stdout download: %v\nstderr=%q", stdoutDownload.err, stdoutDownload.stderr)
	}
	if stdoutDownload.stdout != "picked-photo" {
		t.Fatalf("stdout download = %q", stdoutDownload.stdout)
	}

	outputPath := filepath.Join(t.TempDir(), "picked.jpg")
	downloadResult := runWithPhotosTestServices(t, services, func(ctx context.Context) error {
		cmd := &PhotosPickerDownloadCmd{
			SessionID:   "session-1",
			MediaItemID: "photo-1",
			Out:         outputPath,
		}
		return cmd.Run(ctx, flags)
	})
	if downloadResult.err != nil {
		t.Fatalf("download: %v\nstderr=%q", downloadResult.err, downloadResult.stderr)
	}
	if !strings.Contains(downloadResult.stdout, `"bytes": 12`) {
		t.Fatalf("download output = %s", downloadResult.stdout)
	}
	downloaded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if string(downloaded) != "picked-photo" {
		t.Fatalf("downloaded = %q", downloaded)
	}

	deleteResult := runWithPhotosTestServices(t, services, func(ctx context.Context) error {
		cmd := &PhotosPickerDeleteCmd{SessionID: "session-1"}
		return cmd.Run(ctx, flags)
	})
	if deleteResult.err != nil {
		t.Fatalf("delete: %v\nstderr=%q", deleteResult.err, deleteResult.stderr)
	}
	if !strings.Contains(deleteResult.stdout, `"deleted": true`) {
		t.Fatalf("delete output = %s", deleteResult.stdout)
	}
}

func TestPhotosPickerWaitUsesAPITiming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"session-1",
			"pollingConfig":{"pollInterval":"1s","timeoutIn":"2s"}
		}`)
	}))
	defer srv.Close()
	client := googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL))

	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	waitCalls := 0
	_, err := waitForPhotosPickerSession(
		context.Background(),
		client,
		"session-1",
		0,
		photosPickerWaitRuntime{
			now: func() time.Time { return now },
			wait: func(_ context.Context, duration time.Duration) error {
				waitCalls++
				now = now.Add(duration)
				return nil
			},
		},
	)
	if !errors.Is(err, errPhotosPickerWaitTimeout) {
		t.Fatalf("err = %v", err)
	}
	if waitCalls != 2 {
		t.Fatalf("wait calls = %d, want 2", waitCalls)
	}
}

func TestPhotosPickerWaitHonorsAPIStopSignalWithLocalTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"session-1",
			"pollingConfig":{"pollInterval":"1s","timeoutIn":"0s"}
		}`)
	}))
	defer srv.Close()
	client := googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL))

	waitCalls := 0
	_, err := waitForPhotosPickerSession(
		context.Background(),
		client,
		"session-1",
		time.Minute,
		photosPickerWaitRuntime{
			now: time.Now,
			wait: func(context.Context, time.Duration) error {
				waitCalls++
				return nil
			},
		},
	)
	if !errors.Is(err, errPhotosPickerWaitTimeout) {
		t.Fatalf("err = %v", err)
	}
	if waitCalls != 0 {
		t.Fatalf("wait calls = %d, want 0", waitCalls)
	}
}

func TestPhotosPickerListRejectsRepeatedPageToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"nextPageToken":"repeated"}`)
	}))
	defer srv.Close()

	client := googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL))

	cmd := &PhotosPickerListCmd{SessionID: "session-1", Max: 50, All: true}
	result := runWithPhotosTestServices(t, photosTestServices{
		PhotosPicker: fixedPhotosPickerTestService(client),
	}, func(ctx context.Context) error {
		return cmd.Run(ctx, &RootFlags{Account: "a@example.com"})
	})
	if result.err == nil || !strings.Contains(result.err.Error(), "repeated page token") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestPhotosPickerGetNotFoundExitCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":404,"status":"NOT_FOUND","message":"session missing"}}`)
	}))
	defer srv.Close()

	client := googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL))
	result := runWithPhotosTestServices(t, photosTestServices{
		PhotosPicker: fixedPhotosPickerTestService(client),
	}, func(ctx context.Context) error {
		return (&PhotosPickerGetCmd{SessionID: "deleted-session"}).Run(
			ctx,
			&RootFlags{Account: "a@example.com"},
		)
	})

	if got := ExitCode(stableExitCode(result.err)); got != exitCodeNotFound {
		t.Fatalf("exit code = %d, want %d (err=%v)", got, exitCodeNotFound, result.err)
	}
	if !strings.Contains(result.err.Error(), "404 NOT_FOUND") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestPhotosPickerValidationFailsBeforeClient(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative max items", args: []string{"--account", "a@b.com", "photos", "picker", "create", "--max-items=-1"}, want: "--max-items must be non-negative"},
		{name: "too many items", args: []string{"--account", "a@b.com", "photos", "picker", "create", "--max-items", "2001"}, want: "--max-items must be <= 2000"},
		{name: "empty get session", args: []string{"--account", "a@b.com", "photos", "picker", "get", ""}, want: "empty sessionId"},
		{name: "negative wait", args: []string{"--account", "a@b.com", "photos", "picker", "wait", "session-1", "--timeout=-1s"}, want: "--timeout must be non-negative"},
		{name: "zero list max", args: []string{"--account", "a@b.com", "photos", "picker", "list", "session-1", "--max", "0"}, want: "max must be > 0"},
		{name: "large list max", args: []string{"--account", "a@b.com", "photos", "picker", "list", "session-1", "--max", "101"}, want: "max must be <= 100"},
		{name: "empty media id", args: []string{"--account", "a@b.com", "photos", "picker", "download", "session-1", ""}, want: "empty mediaItemId"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := executeWithPhotosTestServices(t, tc.args, photosTestServices{
				PhotosPicker: unexpectedPhotosPickerTestService(t, "expected validation before creating Picker client"),
			})
			if result.err == nil || ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), tc.want) {
				t.Fatalf("err = %v", result.err)
			}
		})
	}
}

func TestPhotosPickerCreateDryRunSkipsAuth(t *testing.T) {
	result := runWithPhotosTestServices(t, photosTestServices{
		PhotosPicker: unexpectedPhotosPickerTestService(t, "dry-run should not create Picker client"),
	}, func(ctx context.Context) error {
		cmd := &PhotosPickerCreateCmd{MaxItems: 4, Open: true}
		return cmd.Run(ctx, &RootFlags{DryRun: true})
	})
	if ExitCode(result.err) != 0 {
		t.Fatalf("dry-run: %v", result.err)
	}
	if !strings.Contains(result.stdout, `"op": "photos.picker.sessions.create"`) ||
		!strings.Contains(result.stdout, `"max_items": 4`) {
		t.Fatalf("output = %s", result.stdout)
	}
}
