package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	youtube "google.golang.org/api/youtube/v3"
)

func TestYouTubeChannelsListWithAPIKey(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")

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

	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		APIKey: func(_ context.Context, key string) (*youtube.Service, error) {
			gotKey = key
			return svc, nil
		},
	})
	err := runKong(t, &YouTubeChannelsListCmd{}, []string{"--id", " UC123 , ", "--max", "1"}, ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	out := stdout.String()
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
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
	})
	err := runKong(t, &YouTubeActivitiesListCmd{}, []string{"--mine", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeChannelsMineJSONEmptyItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/channels" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("mine"); got != "true" {
			t.Fatalf("mine = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)

	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeChannelsListCmd{}, []string{"--mine", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com", JSON: true})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	var got struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output %q: %v", stdout.String(), err)
	}
	if got.Items == nil {
		t.Fatalf("items is nil in output: %s", stdout.String())
	}
	if len(got.Items) != 0 {
		t.Fatalf("items len = %d, output: %s", len(got.Items), stdout.String())
	}
}

func TestYouTubeSearchWithAPIKey(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")

	var gotKey string
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
						"kind":      "youtube#channel",
						"channelId": "UC123",
					},
					"snippet": map[string]any{
						"title":        "Test Channel",
						"channelTitle": "Test Channel",
						"publishedAt":  "2026-01-02T03:04:05Z",
					},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)

	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		APIKey: func(_ context.Context, key string) (*youtube.Service, error) {
			gotKey = key
			return svc, nil
		},
	})
	err := runKong(t, &YouTubeSearchListCmd{}, []string{"golang tutorials", "--type", "channel", "--order", "videoCount", "--max", "5"}, ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	out := stdout.String()
	if gotKey != "test-key" {
		t.Fatalf("API key = %q", gotKey)
	}
	if !strings.Contains(gotQuery, "q=golang+tutorials") {
		t.Fatalf("query = %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "type=channel") || !strings.Contains(gotQuery, "order=videoCount") {
		t.Fatalf("query = %s", gotQuery)
	}
	if !strings.Contains(out, "UC123") || !strings.Contains(out, "Test Channel") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestYouTubeSearchFiltersUnexpectedKinds(t *testing.T) {
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
						"kind":      "youtube#channel",
						"channelId": "UC123",
					},
					"snippet": map[string]any{"title": "Unexpected Channel"},
				},
				{
					"id": map[string]any{
						"kind":    "youtube#video",
						"videoId": "vid123",
					},
					"snippet": map[string]any{"title": "Expected Video"},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)

	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSearchListCmd{}, []string{"test query", "--type", "video", "--max", "2"}, ctx, &RootFlags{Account: "me@example.com", JSON: true})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !strings.Contains(gotQuery, "type=video") {
		t.Fatalf("query = %s", gotQuery)
	}

	var got struct {
		Items []struct {
			ID struct {
				Kind      string `json:"kind"`
				VideoID   string `json:"videoId"`
				ChannelID string `json:"channelId"`
			} `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output %q: %v", stdout.String(), err)
	}
	if len(got.Items) != 1 || got.Items[0].ID.VideoID != "vid123" || got.Items[0].ID.ChannelID != "" {
		t.Fatalf("unexpected filtered output: %s", stdout.String())
	}
}

func TestYouTubeSearchWithOAuth(t *testing.T) {
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
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
		APIKey: unexpectedYouTubeTestService(t, "API key service should not be used when account is configured"),
	})
	err := runKong(t, &YouTubeSearchListCmd{}, []string{"test query", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeSearchWithAutoAccountUsesOAuthService(t *testing.T) {
	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "auto account query" {
			t.Fatalf("q = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
		APIKey: unexpectedYouTubeTestService(t, "API key service should not be used when --account auto is configured"),
	})
	flags := rootFlagsWithAuthStore(
		&RootFlags{Account: "auto"},
		&fakeSecretsStore{defaultAccount: "default@example.com"},
	)
	err := runKong(t, &YouTubeSearchListCmd{}, []string{"auto account query", "--max", "1"}, ctx, flags)
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "default@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeSearchTypeValidation(t *testing.T) {
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		APIKey: unexpectedYouTubeTestService(t, "should not reach API call with invalid type"),
	})
	err := runKong(t, &YouTubeSearchListCmd{}, []string{"query", "--type", "invalid"}, ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--type must be video, channel, or playlist") {
		t.Fatalf("expected type validation, got %v", err)
	}
}

func assertYouTubeReadWithAccount(
	t *testing.T,
	path string,
	queryKey string,
	queryValue string,
	services func(func(context.Context, string) (*youtube.Service, error)) youtubeTestServices,
	run func(context.Context) error,
) {
	t.Helper()
	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get(queryKey); got != queryValue {
			t.Fatalf("%s = %q", queryKey, got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	t.Cleanup(srv.Close)

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	factory := func(_ context.Context, account string) (*youtube.Service, error) {
		gotAccount = account
		return svc, nil
	}
	testServices := services(factory)
	testServices.APIKey = unexpectedYouTubeTestService(t, "API key service should not be used when account is configured")
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), testServices)
	if err := run(ctx); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeVideosListWithAccountUsesOAuthService(t *testing.T) {
	assertYouTubeReadWithAccount(
		t, "/youtube/v3/videos", "id", "dQw4w9WgXcQ",
		func(factory func(context.Context, string) (*youtube.Service, error)) youtubeTestServices {
			return youtubeTestServices{Account: factory}
		},
		func(ctx context.Context) error {
			return runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "dQw4w9WgXcQ", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
		},
	)
}

func TestYouTubeChannelReadCommandsWithAccountUseOAuthService(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *RootFlags) error
		path string
		key  string
		want string
	}{
		{
			name: "channels by id",
			run: func(ctx context.Context, flags *RootFlags) error {
				return runKong(t, &YouTubeChannelsListCmd{}, []string{"--id", "UC123", "--max", "1"}, ctx, flags)
			},
			path: "/youtube/v3/channels",
			key:  "id",
			want: "UC123",
		},
		{
			name: "playlists by channel",
			run: func(ctx context.Context, flags *RootFlags) error {
				return runKong(t, &YouTubePlaylistsListCmd{}, []string{"--channel-id", "UC123", "--max", "1"}, ctx, flags)
			},
			path: "/youtube/v3/playlists",
			key:  "channelId",
			want: "UC123",
		},
		{
			name: "activities by channel",
			run: func(ctx context.Context, flags *RootFlags) error {
				return runKong(t, &YouTubeActivitiesListCmd{}, []string{"--channel-id", "UC123", "--max", "1"}, ctx, flags)
			},
			path: "/youtube/v3/activities",
			key:  "channelId",
			want: "UC123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAccount string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %s", r.URL.Path)
				}
				if got := r.URL.Query().Get(tt.key); got != tt.want {
					t.Fatalf("%s = %q", tt.key, got)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
			}))
			defer srv.Close()

			svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
			ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
				Account: func(_ context.Context, account string) (*youtube.Service, error) {
					gotAccount = account
					return svc, nil
				},
				APIKey: unexpectedYouTubeTestService(t, "API key service should not be used when account is configured"),
			})
			if err := tt.run(ctx, &RootFlags{Account: "me@example.com"}); err != nil {
				t.Fatalf("runKong: %v", err)
			}
			if gotAccount != "me@example.com" {
				t.Fatalf("account = %q", gotAccount)
			}
		})
	}
}

func TestYouTubeCommentsListWithAccountUsesOAuthService(t *testing.T) {
	assertYouTubeReadWithAccount(
		t, "/youtube/v3/commentThreads", "videoId", "dQw4w9WgXcQ",
		func(factory func(context.Context, string) (*youtube.Service, error)) youtubeTestServices {
			return youtubeTestServices{Comments: factory}
		},
		func(ctx context.Context) error {
			return runKong(t, &YouTubeCommentsListCmd{}, []string{"--video-id", "dQw4w9WgXcQ", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
		},
	)
}

func TestYouTubeVideosListWithAutoAccountUsesOAuthService(t *testing.T) {
	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
		APIKey: unexpectedYouTubeTestService(t, "API key service should not be used when --account auto is configured"),
	})
	flags := rootFlagsWithAuthStore(
		&RootFlags{Account: "auto"},
		&fakeSecretsStore{defaultAccount: "default@example.com"},
	)
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "dQw4w9WgXcQ", "--max", "1"}, ctx, flags)
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

func TestYouTubeSubscriptionsListMine(t *testing.T) {
	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/subscriptions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("mine"); got != "true" {
			t.Fatalf("mine = %q", got)
		}
		if got := r.URL.Query().Get("maxResults"); got != "2" {
			t.Fatalf("maxResults = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": "SUB123",
					"snippet": map[string]any{
						"title":       "Cool Channel",
						"publishedAt": "2025-01-01T00:00:00Z",
						"resourceId": map[string]any{
							"kind":      "youtube#channel",
							"channelId": "UCcool",
						},
					},
				},
			},
			"nextPageToken": "tok1",
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout, stderr bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, &stdout, &stderr), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
	})
	err := runKong(t, &YouTubeSubscriptionsListCmd{}, []string{"--max", "2"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
	out := stdout.String()
	if !strings.Contains(out, "SUB123") || !strings.Contains(out, "UCcool") || !strings.Contains(out, "Cool Channel") {
		t.Fatalf("stdout = %q", out)
	}
	if !strings.Contains(stderr.String(), "tok1") {
		t.Fatalf("expected page token hint in stderr: %q", stderr.String())
	}
}

func TestYouTubeSubscriptionsSubscribe(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/subscriptions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "NEWSUB456",
			"snippet": map[string]any{
				"resourceId": map[string]any{
					"kind":      "youtube#channel",
					"channelId": "UCnew",
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Write: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSubscriptionsSubscribeCmd{}, []string{"--channel-id", "UCnew"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !strings.Contains(string(gotBody), "UCnew") {
		t.Fatalf("request body missing channel ID: %s", gotBody)
	}
	if !strings.Contains(stdout.String(), "NEWSUB456") {
		t.Fatalf("stdout missing subscription ID: %q", stdout.String())
	}
}

func TestYouTubeSubscriptionsUnsubscribeByID(t *testing.T) {
	var deletedID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/subscriptions" && r.Method == http.MethodDelete {
			deletedID = r.URL.Query().Get("id")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSubscriptionsUnsubscribeCmd{}, []string{"--id", "SUB123"}, ctx, &RootFlags{Account: "me@example.com", Force: true})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if deletedID != "SUB123" {
		t.Fatalf("deleted ID = %q", deletedID)
	}
}

func TestYouTubeSubscriptionsUnsubscribeByChannelID(t *testing.T) {
	var deletedID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/subscriptions" && r.Method == http.MethodGet {
			if got := r.URL.Query().Get("forChannelId"); got != "UCcool" {
				t.Fatalf("forChannelId = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "SUB999"}},
			})
			return
		}
		if r.URL.Path == "/youtube/v3/subscriptions" && r.Method == http.MethodDelete {
			deletedID = r.URL.Query().Get("id")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSubscriptionsUnsubscribeCmd{}, []string{"--channel-id", "UCcool"}, ctx, &RootFlags{Account: "me@example.com", Force: true})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if deletedID != "SUB999" {
		t.Fatalf("deleted ID = %q", deletedID)
	}
}

func TestYouTubeSubscriptionsUnsubscribeChannelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube/v3/subscriptions" && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSubscriptionsUnsubscribeCmd{}, []string{"--channel-id", "UCmissing"}, ctx, &RootFlags{Account: "me@example.com", Force: true})
	if err == nil || !strings.Contains(err.Error(), "not subscribed") {
		t.Fatalf("expected not-subscribed error, got %v", err)
	}
}

func TestYouTubeSubscriptionsUnsubscribeValidation(t *testing.T) {
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write: unexpectedYouTubeTestService(t, "should not reach service with missing args"),
	})
	flags := &RootFlags{Account: "me@example.com", Force: true}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "neither id nor channel-id",
			args: []string{},
			want: "set --id or --channel-id",
		},
		{
			name: "both id and channel-id",
			args: []string{"--id", "SUB1", "--channel-id", "UC1"},
			want: "use either --id or --channel-id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runKong(t, &YouTubeSubscriptionsUnsubscribeCmd{}, tt.args, ctx, flags)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestYouTubeVideosListMyRatingUsesOAuthService(t *testing.T) {
	var gotAccount string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": "vidLiked",
					"snippet": map[string]any{
						"title":        "Liked Video",
						"channelTitle": "Some Channel",
						"publishedAt":  "2026-01-02T03:04:05Z",
					},
					"statistics": map[string]any{"viewCount": "42"},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
		APIKey: unexpectedYouTubeTestService(t, "API key service should not be used for --my-rating"),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--my-rating", "like", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
	if !strings.Contains(gotQuery, "myRating=like") {
		t.Fatalf("query = %s", gotQuery)
	}
	out := stdout.String()
	if !strings.Contains(out, "vidLiked") || !strings.Contains(out, "Liked Video") {
		t.Fatalf("stdout = %q", out)
	}
}

// youtubePartValues collects the videos.list "part" selector from a request.
// The Google SDK may send part either comma-joined or as repeated query params,
// so normalize both into a flat slice.
func youtubePartValues(r *http.Request) []string {
	values := r.URL.Query()["part"]
	out := make([]string, 0, len(values))
	for _, raw := range values {
		out = append(out, strings.Split(raw, ",")...)
	}
	return out
}

func TestYouTubeVideosListPreservesDefaultParts(t *testing.T) {
	var gotParts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParts = youtubePartValues(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "vid1", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	want := []string{"snippet", "contentDetails", "statistics"}
	if strings.Join(gotParts, ",") != strings.Join(want, ",") {
		t.Fatalf("parts = %v, want %v", gotParts, want)
	}
}

func TestYouTubeVideosListRequestsAllNonOwnerParts(t *testing.T) {
	var gotParts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotParts = youtubePartValues(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
		APIKey:  unexpectedYouTubeTestService(t, "API key service should not be used when account is configured"),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "vid1", "--parts", "all", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	wantParts := []string{
		"contentDetails", "id", "liveStreamingDetails", "localizations",
		"paidProductPlacementDetails", "player", "recordingDetails", "snippet",
		"statistics", "status", "topicDetails",
	}
	if strings.Join(gotParts, ",") != strings.Join(wantParts, ",") {
		t.Fatalf("parts = %v, want %v", gotParts, wantParts)
	}
	gotSet := make(map[string]bool, len(gotParts))
	for _, part := range gotParts {
		gotSet[part] = true
	}
	for _, owner := range []string{"fileDetails", "processingDetails", "suggestions"} {
		if gotSet[owner] {
			t.Fatalf("part list %v must not request owner-only part %q", gotParts, owner)
		}
	}
}

func TestYouTubeVideosListPartsOverride(t *testing.T) {
	var gotParts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotParts = youtubePartValues(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "vid1", "--parts", "snippet, fileDetails", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if len(gotParts) != 2 || gotParts[0] != "snippet" || gotParts[1] != "fileDetails" {
		t.Fatalf("parts = %v, want [snippet fileDetails]", gotParts)
	}
}

func TestYouTubeVideosListRejectsMixedAllParts(t *testing.T) {
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: unexpectedYouTubeTestService(t, "invalid --parts must not create a service"),
		APIKey:  unexpectedYouTubeTestService(t, "invalid --parts must not create a service"),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "vid1", "--parts", "all,status", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com"})
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--parts all cannot be combined with explicit parts") {
		t.Fatalf("expected mixed all usage error, got %v", err)
	}
}

func TestYouTubeVideosListJSONSerializesNonCoreParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/videos" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": "vidRich",
					"snippet": map[string]any{
						"title":       "Rich Video",
						"publishedAt": "2026-01-02T03:04:05Z",
						"thumbnails": map[string]any{
							"default": map[string]any{"url": "https://img/d.jpg", "width": 120, "height": 90},
							"high":    map[string]any{"url": "https://img/h.jpg", "width": 480, "height": 360},
							"maxres":  map[string]any{"url": "https://img/m.jpg", "width": 1280, "height": 720},
						},
					},
					"status": map[string]any{
						"privacyStatus": "public",
						"uploadStatus":  "processed",
						"madeForKids":   false,
					},
					"topicDetails": map[string]any{
						"topicCategories": []string{"https://en.wikipedia.org/wiki/Music"},
					},
					"liveStreamingDetails": map[string]any{
						"actualStartTime": "2026-01-01T00:00:00Z",
					},
					"paidProductPlacementDetails": map[string]any{
						"hasPaidProductPlacement": true,
					},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "vidRich", "--parts", "all", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com", JSON: true})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	var got struct {
		Items []struct {
			ID     string `json:"id"`
			Status struct {
				PrivacyStatus string `json:"privacyStatus"`
				UploadStatus  string `json:"uploadStatus"`
			} `json:"status"`
			TopicDetails struct {
				TopicCategories []string `json:"topicCategories"`
			} `json:"topicDetails"`
			Snippet struct {
				Thumbnails map[string]struct {
					URL string `json:"url"`
				} `json:"thumbnails"`
			} `json:"snippet"`
			LiveStreamingDetails struct {
				ActualStartTime string `json:"actualStartTime"`
			} `json:"liveStreamingDetails"`
			PaidProductPlacementDetails struct {
				HasPaidProductPlacement bool `json:"hasPaidProductPlacement"`
			} `json:"paidProductPlacementDetails"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output %q: %v", stdout.String(), err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items len = %d: %s", len(got.Items), stdout.String())
	}
	item := got.Items[0]
	if item.Status.PrivacyStatus != "public" {
		t.Fatalf("status.privacyStatus = %q (non-core status part dropped): %s", item.Status.PrivacyStatus, stdout.String())
	}
	if len(item.TopicDetails.TopicCategories) != 1 {
		t.Fatalf("topicDetails.topicCategories = %v (non-core topicDetails part dropped): %s", item.TopicDetails.TopicCategories, stdout.String())
	}
	for _, size := range []string{"default", "high", "maxres"} {
		if item.Snippet.Thumbnails[size].URL == "" {
			t.Fatalf("thumbnail size %q missing from JSON (compacted): %s", size, stdout.String())
		}
	}
	if item.LiveStreamingDetails.ActualStartTime == "" {
		t.Fatalf("liveStreamingDetails.actualStartTime dropped: %s", stdout.String())
	}
	if !item.PaidProductPlacementDetails.HasPaidProductPlacement {
		t.Fatalf("paidProductPlacementDetails.hasPaidProductPlacement dropped: %s", stdout.String())
	}
}

