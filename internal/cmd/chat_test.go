package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

// Tests for chat_helpers.go

func TestNormalizeSpace(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:  "already normalized",
			input: "spaces/abc123",
			want:  "spaces/abc123",
		},
		{
			name:  "without prefix",
			input: "abc123",
			want:  "spaces/abc123",
		},
		{
			name:  "with leading whitespace",
			input: "  spaces/abc123",
			want:  "spaces/abc123",
		},
		{
			name:  "with trailing whitespace",
			input: "spaces/abc123  ",
			want:  "spaces/abc123",
		},
		{
			name:  "id only with whitespace",
			input: "  abc123  ",
			want:  "spaces/abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSpace(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeSpace(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("normalizeSpace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSpaceID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"spaces/abc123", "abc123"},
		{"abc123", "abc123"},
		{"spaces/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := spaceID(tt.input); got != tt.want {
				t.Errorf("spaceID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeUser(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"users/user@example.com", "users/user@example.com"},
		{"user@example.com", "users/user@example.com"},
		{"  user@example.com  ", "users/user@example.com"},
		{"  users/user@example.com  ", "users/user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeUser(tt.input); got != tt.want {
				t.Errorf("normalizeUser(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeThread(t *testing.T) {
	tests := []struct {
		name    string
		space   string
		thread  string
		want    string
		wantErr bool
	}{
		{
			name:    "empty thread",
			space:   "spaces/abc",
			thread:  "",
			wantErr: true,
		},
		{
			name:    "whitespace only thread",
			space:   "spaces/abc",
			thread:  "   ",
			wantErr: true,
		},
		{
			name:   "full thread resource",
			space:  "spaces/abc",
			thread: "spaces/abc/threads/t1",
			want:   "spaces/abc/threads/t1",
		},
		{
			name:    "invalid full resource missing threads",
			space:   "spaces/abc",
			thread:  "spaces/abc/messages/m1",
			wantErr: true,
		},
		{
			name:   "thread id only",
			space:  "spaces/abc",
			thread: "t1",
			want:   "spaces/abc/threads/t1",
		},
		{
			name:   "thread with threads/ prefix",
			space:  "spaces/abc",
			thread: "threads/t1",
			want:   "spaces/abc/threads/t1",
		},
		{
			name:    "invalid thread id with slash",
			space:   "spaces/abc",
			thread:  "t1/extra",
			wantErr: true,
		},
		{
			name:   "space without prefix",
			space:  "abc",
			thread: "t1",
			want:   "spaces/abc/threads/t1",
		},
		{
			name:    "empty space",
			space:   "",
			thread:  "t1",
			wantErr: true,
		},
		{
			name:   "thread id with whitespace",
			space:  "spaces/abc",
			thread: "  t1  ",
			want:   "spaces/abc/threads/t1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeThread(tt.space, tt.thread)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeThread(%q, %q) error = %v, wantErr %v", tt.space, tt.thread, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("normalizeThread(%q, %q) = %q, want %q", tt.space, tt.thread, got, tt.want)
			}
		})
	}
}

func TestRequireWorkspaceAccount(t *testing.T) {
	tests := []struct {
		account string
		wantErr bool
	}{
		{"user@gmail.com", true},
		{"user@googlemail.com", true},
		{"user@company.com", false},
		{"user@workspace.org", false},
		{"admin@example.co", false},
	}

	for _, tt := range tests {
		t.Run(tt.account, func(t *testing.T) {
			err := requireWorkspaceAccount(tt.account)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireWorkspaceAccount(%q) error = %v, wantErr %v", tt.account, err, tt.wantErr)
			}
		})
	}
}

