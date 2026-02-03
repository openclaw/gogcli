package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func TestYouTubeAccountsCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/youtube/v3/channels") || r.URL.Query().Get("managedByMe") != "true" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "ch1", "snippet": map[string]any{"title": "Brand", "customUrl": "brand"}},
			},
		})
	})
	stubYouTube(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &YouTubeAccountsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Brand") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestYouTubeChannelsCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/youtube/v3/channels") || r.URL.Query().Get("mine") != "true" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "ch2", "snippet": map[string]any{"title": "Owner", "customUrl": "owner"}},
			},
		})
	})
	stubYouTube(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &YouTubeChannelsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Owner") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubYouTube(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newYouTubeService
	svc, err := youtube.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new youtube service: %v", err)
	}
	newYouTubeService = func(context.Context, string) (*youtube.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newYouTubeService = orig
		srv.Close()
	})
	return srv
}
