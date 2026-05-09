package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

var errUnexpectedChatServiceCall = errors.New("unexpected chat service call")

func useFakeChatService(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }
}

func TestChatSpaceDisplayNameMatches(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		query       string
		exact       bool
		want        bool
	}{
		{name: "substring case insensitive", displayName: "My Project Team", query: "project", want: true},
		{name: "substring miss", displayName: "Random Channel", query: "project", want: false},
		{name: "exact case insensitive", displayName: "Project Alpha", query: "project alpha", exact: true, want: true},
		{name: "exact does not substring", displayName: "Project Alpha", query: "project", exact: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chatSpaceDisplayNameMatches(tt.displayName, tt.query, tt.exact)
			if got != tt.want {
				t.Fatalf("match = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestExecute_ChatSpacesList_Text(t *testing.T) {
	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces": []map[string]any{
				{"name": "spaces/aaa", "displayName": "Engineering", "spaceType": "SPACE"},
				{"name": "spaces/bbb", "displayName": "", "spaceType": "DIRECT_MESSAGE"},
			},
			"nextPageToken": "npt",
		})
	})

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "spaces", "list", "--max", "2"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "# Next page: --page npt") {
			t.Fatalf("unexpected stderr=%q", errOut)
		}
	})
	if !strings.Contains(out, "RESOURCE") || !strings.Contains(out, "spaces/aaa") || !strings.Contains(out, "Engineering") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_ChatSpacesList_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "user@gmail.com", "chat", "spaces", "list"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_ChatSpacesFind_JSON(t *testing.T) {
	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces")) {
			http.NotFound(w, r)
			return
		}
		token := r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		if token == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{
					{"name": "spaces/aaa", "displayName": "Engineering", "spaceType": "SPACE"},
				},
				"nextPageToken": "next",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces": []map[string]any{
				{"name": "spaces/bbb", "displayName": "Other", "spaceType": "SPACE"},
			},
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "find", "Engineering"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Spaces []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Spaces) != 1 || parsed.Spaces[0].Resource != "spaces/aaa" {
		t.Fatalf("unexpected spaces: %#v", parsed.Spaces)
	}
}

func TestExecute_ChatSpacesFind_Substring(t *testing.T) {
	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces": []map[string]any{
				{"name": "spaces/aaa", "displayName": "My Project Team", "spaceType": "SPACE"},
				{"name": "spaces/bbb", "displayName": "Project Alpha", "spaceType": "SPACE"},
				{"name": "spaces/ccc", "displayName": "Random Channel", "spaceType": "SPACE"},
				{"name": "spaces/ddd", "displayName": "Old Project Archive", "spaceType": "SPACE"},
			},
		})
	})

	// Default behavior: substring, case-insensitive. "project" must match all
	// three entries whose DisplayName contains "Project", and must exclude the
	// unrelated "Random Channel".
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "find", "project"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Spaces []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := make(map[string]bool, len(parsed.Spaces))
	for _, s := range parsed.Spaces {
		got[s.Resource] = true
	}
	if len(got) != 3 || !got["spaces/aaa"] || !got["spaces/bbb"] || !got["spaces/ddd"] {
		t.Fatalf("substring search must match all three 'Project' spaces, got %#v", parsed.Spaces)
	}
	if got["spaces/ccc"] {
		t.Fatalf("substring search must not match 'Random Channel', got %#v", parsed.Spaces)
	}
}