func TestParseCommaArgs(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{
			name:   "empty",
			values: nil,
			want:   []string{},
		},
		{
			name:   "single value",
			values: []string{"a@b.com"},
			want:   []string{"a@b.com"},
		},
		{
			name:   "multiple separate values",
			values: []string{"a@b.com", "c@d.com"},
			want:   []string{"a@b.com", "c@d.com"},
		},
		{
			name:   "comma separated",
			values: []string{"a@b.com,c@d.com"},
			want:   []string{"a@b.com", "c@d.com"},
		},
		{
			name:   "mixed",
			values: []string{"a@b.com,c@d.com", "e@f.com"},
			want:   []string{"a@b.com", "c@d.com", "e@f.com"},
		},
		{
			name:   "with whitespace",
			values: []string{" a@b.com , c@d.com "},
			want:   []string{"a@b.com", "c@d.com"},
		},
		{
			name:   "empty entries filtered",
			values: []string{"a@b.com,,c@d.com", ""},
			want:   []string{"a@b.com", "c@d.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaArgs(tt.values)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommaArgs(%v) = %v, want %v", tt.values, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommaArgs(%v)[%d] = %q, want %q", tt.values, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestChatSpaceType(t *testing.T) {
	tests := []struct {
		name  string
		space *chat.Space
		want  string
	}{
		{
			name:  "nil space",
			space: nil,
			want:  "",
		},
		{
			name:  "spaceType set",
			space: &chat.Space{SpaceType: "SPACE"},
			want:  "SPACE",
		},
		{
			name:  "type set (legacy)",
			space: &chat.Space{Type: "DIRECT_MESSAGE"},
			want:  "DIRECT_MESSAGE",
		},
		{
			name:  "spaceType preferred over type",
			space: &chat.Space{SpaceType: "SPACE", Type: "DM"},
			want:  "SPACE",
		},
		{
			name:  "empty",
			space: &chat.Space{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatSpaceType(tt.space); got != tt.want {
				t.Errorf("chatSpaceType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatMessageSender(t *testing.T) {
	tests := []struct {
		name string
		msg  *chat.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "nil sender",
			msg:  &chat.Message{},
			want: "",
		},
		{
			name: "display name set",
			msg:  &chat.Message{Sender: &chat.User{DisplayName: "Ada"}},
			want: "Ada",
		},
		{
			name: "name set",
			msg:  &chat.Message{Sender: &chat.User{Name: "users/abc"}},
			want: "users/abc",
		},
		{
			name: "display name preferred",
			msg:  &chat.Message{Sender: &chat.User{DisplayName: "Ada", Name: "users/abc"}},
			want: "Ada",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatMessageSender(tt.msg); got != tt.want {
				t.Errorf("chatMessageSender() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatMessageText(t *testing.T) {
	tests := []struct {
		name string
		msg  *chat.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "text set",
			msg:  &chat.Message{Text: "hello"},
			want: "hello",
		},
		{
			name: "argument text set",
			msg:  &chat.Message{ArgumentText: "arg text"},
			want: "arg text",
		},
		{
			name: "text preferred",
			msg:  &chat.Message{Text: "hello", ArgumentText: "arg text"},
			want: "hello",
		},
		{
			name: "empty message",
			msg:  &chat.Message{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatMessageText(tt.msg); got != tt.want {
				t.Errorf("chatMessageText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatMessageThread(t *testing.T) {
	tests := []struct {
		name string
		msg  *chat.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "nil thread",
			msg:  &chat.Message{},
			want: "",
		},
		{
			name: "thread set",
			msg:  &chat.Message{Thread: &chat.Thread{Name: "spaces/abc/threads/t1"}},
			want: "spaces/abc/threads/t1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatMessageThread(tt.msg); got != tt.want {
				t.Errorf("chatMessageThread() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeChatText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello\tworld", "hello world"},
		{"hello\nworld", "hello world"},
		{"hello\rworld", "hello world"},
		{"hello\t\n\rworld", "hello   world"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeChatText(tt.input); got != tt.want {
				t.Errorf("sanitizeChatText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Tests for chat_dm.go

func TestChatDMSend_MissingEmail(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "dm", "send", "", "--text", "hi"})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
	if !strings.Contains(err.Error(), "required: email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatDMSend_MissingText(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "dm", "send", "user@example.com"})
	if err == nil {
		t.Fatal("expected error for missing text")
	}
	if !strings.Contains(err.Error(), "required: --text") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatDMSend_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatal("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "user@gmail.com", "chat", "dm", "send", "other@example.com", "--text", "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatDMSend_InvalidThread(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "spaces/dm1",
			})
			return
		}
		http.NotFound(w, r)
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

	err = Execute([]string{"--account", "a@company.com", "chat", "dm", "send", "user@example.com", "--text", "hi", "--thread", "invalid/path/format"})
	if err == nil {
		t.Fatal("expected error for invalid thread")
	}
	if !strings.Contains(err.Error(), "invalid thread") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatDMSend_WithThread_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotThread string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "spaces/dm1",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces/dm1/messages"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if thread, ok := body["thread"].(map[string]any); ok {
				gotThread, _ = thread["name"].(string)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":   "spaces/dm1/messages/m1",
				"thread": map[string]any{"name": "spaces/dm1/threads/t1"},
			})
		default:
			http.NotFound(w, r)
		}
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
			if err := Execute([]string{"--account", "a@company.com", "chat", "dm", "send", "user@example.com", "--text", "hi", "--thread", "t1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotThread != "spaces/dm1/threads/t1" {
		t.Fatalf("unexpected thread: %q", gotThread)
	}
	if !strings.Contains(out, "thread") && !strings.Contains(out, "spaces/dm1/threads/t1") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestChatDMSpace_MissingEmail(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "dm", "space", ""})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
	if !strings.Contains(err.Error(), "required: email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatDMSpace_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatal("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "user@gmail.com", "chat", "dm", "space", "other@example.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatDMSpace_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "spaces/dm1",
				"displayName": "DM with User",
				"spaceType":   "DIRECT_MESSAGE",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--account", "a@company.com", "chat", "dm", "space", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "spaces/dm1") {
		t.Fatalf("unexpected out=%q", out)
	}
	if !strings.Contains(out, "DM with User") {
		t.Fatalf("unexpected out=%q", out)
	}
}

// Tests for chat_spaces.go

func TestChatSpacesList_EmptyResults(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
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

	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(errOut, "No spaces") {
		t.Fatalf("unexpected stderr=%q", errOut)
	}
}

func TestChatSpacesList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{
					{"name": "spaces/aaa", "displayName": "Engineering", "spaceType": "SPACE", "spaceThreadingState": "THREADED_MESSAGES"},
				},
				"nextPageToken": "npt",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--json", "--account", "a@company.com", "chat", "spaces", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Spaces []struct {
			Resource  string `json:"resource"`
			Name      string `json:"name"`
			SpaceType string `json:"type"`
			Threading string `json:"threading"`
		} `json:"spaces"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Spaces) != 1 {
		t.Fatalf("unexpected spaces count: %d", len(parsed.Spaces))
	}
	if parsed.Spaces[0].Resource != "spaces/aaa" {
		t.Fatalf("unexpected resource: %q", parsed.Spaces[0].Resource)
	}
	if parsed.Spaces[0].Threading != "THREADED_MESSAGES" {
		t.Fatalf("unexpected threading: %q", parsed.Spaces[0].Threading)
	}
	if parsed.NextPageToken != "npt" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestChatSpacesFind_MissingDisplayName(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "find", ""})
	if err == nil {
		t.Fatal("expected error for missing displayName")
	}
	if !strings.Contains(err.Error(), "required: displayName") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatSpacesFind_NoResults_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{
					{"name": "spaces/aaa", "displayName": "Engineering", "spaceType": "SPACE"},
				},
			})
			return
		}
		http.NotFound(w, r)
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

	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "find", "NonExistent"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(errOut, "No results") {
		t.Fatalf("unexpected stderr=%q", errOut)
	}
}

func TestChatSpacesFind_CaseInsensitive(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{
					{"name": "spaces/aaa", "displayName": "Engineering", "spaceType": "SPACE"},
				},
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "find", "ENGINEERING"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "spaces/aaa") {
		t.Fatalf("unexpected out=%q (should find case-insensitive match)", out)
	}
}

func TestChatSpacesCreate_MissingDisplayName(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "create", ""})
	if err == nil {
		t.Fatal("expected error for missing displayName")
	}
	if !strings.Contains(err.Error(), "required: displayName") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatSpacesCreate_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatal("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "user@gmail.com", "chat", "spaces", "create", "MySpace"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatSpacesCreate_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "spaces/new",
				"displayName": "Engineering",
				"spaceType":   "SPACE",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "create", "Engineering"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "spaces/new") {
		t.Fatalf("unexpected out=%q", out)
	}
	if !strings.Contains(out, "Engineering") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestChatSpacesCreate_WithCommaSeparatedMembers(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var mu sync.Mutex
	var gotMembers int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			memberships, _ := body["memberships"].([]any)
			mu.Lock()
			gotMembers = len(memberships)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "spaces/new",
				"displayName": "Team",
				"spaceType":   "SPACE",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--account", "a@company.com", "chat", "spaces", "create", "Team", "--member", "a@company.com,b@company.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	defer mu.Unlock()
	if gotMembers != 2 {
		t.Fatalf("unexpected members count: %d, expected 2", gotMembers)
	}
}

// Tests for chat_messages.go

func TestChatMessagesListEmptyResults(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
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

	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--account", "a@company.com", "chat", "messages", "list", "spaces/aaa"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(errOut, "No messages") {
		t.Fatalf("unexpected stderr=%q", errOut)
	}
}

func TestChatMessagesSend_MissingSpace(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "messages", "send", "", "--text", "hi"})
	if err == nil {
		t.Fatal("expected error for missing space")
	}
	if !strings.Contains(err.Error(), "required: space") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatMessagesSend_MissingText(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "messages", "send", "spaces/aaa"})
	if err == nil {
		t.Fatal("expected error for missing text")
	}
	if !strings.Contains(err.Error(), "required: --text") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatMessagesSend_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotText string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotText, _ = body["text"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "spaces/aaa/messages/m1",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--account", "a@company.com", "chat", "messages", "send", "spaces/aaa", "--text", "hello world"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotText != "hello world" {
		t.Fatalf("unexpected text: %q", gotText)
	}
	if !strings.Contains(out, "spaces/aaa/messages/m1") {
		t.Fatalf("unexpected out=%q", out)
	}
}

// Tests for chat_threads.go

func TestChatThreadsList_EmptyResults(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
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

	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--account", "a@company.com", "chat", "threads", "list", "spaces/aaa"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(errOut, "No threads") {
		t.Fatalf("unexpected stderr=%q", errOut)
	}
}

func TestChatThreadsList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"name": "spaces/aaa/messages/m1", "thread": map[string]any{"name": "spaces/aaa/threads/t1"}, "text": "hello", "createTime": "2025-01-01T00:00:00Z", "sender": map[string]any{"displayName": "Ada"}},
					{"name": "spaces/aaa/messages/m2", "thread": map[string]any{"name": "spaces/aaa/threads/t2"}, "text": "world", "createTime": "2025-01-02T00:00:00Z"},
				},
				"nextPageToken": "npt",
			})
			return
		}
		http.NotFound(w, r)
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
			if err := Execute([]string{"--json", "--account", "a@company.com", "chat", "threads", "list", "spaces/aaa"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Threads []struct {
			Thread     string `json:"thread"`
			Message    string `json:"message"`
			Sender     string `json:"sender"`
			Text       string `json:"text"`
			CreateTime string `json:"createTime"`
		} `json:"threads"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Threads) != 2 {
		t.Fatalf("unexpected threads count: %d", len(parsed.Threads))
	}
	if parsed.Threads[0].Thread != "spaces/aaa/threads/t1" {
		t.Fatalf("unexpected thread: %q", parsed.Threads[0].Thread)
	}
	if parsed.Threads[0].Sender != "Ada" {
		t.Fatalf("unexpected sender: %q", parsed.Threads[0].Sender)
	}
	if parsed.NextPageToken != "npt" {
		t.Fatalf("unexpected nextPageToken: %q", parsed.NextPageToken)
	}
}

func TestChatThreadsList_MissingSpace(t *testing.T) {
	err := Execute([]string{"--account", "a@company.com", "chat", "threads", "list", ""})
	if err == nil {
		t.Fatal("expected error for missing space")
	}
	if !strings.Contains(err.Error(), "required: space") {
		t.Fatalf("unexpected error: %v", err)
	}
}
