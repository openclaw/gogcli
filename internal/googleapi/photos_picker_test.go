package googleapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPhotosPickerSessionLifecycle(t *testing.T) {
	var createCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sessions":
			createCalls.Add(1)

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}

			config := body["pickingConfig"].(map[string]any)
			if config["maxItemCount"] != "12" {
				t.Fatalf("create body = %#v", body)
			}
			_, _ = io.WriteString(w, `{
				"id":"session-1",
				"pickerUri":"https://photos.google.com/picker/session-1",
				"pollingConfig":{"pollInterval":"1s","timeoutIn":"60s"},
				"pickingConfig":{"maxItemCount":"12"}
			}`)
		case r.Method == http.MethodGet && r.URL.Path == "/sessions/session-1":
			_, _ = io.WriteString(w, `{"id":"session-1","mediaItemsSet":true}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/sessions/session-1":
			_, _ = io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewPhotosPickerClient(srv.Client(), WithPhotosPickerBaseURL(srv.URL))

	session, err := client.CreateSession(context.Background(), 12)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if session.ID != "session-1" || session.PickingConfig == nil || session.PickingConfig.MaxItemCount != 12 {
		t.Fatalf("session = %#v", session)
	}

	got, err := client.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if !got.MediaItemsSet {
		t.Fatalf("session not ready: %#v", got)
	}

	if err := client.DeleteSession(context.Background(), session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if createCalls.Load() != 1 {
		t.Fatalf("create calls = %d", createCalls.Load())
	}
}

func TestPhotosPickerFindMediaItemPaginates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/mediaItems" {
			http.NotFound(w, r)
			return
		}

		if r.URL.Query().Get("sessionId") != "session-1" || r.URL.Query().Get("pageSize") != "100" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("pageToken") == "" {
			_, _ = io.WriteString(w, `{"mediaItems":[{"id":"other"}],"nextPageToken":"page-2"}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"mediaItems":[{
				"id":"wanted",
				"type":"PHOTO",
				"mediaFile":{"filename":"photo.jpg","baseUrl":"https://example.test/photo"}
			}]
		}`)
	}))
	defer srv.Close()

	client := NewPhotosPickerClient(srv.Client(), WithPhotosPickerBaseURL(srv.URL))

	item, err := client.FindMediaItem(context.Background(), "session-1", "wanted")
	if err != nil {
		t.Fatalf("FindMediaItem: %v", err)
	}

	if item.ID != "wanted" || item.MediaFile == nil || item.MediaFile.Filename != "photo.jpg" {
		t.Fatalf("item = %#v", item)
	}
}

func TestPhotosPickerDownloadMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/photo=d" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, "photo-bytes")
	}))
	defer srv.Close()

	client := NewPhotosPickerClient(srv.Client())

	resp, err := client.DownloadMedia(context.Background(), &PhotosPickerMediaItem{
		ID:   "photo-1",
		Type: "PHOTO",
		MediaFile: &PhotosPickerMediaFile{
			BaseURL: srv.URL + "/photo",
		},
	})
	if err != nil {
		t.Fatalf("DownloadMedia: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if string(body) != "photo-bytes" {
		t.Fatalf("body = %q", body)
	}
}

func TestPhotosPickerDownloadRejectsUnreadyVideo(t *testing.T) {
	client := NewPhotosPickerClient(nil)

	resp, err := client.DownloadMedia(context.Background(), &PhotosPickerMediaItem{
		ID:   "video-1",
		Type: "VIDEO",
		MediaFile: &PhotosPickerMediaFile{
			BaseURL: "https://example.test/video",
			MediaFileMetadata: &PhotosPickerMediaFileMetadata{
				VideoMetadata: &PhotosPickerVideoMetadata{ProcessingStatus: "PROCESSING"},
			},
		},
	})
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("err = %v", err)
	}
}

func TestPhotosPickerAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusFailedDependency)
		_, _ = io.WriteString(w, `{"error":{"code":424,"status":"FAILED_PRECONDITION","message":"finish picking first"}}`)
	}))
	defer srv.Close()

	client := NewPhotosPickerClient(srv.Client(), WithPhotosPickerBaseURL(srv.URL))

	_, err := client.ListMediaItems(context.Background(), "session-1", 50, "")
	if err == nil || !strings.Contains(err.Error(), "FAILED_PRECONDITION") {
		t.Fatalf("err = %v", err)
	}
}
