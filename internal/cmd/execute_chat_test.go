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

func TestExecute_ChatSpacesList_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestExecute_ChatSpacesFind_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestExecute_ChatSpacesCreate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var mu sync.Mutex
	var gotType string
	var gotMembers int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotFilter string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "list", "spaces/aaa", "--unread"}); err != nil {
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
}

func TestExecute_ChatMessagesSend_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotText string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotText, _ = body["text"].(string)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/aaa/messages/msg2",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "messages", "send", "spaces/aaa", "--text", "hello"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if gotText != "hello" {
		t.Fatalf("unexpected text: %q", gotText)
	}
	if !strings.Contains(out, "spaces/aaa/messages/msg2") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_ChatThreadsList_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotMember string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotText string

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
			gotText, _ = body["text"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "spaces/dm1/messages/m1",
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
