package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	youtube "google.golang.org/api/youtube/v3"
)

func TestYouTubeSubscriptionsListAllUsesPageSizeAndStartCursor(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("pageToken"))
		if got := r.URL.Query().Get("maxResults"); got != "2" {
			t.Fatalf("maxResults = %q", got)
		}
		switch r.URL.Query().Get("pageToken") {
		case "start":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items":         []map[string]any{{"id": "SUB1"}},
				"nextPageToken": "next",
			})
		case "next":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "SUB2"}},
			})
		default:
			t.Fatalf("unexpected page token: %q", r.URL.Query().Get("pageToken"))
		}
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSubscriptionsListCmd{}, []string{"--all", "--max", "2", "--page", "start"}, ctx, &RootFlags{
		Account: "me@example.com",
		JSON:    true,
	})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if strings.Join(pages, ",") != "start,next" {
		t.Fatalf("pages = %v", pages)
	}
	var got struct {
		Items         []json.RawMessage `json:"items"`
		NextPageToken string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output: %v\n%s", err, stdout.String())
	}
	if len(got.Items) != 2 || got.NextPageToken != "" {
		t.Fatalf("output = %s", stdout.String())
	}
}

func TestYouTubeSubscriptionsListAllEmitsEmptyArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
	})
	err := runKong(t, &YouTubeSubscriptionsListCmd{}, []string{"--all"}, ctx, &RootFlags{
		Account: "me@example.com",
		JSON:    true,
	})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}
	var got struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output: %v\n%s", err, stdout.String())
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("expected empty array, got %s", stdout.String())
	}
}

func TestYouTubeMutationDryRunsAreOffline(t *testing.T) {
	tests := []struct {
		name string
		cmd  interface {
			Run(context.Context, *RootFlags) error
		}
		args []string
		op   string
	}{
		{name: "subscribe", cmd: &YouTubeSubscriptionsSubscribeCmd{}, args: []string{"--channel-id", "UC1"}, op: "youtube.subscriptions.subscribe"},
		{name: "unsubscribe id", cmd: &YouTubeSubscriptionsUnsubscribeCmd{}, args: []string{"--id", "SUB1"}, op: "youtube.subscriptions.unsubscribe"},
		{name: "unsubscribe channel", cmd: &YouTubeSubscriptionsUnsubscribeCmd{}, args: []string{"--channel-id", "UC1"}, op: "youtube.subscriptions.unsubscribe"},
		{name: "playlist create", cmd: &YouTubePlaylistsCreateCmd{}, args: []string{"--title", "Test"}, op: "youtube.playlists.create"},
		{name: "playlist add", cmd: &YouTubePlaylistsAddCmd{}, args: []string{"--playlist-id", "PL1", "--video-id", "VID1"}, op: "youtube.playlists.add"},
		{name: "playlist remove item", cmd: &YouTubePlaylistsRemoveCmd{}, args: []string{"--item-id", "ITEM1"}, op: "youtube.playlists.remove"},
		{name: "playlist remove video", cmd: &YouTubePlaylistsRemoveCmd{}, args: []string{"--playlist-id", "PL1", "--video-id", "VID1"}, op: "youtube.playlists.remove"},
		{name: "playlist delete", cmd: &YouTubePlaylistsDeleteCmd{}, args: []string{"PL1"}, op: "youtube.playlists.delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
				Write: unexpectedYouTubeTestService(t, "dry-run must not create a YouTube service"),
			})
			err := runKong(t, tt.cmd, tt.args, ctx, &RootFlags{
				Account: "me@example.com",
				DryRun:  true,
				JSON:    true,
			})
			if ExitCode(err) != 0 {
				t.Fatalf("expected dry-run exit 0, got %v", err)
			}
			var got struct {
				DryRun bool   `json:"dry_run"`
				Op     string `json:"op"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("json output: %v\n%s", err, stdout.String())
			}
			if !got.DryRun || got.Op != tt.op {
				t.Fatalf("output = %s", stdout.String())
			}
		})
	}
}

func TestYouTubePlaylistCreateDefaultsPrivate(t *testing.T) {
	var stdout bytes.Buffer
	ctx := withYouTubeTestServices(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), youtubeTestServices{
		Write: unexpectedYouTubeTestService(t, "dry-run must not create a YouTube service"),
	})
	err := runKong(t, &YouTubePlaylistsCreateCmd{}, []string{"--title", "Test"}, ctx, &RootFlags{
		Account: "me@example.com",
		DryRun:  true,
		JSON:    true,
	})
	if ExitCode(err) != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}
	var got struct {
		Request struct {
			Privacy string `json:"privacy"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json output: %v\n%s", err, stdout.String())
	}
	if got.Request.Privacy != "private" {
		t.Fatalf("privacy = %q\n%s", got.Request.Privacy, stdout.String())
	}
}

