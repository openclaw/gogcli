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

	originalClient := newPhotosPickerClient
	originalOpen := openPhotosPickerBrowser
	t.Cleanup(func() {
		newPhotosPickerClient = originalClient
		openPhotosPickerBrowser = originalOpen
	})
	newPhotosPickerClient = func(context.Context, string) (*googleapi.PhotosPickerClient, error) {
		return googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL)), nil
	}
	openedURI := ""
	openPhotosPickerBrowser = func(uri string) error {
		openedURI = uri
		return nil
	}
	ctx := newCalendarJSONContext(t)
	flags := &RootFlags{Account: "a@example.com"}

	createOut := captureStdout(t, func() {
		cmd := &PhotosPickerCreateCmd{MaxItems: 2, Open: true}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}
	})
	if !strings.Contains(createOut, `"id": "session-1"`) {
		t.Fatalf("create output = %s", createOut)
	}
	if openedURI != "https://photos.google.com/picker/session-1" {
		t.Fatalf("opened URI = %q", openedURI)
	}

	waitOut := captureStdout(t, func() {
		cmd := &PhotosPickerWaitCmd{SessionID: "session-1", Timeout: time.Second}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("wait: %v", err)
		}
	})
	if !strings.Contains(waitOut, `"mediaItemsSet": true`) {
		t.Fatalf("wait output = %s", waitOut)
	}

	listOut := captureStdout(t, func() {
		cmd := &PhotosPickerListCmd{SessionID: "session-1", Max: 50}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	if !strings.Contains(listOut, `"id": "photo-1"`) {
		t.Fatalf("list output = %s", listOut)
	}

	outputPath := filepath.Join(t.TempDir(), "picked.jpg")
	downloadOut := captureStdout(t, func() {
		cmd := &PhotosPickerDownloadCmd{
			SessionID:   "session-1",
			MediaItemID: "photo-1",
			Out:         outputPath,
		}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("download: %v", err)
		}
	})
	if !strings.Contains(downloadOut, `"bytes": 12`) {
		t.Fatalf("download output = %s", downloadOut)
	}
	downloaded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if string(downloaded) != "picked-photo" {
		t.Fatalf("downloaded = %q", downloaded)
	}

	deleteOut := captureStdout(t, func() {
		cmd := &PhotosPickerDeleteCmd{SessionID: "session-1"}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})
	if !strings.Contains(deleteOut, `"deleted": true`) {
		t.Fatalf("delete output = %s", deleteOut)
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

	originalClient := newPhotosPickerClient
	t.Cleanup(func() { newPhotosPickerClient = originalClient })
	newPhotosPickerClient = func(context.Context, string) (*googleapi.PhotosPickerClient, error) {
		return googleapi.NewPhotosPickerClient(srv.Client(), googleapi.WithPhotosPickerBaseURL(srv.URL)), nil
	}

	cmd := &PhotosPickerListCmd{SessionID: "session-1", Max: 50, All: true}
	err := cmd.Run(newCalendarJSONContext(t), &RootFlags{Account: "a@example.com"})
	if err == nil || !strings.Contains(err.Error(), "repeated page token") {
		t.Fatalf("err = %v", err)
	}
}

func TestPhotosPickerValidationFailsBeforeClient(t *testing.T) {
	originalClient := newPhotosPickerClient
	t.Cleanup(func() { newPhotosPickerClient = originalClient })
	newPhotosPickerClient = func(context.Context, string) (*googleapi.PhotosPickerClient, error) {
		t.Fatalf("expected validation before creating Picker client")
		return nil, context.Canceled
	}

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
			_ = captureStderr(t, func() {
				err := Execute(tc.args)
				if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("err = %v", err)
				}
			})
		})
	}
}

func TestPhotosPickerCreateDryRunSkipsAuth(t *testing.T) {
	originalClient := newPhotosPickerClient
	t.Cleanup(func() { newPhotosPickerClient = originalClient })
	newPhotosPickerClient = func(context.Context, string) (*googleapi.PhotosPickerClient, error) {
		t.Fatalf("dry-run should not create Picker client")
		return nil, context.Canceled
	}

	output := captureStdout(t, func() {
		cmd := &PhotosPickerCreateCmd{MaxItems: 4, Open: true}
		if err := cmd.Run(newCalendarJSONContext(t), &RootFlags{DryRun: true}); ExitCode(err) != 0 {
			t.Fatalf("dry-run: %v", err)
		}
	})
	if !strings.Contains(output, `"op": "photos.picker.sessions.create"`) ||
		!strings.Contains(output, `"max_items": 4`) {
		t.Fatalf("output = %s", output)
	}
}