// A video with no liveStreamingDetails (a normal non-live video) must still
// serialize cleanly — the SDK omits parts with no data, never errors.
func TestYouTubeVideosListToleratesPartialParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":      "vidPlain",
					"snippet": map[string]any{"title": "Plain Video"},
					"status":  map[string]any{"privacyStatus": "unlisted"},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeVideosListCmd{}, []string{"--id", "vidPlain", "--parts", "all", "--max", "1"}, ctx, &RootFlags{Account: "me@example.com", JSON: true})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	var got struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output %q: %v", stdout.String(), err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items len = %d: %s", len(got.Items), stdout.String())
	}
}

func TestYouTubeVideosListMyRatingValidation(t *testing.T) {
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: unexpectedYouTubeTestService(t, "should not reach service with invalid my-rating"),
		APIKey:  unexpectedYouTubeTestService(t, "should not reach service with invalid my-rating"),
	})
	flags := &RootFlags{Account: "me@example.com"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid value",
			args: []string{"--my-rating", "love", "--max", "1"},
			want: "--my-rating must be like or dislike",
		},
		{
			name: "combined with id",
			args: []string{"--id", "vid1", "--my-rating", "like", "--max", "1"},
			want: "use only one of --id, --chart, or --my-rating",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runKong(t, &YouTubeVideosListCmd{}, tt.args, ctx, flags)
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected usage error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestYouTubePlaylistsItemsListWithAPIKey(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/playlistItems" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id": "PLITEM1",
					"snippet": map[string]any{
						"title":                  "First Video",
						"position":               0,
						"videoOwnerChannelTitle": "Uploader Channel",
						"publishedAt":            "2026-01-02T03:04:05Z",
						"resourceId": map[string]any{
							"kind":    "youtube#video",
							"videoId": "vid111",
						},
					},
					"contentDetails": map[string]any{"videoId": "vid111"},
				},
			},
		})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		APIKey: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubePlaylistsItemsListCmd{}, []string{"--playlist-id", "PL123", "--max", "10"}, ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !strings.Contains(gotQuery, "playlistId=PL123") || !strings.Contains(gotQuery, "maxResults=10") {
		t.Fatalf("query = %s", gotQuery)
	}
	out := stdout.String()
	if !strings.Contains(out, "vid111") || !strings.Contains(out, "First Video") || !strings.Contains(out, "Uploader Channel") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestYouTubePlaylistsItemsListWithAccountUsesOAuthService(t *testing.T) {
	var gotAccount string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube/v3/playlistItems" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
		APIKey: unexpectedYouTubeTestService(t, "API key service should not be used when account is configured"),
	})
	err := runKong(t, &YouTubePlaylistsItemsListCmd{}, []string{"--playlist-id", "LL", "--max", "5"}, ctx, &RootFlags{Account: "me@example.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "me@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
	if !strings.Contains(gotQuery, "playlistId=LL") {
		t.Fatalf("query = %s", gotQuery)
	}
}

func TestYouTubePlaylistsItemsListLikedUsesDefaultAccount(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_YOUTUBE_API_KEY", "test-key")

	var gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: func(_ context.Context, account string) (*youtube.Service, error) {
			gotAccount = account
			return svc, nil
		},
		APIKey: unexpectedYouTubeTestService(t, "should not reach API key service for the liked playlist"),
	})
	flags := rootFlagsWithAuthStore(nil, &fakeSecretsStore{defaultAccount: "default@example.com"})
	if err := runKong(t, &YouTubePlaylistsItemsListCmd{}, []string{"--playlist-id", "LL", "--max", "5"}, ctx, flags); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotAccount != "default@example.com" {
		t.Fatalf("account = %q", gotAccount)
	}
}

