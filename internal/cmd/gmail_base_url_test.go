package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
)

func TestGmailBaseURLRoutesAuthenticatedCommands(t *testing.T) {
	requests := make(chan *http.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":"message-1"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	t.Setenv("GOG_GMAIL_BASE_URL", server.URL+"/")

	runtime := &app.Runtime{ServicesManaged: true}
	result := executeWithTestRuntime(t, []string{
		"--json", "--access-token", "test-token", "gmail", "get", "message-1",
	}, runtime)
	if result.err != nil {
		t.Fatalf("gmail get: %v\nstderr=%s", result.err, result.stderr)
	}

	result = executeWithTestRuntime(t, []string{
		"--json", "--force", "--access-token", "test-token", "gmail", "batch", "delete", "message-1",
	}, runtime)
	if result.err != nil {
		t.Fatalf("gmail batch delete: %v\nstderr=%s", result.err, result.stderr)
	}

	wantRequests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/gmail/v1/users/me/messages/message-1"},
		{method: http.MethodPost, path: "/gmail/v1/users/me/messages/batchDelete"},
	}
	for _, want := range wantRequests {
		request := <-requests
		if request.Method != want.method || request.URL.Path != want.path {
			t.Errorf("request = %s %s, want %s %s", request.Method, request.URL.Path, want.method, want.path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
	}
}
