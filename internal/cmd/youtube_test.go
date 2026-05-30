package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	youtube "google.golang.org/api/youtube/v3"

	"github.com/steipete/gogcli/internal/secrets"
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

func TestYouTubeVideosListWithAccountUsesOAuthService(t *testing.T) {
	origOAuth := newYouTubeForAccount
	origAPIKey := newYouTubeWithAPIKey
	t.Cleanup(func() {
		newYouTubeForAccount = origOAuth
		newYouTubeWithAPIKey = origAPIKey
	})

	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != "dQw4w9WgXcQ" {
			t.Fatalf("id = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeForAccount = func(_ context.Context, account string) (*youtube.Service, error) {
		gotAccount = account
		return svc, nil
	}
	newYouTubeWithAPIKey = func(context.Context, string) (*youtube.Service, error) {
		t.Fatal("API key service should not be used when account is configured")
		return nil, errors.New("unexpected API key service")
	}

	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "dQw4w9WgXcQ", "--max", "1"}, newQuietUIContext(t), &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeCommentsListWithAccountUsesOAuthService(t *testing.T) {
	origOAuth := newYouTubeCommentsForAccount
	origAPIKey := newYouTubeWithAPIKey
	t.Cleanup(func() {
		newYouTubeCommentsForAccount = origOAuth
		newYouTubeWithAPIKey = origAPIKey
	})

	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/commentThreads" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("videoId"); got != "dQw4w9WgXcQ" {
			t.Fatalf("videoId = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeCommentsForAccount = func(_ context.Context, account string) (*youtube.Service, error) {
		gotAccount = account
		return svc, nil
	}
	newYouTubeWithAPIKey = func(context.Context, string) (*youtube.Service, error) {
		t.Fatal("API key service should not be used when account is configured")
		return nil, errors.New("unexpected API key service")
	}

	err := runKong(t, &YouTubeCommentsListCmd{}, []string{"--video-id", "dQw4w9WgXcQ", "--max", "1"}, newQuietUIContext(t), &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeVideosListWithAutoAccountUsesOAuthService(t *testing.T) {
	origOAuth := newYouTubeForAccount
	origAPIKey := newYouTubeWithAPIKey
	origStore := openSecretsStoreForAccount
	t.Cleanup(func() {
		newYouTubeForAccount = origOAuth
		newYouTubeWithAPIKey = origAPIKey
		openSecretsStoreForAccount = origStore
	})
	openSecretsStoreForAccount = func() (secrets.Store, error) {
		return &fakeSecretsStore{defaultAccount: "default@example.com"}, nil
	}

	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	newYouTubeForAccount = func(_ context.Context, account string) (*youtube.Service, error) {
		gotAccount = account
		return svc, nil
	}
	newYouTubeWithAPIKey = func(context.Context, string) (*youtube.Service, error) {
		t.Fatal("API key service should not be used when --account auto is configured")
		return nil, errors.New("unexpected API key service")
	}

	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "dQw4w9WgXcQ", "--max", "1"}, newQuietUIContext(t), &RootFlags{Account: "auto"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "default@example.com" {
		t.Fatalf("account = %q", gotAccount)
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