func TestYouTubeValidationRejectsBlankSelectorsBeforeService(t *testing.T) {
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account:  unexpectedYouTubeTestService(t, "expected validation to fail before OAuth YouTube service creation"),
		Comments: unexpectedYouTubeTestService(t, "expected validation to fail before OAuth YouTube comments service creation"),
		Write:    unexpectedYouTubeTestService(t, "expected validation to fail before OAuth YouTube write service creation"),
		APIKey:   unexpectedYouTubeTestService(t, "expected validation to fail before API-key YouTube service creation"),
	})
	flags := &RootFlags{Account: "me@example.com"}
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "videos empty csv ids",
			run: func() error {
				return runKong(t, &YouTubeVideosListCmd{}, []string{"--id", ",", "--max", "1"}, ctx, flags)
			},
			want: "set --id VIDEO_IDS, --chart mostPopular, or --my-rating like",
		},
		{
			name: "playlist items blank id",
			run: func() error {
				return runKong(t, &YouTubePlaylistsItemsListCmd{}, []string{"--playlist-id", " ", "--max", "1"}, ctx, flags)
			},
			want: "set --playlist-id ID",
		},
		{
			name: "channels empty csv ids",
			run: func() error {
				return runKong(t, &YouTubeChannelsListCmd{}, []string{"--id", ",", "--max", "1"}, ctx, flags)
			},
			want: "set --id CHANNEL_IDS or --mine",
		},
		{
			name: "comments blank video",
			run: func() error {
				return runKong(t, &YouTubeCommentsListCmd{}, []string{"--video-id", " ", "--max", "1"}, ctx, flags)
			},
			want: "set --video-id ID or --channel-id ID",
		},
		{
			name: "activities blank channel",
			run: func() error {
				return runKong(t, &YouTubeActivitiesListCmd{}, []string{"--channel-id", " ", "--max", "1"}, ctx, flags)
			},
			want: "set --channel-id ID or --mine",
		},
		{
			name: "playlists blank channel",
			run: func() error {
				return runKong(t, &YouTubePlaylistsListCmd{}, []string{"--channel-id", " ", "--max", "1"}, ctx, flags)
			},
			want: "set --channel-id ID or --mine",
		},
		{
			name: "chart blank region",
			run: func() error {
				return runKong(t, &YouTubeVideosListCmd{}, []string{"--chart", "mostPopular", "--region", " ", "--max", "1"}, ctx, flags)
			},
			want: "--chart mostPopular requires --region",
		},
		{
			name: "search empty type csv",
			run: func() error {
				return runKong(t, &YouTubeSearchListCmd{}, []string{"query", "--type", ",", "--max", "1"}, ctx, flags)
			},
			want: "--type must be video, channel, or playlist",
		},
		{
			name: "search blank query",
			run: func() error {
				return runKong(t, &YouTubeSearchListCmd{}, []string{" ", "--max", "1"}, ctx, flags)
			},
			want: "search query is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected usage error containing %q, got %v", tt.want, err)
			}
		})
	}
}
