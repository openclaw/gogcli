package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	youtube "google.golang.org/api/youtube/v3"
)

func TestYouTubeChannelsListWithAPIKey(t *testing.T) {
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")
	origNew := newYouTubeWithAPIKey
	t.Cleanup(func() { newYouTubeWithAPIKey = origNew })

	var gotKey string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/youtube/v3/channels" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": "UC123",
					"snippet": map[string]any{
						"title":       "Test Channel",
						"publishedAt": "2026-01-02T03:04:05Z",
					},
					"statistics": map[string]any{
						"subscriberCount": "7",
						"videoCount":      "3",
						"viewCount":       "99",
					},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeWithAPIKey = func(_ context.Context, key string) (*youtube.Service, error) {
		gotKey = key
		return svc, nil
	}

	var err error
	out := captureStdout(t, func() {
		ctx := newCmdOutputContext(t, &bytes.Buffer{}, &bytes.Buffer{})
		err = runKong(t, &YouTubeChannelsListCmd{}, []string{"--id", " UC123 , ", "--max", "1"}, ctx, &RootFlags{})
	})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotKey != "test-key" {
		t.Fatalf("API key = %q", gotKey)
	}
	if !strings.Contains(gotQuery, "id=UC123") || !strings.Contains(gotQuery, "maxResults=1") {
		t.Fatalf("query = %s", gotQuery)
	}
	if !strings.Contains(out, "UC123") || !strings.Contains(out, "Test Channel") {
		t.Fatalf("stdout = %q", out)
	}
	if strings.Contains(out, "youtube.ChannelListResponse") {
		t.Fatalf("stdout leaked Go struct dump: %q", out)
	}
}

func TestYouTubeMineUsesOAuthService(t *testing.T) {
	origNew := newYouTubeForAccount
	t.Cleanup(func() { newYouTubeForAccount = origNew })

	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/activities" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("mine"); got != "true" {
			t.Fatalf("mine = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeForAccount = func(_ context.Context, account string) (*youtube.Service, error) {
		gotAccount = account
		return svc, nil
	}

	err := runKong(t, &YouTubeActivitiesListCmd{}, []string{"--mine", "--max", "1"}, newQuietUIContext(t), &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeSearchWithAPIKey(t *testing.T) {
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")
	origNew := newYouTubeWithAPIKey
	t.Cleanup(func() { newYouTubeWithAPIKey = origNew })

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/youtube/v3/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": map[string]any{
						"kind":    "youtube#video",
						"videoId": "abc123",
					},
					"snippet": map[string]any{
						"title":        "Test Video",
						"channelTitle": "Test Channel",
						"publishedAt":  "2026-01-02T03:04:05Z",
					},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeWithAPIKey = func(_ context.Context, key string) (*youtube.Service, error) {
		return svc, nil
	}

	var err error
	out := captureStdout(t, func() {
		ctx := newCmdOutputContext(t, &bytes.Buffer{}, &bytes.Buffer{})
		err = runKong(t, &YouTubeSearchListCmd{}, []string{"golang tutorials", "--max", "5"}, ctx, &RootFlags{})
	})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !strings.Contains(gotQuery, "q=golang+tutorials") {
		t.Fatalf("query = %s", gotQuery)
	}
	if !strings.Contains(out, "abc123") || !strings.Contains(out, "Test Video") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestYouTubeSearchWithOAuth(t *testing.T) {
	origNew := newYouTubeForAccount
	t.Cleanup(func() { newYouTubeForAccount = origNew })

	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "test query" {
			t.Fatalf("q = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeForAccount = func(_ context.Context, account string) (*youtube.Service, error) {
		gotAccount = account
		return svc, nil
	}

	err := runKong(t, &YouTubeSearchListCmd{}, []string{"test query", "--max", "1"}, newQuietUIContext(t), &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeSearchTypeValidation(t *testing.T) {
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")
	origNew := newYouTubeWithAPIKey
	t.Cleanup(func() { newYouTubeWithAPIKey = origNew })
	newYouTubeWithAPIKey = func(_ context.Context, key string) (*youtube.Service, error) {
		t.Fatal("should not reach API call with invalid type")
		return nil, nil
	}

	err := runKong(t, &YouTubeSearchListCmd{}, []string{"query", "--type", "invalid"}, newQuietUIContext(t), &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--type must be video, channel, or playlist") {
		t.Fatalf("expected type validation, got %v", err)
	}
}

func TestYouTubeValidation(t *testing.T) {
	err := runKong(t, &YouTubeChannelsListCmd{}, []string{"--id", "UC123", "--max", "51"}, newQuietUIContext(t), &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--max must be between 1 and 50") {
		t.Fatalf("expected max validation, got %v", err)
	}

	err = runKong(t, &YouTubeActivitiesListCmd{}, []string{"--channel-id", "UC123", "--mine"}, newQuietUIContext(t), &RootFlags{Account: "me@example.com"})
	if err == nil || !strings.Contains(err.Error(), "either --channel-id or --mine") {
		t.Fatalf("expected mutually exclusive validation, got %v", err)
	}
}
