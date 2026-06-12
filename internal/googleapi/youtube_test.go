package googleapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/steipete/gogcli/internal/googleauth"
)

func TestYouTubeScopeContract(t *testing.T) {
	readScopes, err := googleauth.Scopes(googleauth.ServiceYouTube)
	if err != nil {
		t.Fatalf("Scopes(ServiceYouTube): %v", err)
	}

	if len(readScopes) != 1 || readScopes[0] != "https://www.googleapis.com/auth/youtube.readonly" {
		t.Fatalf("read scopes = %v", readScopes)
	}

	if scopeYouTubeForceSSL != "https://www.googleapis.com/auth/youtube.force-ssl" {
		t.Fatalf("write scope = %q", scopeYouTubeForceSSL)
	}

	if scopeYouTubeForceSSL == readScopes[0] {
		t.Fatal("write scope must remain separate from read scope")
	}
}

func TestNewYouTubeAPIKeyHTTPClientAddsKeyWithRetryTransport(t *testing.T) {
	client, err := newYouTubeAPIKeyHTTPClient(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("newYouTubeAPIKeyHTTPClient: %v", err)
	}

	if _, ok := client.Transport.(*RetryTransport); !ok {
		t.Fatalf("transport = %T, want *RetryTransport", client.Transport)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Fatalf("key = %q", got)
		}

		if got := r.URL.Query().Get("part"); got != "snippet" {
			t.Fatalf("part = %q", got)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/youtube/v3/videos?part=snippet", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