func TestExecute_ChatSpacesFind_Exact(t *testing.T) {
	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces": []map[string]any{
				{"name": "spaces/aaa", "displayName": "My Project Team", "spaceType": "SPACE"},
				{"name": "spaces/bbb", "displayName": "Project Alpha", "spaceType": "SPACE"},
			},
		})
	})

	// --exact must restore the legacy case-insensitive equality behavior: only
	// the space whose DisplayName equals "project alpha" (ignoring case)
	// is returned.
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "find", "--exact", "project alpha"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Spaces []struct {
			Resource string `json:"resource"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Spaces) != 1 || parsed.Spaces[0].Resource != "spaces/bbb" {
		t.Fatalf("--exact must return only 'Project Alpha', got %#v", parsed.Spaces)
	}
}

func TestExecute_ChatSpacesCreate_JSON(t *testing.T) {
	var mu sync.Mutex
	var gotType string
	var gotMembers int

	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		space := body["space"].(map[string]any)
		members := body["memberships"].([]any)
		mu.Lock()
		gotType, _ = space["spaceType"].(string)
		gotMembers = len(members)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/new",
			"displayName": "Engineering",
			"spaceType":   "SPACE",
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "create", "Engineering", "--member", "a@b.com", "--member", "b@b.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Space struct {
			Name string `json:"name"`
		} `json:"space"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Space.Name != "spaces/new" {
		t.Fatalf("unexpected space: %#v", parsed.Space)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotType != "SPACE" || gotMembers != 2 {
		t.Fatalf("unexpected setup: type=%q members=%d", gotType, gotMembers)
	}
}

func TestExecute_ChatMessagesList_Text_Unread(t *testing.T) {
	var gotFilter string

	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/spaceReadState") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"lastReadTime": "2025-01-01T00:00:00Z"})
		case strings.Contains(r.URL.Path, "/messages") && r.Method == http.MethodGet:
			gotFilter = r.URL.Query().Get("filter")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{
					"name":       "spaces/aaa/messages/msg1",
					"text":       "hello",
					"createTime": "2025-01-02T00:00:00Z",
					"sender": map[string]any{
						"displayName": "Ada",
					},
					"thread": map[string]any{
						"name": "spaces/aaa/threads/t1",
					},
				}},
				"nextPageToken": "npt",
			})
		default:
			http.NotFound(w, r)
		}
	})

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "list", "spaces/aaa", "--unread", "--thread", "t1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "# Next page: --page npt") {
			t.Fatalf("unexpected stderr=%q", errOut)
		}
	})
	if !strings.Contains(out, "RESOURCE") || !strings.Contains(out, "messages/msg1") || !strings.Contains(out, "hello") {
		t.Fatalf("unexpected out=%q", out)
	}
	if !strings.Contains(gotFilter, "createTime > \"2025-01-01T00:00:00Z\"") {
		t.Fatalf("unexpected filter: %q", gotFilter)
	}
	if !strings.Contains(gotFilter, "thread.name = \"spaces/aaa/threads/t1\"") {
		t.Fatalf("unexpected thread filter: %q", gotFilter)
	}
}

func TestExecute_ChatMessagesSend_JSON(t *testing.T) {
	var gotText string
	var gotThread string

	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotText, _ = body["text"].(string)
		if thread, ok := body["thread"].(map[string]any); ok {
			gotThread, _ = thread["name"].(string)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/aaa/messages/msg2",
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "messages", "send", "spaces/aaa", "--text", "hello", "--thread", "t1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if gotText != "hello" {
		t.Fatalf("unexpected text: %q", gotText)
	}
	if gotThread != "spaces/aaa/threads/t1" {
		t.Fatalf("unexpected thread: %q", gotThread)
	}
	if !strings.Contains(out, "spaces/aaa/messages/msg2") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_ChatThreadsList_Text(t *testing.T) {
	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": []map[string]any{
				{"name": "spaces/aaa/messages/m1", "thread": map[string]any{"name": "spaces/aaa/threads/t1"}, "text": "t1"},
				{"name": "spaces/aaa/messages/m2", "thread": map[string]any{"name": "spaces/aaa/threads/t1"}, "text": "t1 again"},
				{"name": "spaces/aaa/messages/m3", "thread": map[string]any{"name": "spaces/aaa/threads/t2"}, "text": "t2"},
			},
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "threads", "list", "spaces/aaa"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if strings.Count(out, "threads/t1") != 1 || !strings.Contains(out, "threads/t2") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_ChatDMSpace_JSON(t *testing.T) {
	var gotMember string

	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		members := body["memberships"].([]any)
		member := members[0].(map[string]any)["member"].(map[string]any)
		gotMember, _ = member["name"].(string)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "spaces/dm1",
			"spaceType": "DIRECT_MESSAGE",
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "dm", "space", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if gotMember != "users/user@example.com" {
		t.Fatalf("unexpected member: %q", gotMember)
	}
	if !strings.Contains(out, "spaces/dm1") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_ChatDMSend_JSON(t *testing.T) {
	var gotText string

	useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces:setup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "spaces/dm1",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/spaces/dm1/messages"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotText, _ = body["text"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "spaces/dm1/messages/m1",
			})
		default:
			http.NotFound(w, r)
		}
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "dm", "send", "user@example.com", "--text", "ping"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if gotText != "ping" {
		t.Fatalf("unexpected text: %q", gotText)
	}
	if !strings.Contains(out, "spaces/dm1/messages/m1") {
		t.Fatalf("unexpected out=%q", out)
	}
}
