package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

func TestNormalizeMessage(t *testing.T) {
	tests := []struct {
		name    string
		space   string
		msg     string
		want    string
		wantErr bool
	}{
		{
			name: "full resource path",
			msg:  "spaces/AAA/messages/msg1",
			want: "spaces/AAA/messages/msg1",
		},
		{
			name:  "bare id with space",
			space: "spaces/AAA",
			msg:   "msg1",
			want:  "spaces/AAA/messages/msg1",
		},
		{
			name:  "bare id with space id (no prefix)",
			space: "AAA",
			msg:   "msg1",
			want:  "spaces/AAA/messages/msg1",
		},
		{
			name:    "bare id without space",
			msg:     "msg1",
			wantErr: true,
		},
		{
			name:    "empty message",
			wantErr: true,
		},
		{
			name:    "spaces/ prefix but missing /messages/",
			msg:     "spaces/AAA/threads/t1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeMessage(tt.space, tt.msg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeMessage(%q, %q) error = %v, wantErr %v", tt.space, tt.msg, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("normalizeMessage(%q, %q) = %q, want %q", tt.space, tt.msg, got, tt.want)
			}
		})
	}
}

func TestExecute_ChatMessagesReactionsCreate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotEmoji string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/reactions")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if emoji, ok := body["emoji"].(map[string]any); ok {
			gotEmoji, _ = emoji["unicode"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/AAA/messages/msg1/reactions/r1",
			"emoji": map[string]any{"unicode": gotEmoji},
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "messages", "reactions", "create", "spaces/AAA/messages/msg1", "📦"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotEmoji != "📦" {
		t.Fatalf("unexpected emoji sent: %q", gotEmoji)
	}

	var parsed struct {
		Reaction struct {
			Name string `json:"name"`
		} `json:"reaction"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(parsed.Reaction.Name, "/reactions/") {
		t.Fatalf("unexpected reaction name: %q", parsed.Reaction.Name)
	}
}

func TestExecute_ChatMessagesReactionsCreate_BareIDWithSpace(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/reactions")) {
			http.NotFound(w, r)
			return
		}
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/AAA/messages/msg1/reactions/r1",
			"emoji": map[string]any{"unicode": "📦"},
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "reactions", "create", "msg1", "📦", "--space", "AAA"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotPath, "spaces/AAA/messages/msg1") {
		t.Fatalf("unexpected request path: %q", gotPath)
	}
}

func TestExecute_ChatMessagesReact_Shorthand(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotPath string
	var gotEmoji string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/reactions")) {
			http.NotFound(w, r)
			return
		}
		gotPath = r.URL.Path
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if emoji, ok := body["emoji"].(map[string]any); ok {
			gotEmoji, _ = emoji["unicode"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/AAA/messages/msg1/reactions/r1",
			"emoji": map[string]any{"unicode": gotEmoji},
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "react", "spaces/AAA/messages/msg1", "📦"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotPath, "spaces/AAA/messages/msg1/reactions") {
		t.Fatalf("unexpected request path: %q", gotPath)
	}
	if gotEmoji != "📦" {
		t.Fatalf("unexpected emoji sent: %q", gotEmoji)
	}
	if !strings.Contains(out, "spaces/AAA/messages/msg1/reactions/r1") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecute_ChatMessagesReactionsCreate_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "user@gmail.com", "chat", "messages", "reactions", "create", "spaces/AAA/messages/msg1", "📦"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_ChatMessagesReactionsList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reactions")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reactions": []map[string]any{
				{
					"name":  "spaces/AAA/messages/msg1/reactions/r1",
					"emoji": map[string]any{"unicode": "📦"},
					"user":  map[string]any{"displayName": "Ada"},
				},
			},
			"nextPageToken": "",
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "messages", "reactions", "list", "spaces/AAA/messages/msg1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Reactions []struct {
			Resource string `json:"resource"`
			Emoji    string `json:"emoji"`
			User     string `json:"user"`
		} `json:"reactions"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Reactions) != 1 || parsed.Reactions[0].Emoji != "📦" || parsed.Reactions[0].User != "Ada" {
		t.Fatalf("unexpected reactions: %#v", parsed.Reactions)
	}
}

func TestExecute_ChatMessagesReactionsDelete_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/reactions/")) {
			http.NotFound(w, r)
			return
		}
		deletedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "reactions", "delete", "spaces/AAA/messages/msg1/reactions/r1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(deletedPath, "/reactions/r1") {
		t.Fatalf("unexpected delete path: %q", deletedPath)
	}
	if !strings.Contains(out, "spaces/AAA/messages/msg1/reactions/r1") {
		t.Fatalf("unexpected output: %q", out)
	}
}