func TestYouTubePlaylistCreateAndAdd(t *testing.T) {
	var createBody, addBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		switch r.URL.Path {
		case "/youtube/v3/playlists":
			createBody = body
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "PL1",
				"snippet": map[string]any{"title": "Test"},
				"status":  map[string]any{"privacyStatus": "private"},
			})
		case "/youtube/v3/playlistItems":
			addBody = body
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ITEM1"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write:   fixedYouTubeTestService(svc),
		Account: unexpectedYouTubeTestService(t, "write command called read service"),
	})
	flags := &RootFlags{Account: "me@example.com"}
	if err := runKong(t, &YouTubePlaylistsCreateCmd{}, []string{"--title", " Test ", "--description", " proof ", "--privacy", "private"}, ctx, flags); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := runKong(t, &YouTubePlaylistsAddCmd{}, []string{"--playlist-id", "PL1", "--video-id", "VID1", "--position", "0"}, ctx, flags); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(string(createBody), `"title":"Test"`) ||
		!strings.Contains(string(createBody), `"description":"proof"`) ||
		!strings.Contains(string(createBody), `"privacyStatus":"private"`) {
		t.Fatalf("create body = %s", createBody)
	}
	if !strings.Contains(string(addBody), `"playlistId":"PL1"`) ||
		!strings.Contains(string(addBody), `"videoId":"VID1"`) ||
		!strings.Contains(string(addBody), `"position":0`) {
		t.Fatalf("add body = %s", addBody)
	}
}

func TestYouTubePlaylistRemoveAndDelete(t *testing.T) {
	var deletedItems, deletedPlaylists []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/youtube/v3/playlistItems" && r.Method == http.MethodGet:
			if r.URL.Query().Get("playlistId") != "PL1" || r.URL.Query().Get("videoId") != "VID1" {
				t.Fatalf("lookup query = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"id": "ITEM1"}}})
		case r.URL.Path == "/youtube/v3/playlistItems" && r.Method == http.MethodDelete:
			deletedItems = append(deletedItems, r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/youtube/v3/playlists" && r.Method == http.MethodDelete:
			deletedPlaylists = append(deletedPlaylists, r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write: fixedYouTubeTestService(svc),
	})
	flags := &RootFlags{Account: "me@example.com", Force: true}
	if err := runKong(t, &YouTubePlaylistsRemoveCmd{}, []string{"--playlist-id", "PL1", "--video-id", "VID1"}, ctx, flags); err != nil {
		t.Fatalf("remove by video: %v", err)
	}
	if err := runKong(t, &YouTubePlaylistsRemoveCmd{}, []string{"--item-id", "ITEM2"}, ctx, flags); err != nil {
		t.Fatalf("remove by item: %v", err)
	}
	if err := runKong(t, &YouTubePlaylistsDeleteCmd{}, []string{"PL1"}, ctx, flags); err != nil {
		t.Fatalf("delete playlist: %v", err)
	}
	if strings.Join(deletedItems, ",") != "ITEM1,ITEM2" {
		t.Fatalf("deleted items = %v", deletedItems)
	}
	if strings.Join(deletedPlaylists, ",") != "PL1" {
		t.Fatalf("deleted playlists = %v", deletedPlaylists)
	}
}

func TestYouTubeMutationValidationBeforeService(t *testing.T) {
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Write: unexpectedYouTubeTestService(t, "invalid command must not create service"),
	})
	flags := &RootFlags{Account: "me@example.com", Force: true}
	tests := []struct {
		name string
		cmd  interface {
			Run(context.Context, *RootFlags) error
		}
		args []string
		want string
	}{
		{name: "negative position", cmd: &YouTubePlaylistsAddCmd{}, args: []string{"--playlist-id", "PL1", "--video-id", "VID1", "--position=-2"}, want: "--position must be >= 0"},
		{name: "remove missing selector", cmd: &YouTubePlaylistsRemoveCmd{}, want: "set --video-id or --item-id"},
		{name: "remove both selectors", cmd: &YouTubePlaylistsRemoveCmd{}, args: []string{"--video-id", "VID1", "--item-id", "ITEM1"}, want: "either --video-id or --item-id"},
		{name: "remove missing playlist", cmd: &YouTubePlaylistsRemoveCmd{}, args: []string{"--video-id", "VID1"}, want: "--playlist-id is required"},
		{name: "delete blank", cmd: &YouTubePlaylistsDeleteCmd{}, args: []string{" "}, want: "playlist-id is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runKong(t, tt.cmd, tt.args, ctx, flags)
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected usage error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestYouTubeReadCommandsNeverCallWriteService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	defer srv.Close()

	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", youtube.NewService)
	ctx := withYouTubeTestServices(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), youtubeTestServices{
		Account: fixedYouTubeTestService(svc),
		Write:   unexpectedYouTubeTestService(t, "read command called write service"),
	})
	flags := &RootFlags{Account: "me@example.com"}
	if err := runKong(t, &YouTubeSubscriptionsListCmd{}, []string{"--max", "1"}, ctx, flags); err != nil {
		t.Fatalf("subscriptions list: %v", err)
	}
	if err := runKong(t, &YouTubePlaylistsListCmd{}, []string{"--mine", "--max", "1"}, ctx, flags); err != nil {
		t.Fatalf("playlists list: %v", err)
	}
}

func TestWrapYouTubeWriteError(t *testing.T) {
	original := errors.New("ACCESS_TOKEN_SCOPE_INSUFFICIENT")
	err := wrapYouTubeWriteError(original, &RootFlags{Account: "me@example.com"})
	if !strings.Contains(err.Error(), youtubeForceSSLOAuthScope) ||
		!strings.Contains(err.Error(), "gog auth add me@example.com") ||
		!errors.Is(err, original) {
		t.Fatalf("wrapped error = %v", err)
	}
	if got := wrapYouTubeWriteError(errors.New("quota exceeded"), &RootFlags{Account: "me@example.com"}); strings.Contains(got.Error(), "auth add") {
		t.Fatalf("unrelated error was wrapped: %v", got)
	}
}
