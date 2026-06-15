package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/chat/v1"
)

func useFakeChatService(t *testing.T, handler http.HandlerFunc) *chat.Service {
	t.Helper()
	return newChatTestService(t, handler)
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
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "chat", "spaces", "list", "--max", "2"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stderr, "# More results: use --all/--all-pages to fetch every page, or --page npt for the next page") {
		t.Fatalf("unexpected stderr=%q", result.stderr)
	}
	if !strings.Contains(result.stdout, "RESOURCE") || !strings.Contains(result.stdout, "spaces/aaa") || !strings.Contains(result.stdout, "Engineering") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_ChatSpacesList_ConsumerBlocked(t *testing.T) {
	result := executeWithChatTestServiceFactory(
		t,
		[]string{"--account", "user@gmail.com", "chat", "spaces", "list"},
		unexpectedChatTestService(t, "unexpected chat service call"),
	)
	err := result.err
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_ChatListInvalidMaxFailsBeforeWorkspaceCheck(t *testing.T) {
	cases := [][]string{
		{"--account", "user@gmail.com", "chat", "spaces", "list", "--max", "0"},
		{"--account", "user@gmail.com", "chat", "spaces", "list", "--max=-1"},
		{"--account", "user@gmail.com", "chat", "spaces", "find", "Engineering", "--max", "0"},
		{"--account", "user@gmail.com", "chat", "spaces", "find", "Engineering", "--max=-1"},
		{"--account", "user@gmail.com", "chat", "messages", "list", "spaces/AAA", "--max", "0"},
		{"--account", "user@gmail.com", "chat", "messages", "list", "spaces/AAA", "--max=-1"},
		{"--account", "user@gmail.com", "chat", "threads", "list", "spaces/AAA", "--max", "0"},
		{"--account", "user@gmail.com", "chat", "threads", "list", "spaces/AAA", "--max=-1"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			result := executeWithChatTestServiceFactory(
				t,
				args,
				unexpectedChatTestService(t, "expected max validation to fail before creating chat service"),
			)
			err := result.err
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestExecute_ChatSpacesFind_JSON(t *testing.T) {
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "spaces", "find", "Engineering"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Spaces []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Spaces) != 1 || parsed.Spaces[0].Resource != "spaces/aaa" {
		t.Fatalf("unexpected spaces: %#v", parsed.Spaces)
	}
}

func TestExecute_ChatSpacesFind_Substring(t *testing.T) {
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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
	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "spaces", "find", "project"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Spaces []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
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
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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
	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "spaces", "find", "--exact", "project alpha"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Spaces []struct {
			Resource string `json:"resource"`
		} `json:"spaces"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
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

	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "spaces", "create", "Engineering", "--member", "a@b.com", "--member", "b@b.com"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Space struct {
			Name string `json:"name"`
		} `json:"space"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
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

func TestExecute_ChatSpacesCreate_InvalidMemberFailsBeforeDryRun(t *testing.T) {
	testCases := [][]string{
		{"--account", "a@b.com", "--dry-run", "chat", "spaces", "create", "Team", "--member", "nope"},
		{"--account", "a@b.com", "--dry-run", "chat", "spaces", "create", "Team", "--member", "Tester <x@example.com>"},
		{"--account", "a@b.com", "--dry-run", "chat", "spaces", "create", "Team", "--member", "users/"},
		{"--account", "a@b.com", "--dry-run", "chat", "spaces", "create", "Team", "--member", "users/foo/bar"},
		{"--account", "a@b.com", "--dry-run", "chat", "spaces", "create", "Team", "--member", "users/Tester <x@example.com>"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[4:], "_"), func(t *testing.T) {
			result := executeWithChatTestServiceFactory(
				t,
				args,
				unexpectedChatTestService(t, "expected validation to fail before creating chat service"),
			)
			var exitErr *ExitError
			if !errors.As(result.err, &exitErr) || exitErr.Code != 2 || !strings.Contains(result.err.Error(), "invalid --member") {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}

func TestExecute_ChatMessagesList_Text_Unread(t *testing.T) {
	var gotFilter string

	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "chat", "messages", "list", "spaces/aaa", "--unread", "--thread", "t1"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stderr, "# More results: use --all/--all-pages to fetch every page, or --page npt for the next page") {
		t.Fatalf("unexpected stderr=%q", result.stderr)
	}
	if !strings.Contains(result.stdout, "RESOURCE") || !strings.Contains(result.stdout, "messages/msg1") || !strings.Contains(result.stdout, "hello") {
		t.Fatalf("unexpected out=%q", result.stdout)
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

	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "messages", "send", "spaces/aaa", "--text", "hello", "--thread", "t1"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if gotText != "hello" {
		t.Fatalf("unexpected text: %q", gotText)
	}
	if gotThread != "spaces/aaa/threads/t1" {
		t.Fatalf("unexpected thread: %q", gotThread)
	}
	if !strings.Contains(result.stdout, "spaces/aaa/messages/msg2") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_ChatMessagesSend_WithAttachment(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(imgPath, []byte("\x89PNG\r\n\x1a\nfake"), 0o600); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	var uploadHits int
	var gotUploadParent string
	var gotAttachmentToken string

	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "attachments:upload"):
			uploadHits++
			gotUploadParent = strings.TrimPrefix(r.URL.Path, "/upload/v1/")
			gotUploadParent = strings.TrimPrefix(gotUploadParent, "v1/")
			gotUploadParent = strings.TrimSuffix(gotUploadParent, "/attachments:upload")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"attachmentDataRef": map[string]any{"attachmentUploadToken": "tok-123"},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if atts, ok := body["attachment"].([]any); ok && len(atts) == 1 {
				if att, ok := atts[0].(map[string]any); ok {
					if ref, ok := att["attachmentDataRef"].(map[string]any); ok {
						gotAttachmentToken, _ = ref["attachmentUploadToken"].(string)
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "spaces/aaa/messages/msg3"})
		default:
			http.NotFound(w, r)
		}
	})

	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "messages", "send", "spaces/aaa", "--text", "look", "--attach", imgPath}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if uploadHits != 1 {
		t.Fatalf("expected exactly 1 upload, got %d", uploadHits)
	}
	if gotUploadParent != "spaces/aaa" {
		t.Fatalf("unexpected upload parent: %q", gotUploadParent)
	}
	if gotAttachmentToken != "tok-123" {
		t.Fatalf("attachment token not forwarded to message, got %q", gotAttachmentToken)
	}
	if !strings.Contains(result.stdout, "spaces/aaa/messages/msg3") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_ChatMessagesSend_AttachmentOnlyNoText(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(imgPath, []byte("fake-bytes"), 0o600); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	var messageSent bool
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "attachments:upload"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"attachmentDataRef": map[string]any{"attachmentUploadToken": "tok-xyz"},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages"):
			messageSent = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "spaces/aaa/messages/msg4"})
		default:
			http.NotFound(w, r)
		}
	})

	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "chat", "messages", "send", "spaces/aaa", "--attach", imgPath}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !messageSent {
		t.Fatalf("expected message to be sent with attachment-only payload")
	}
}

func TestExecute_ChatMessagesSend_NoTextNoAttachFails(t *testing.T) {
	result := executeWithChatTestServiceFactory(
		t,
		[]string{"--account", "a@b.com", "chat", "messages", "send", "spaces/aaa"},
		unexpectedChatTestService(t, "expected validation to fail before creating chat service"),
	)
	var exitErr *ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("unexpected err: %v", result.err)
	}
}

func TestExecute_ChatMessagesSend_InvalidResourceFailsBeforeDryRun(t *testing.T) {
	testCases := [][]string{
		{"--account", "a@b.com", "--dry-run", "chat", "messages", "send", "spaces/AAA/extra", "--text", "ping"},
		{"--account", "a@b.com", "--dry-run", "chat", "messages", "send", "spaces/AAA", "--text", "ping", "--thread", "spaces/AAA/threads/t1/extra"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[4:], "_"), func(t *testing.T) {
			result := executeWithChatTestServiceFactory(
				t,
				args,
				unexpectedChatTestService(t, "expected validation to fail before creating chat service"),
			)
			var exitErr *ExitError
			if !errors.As(result.err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}

func TestExecute_ChatThreadsList_Text(t *testing.T) {
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "chat", "threads", "list", "spaces/aaa"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if strings.Count(result.stdout, "threads/t1") != 1 || !strings.Contains(result.stdout, "threads/t2") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_ChatDMSpace_JSON(t *testing.T) {
	var gotMember string

	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "dm", "space", "user@example.com"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if gotMember != "users/user@example.com" {
		t.Fatalf("unexpected member: %q", gotMember)
	}
	if !strings.Contains(result.stdout, "spaces/dm1") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_ChatDMSend_JSON(t *testing.T) {
	var gotText string

	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithChatTestService(t, []string{"--json", "--account", "a@b.com", "chat", "dm", "send", "user@example.com", "--text", "ping"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if gotText != "ping" {
		t.Fatalf("unexpected text: %q", gotText)
	}
	if !strings.Contains(result.stdout, "spaces/dm1/messages/m1") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_ChatDM_InvalidEmailFailsBeforeDryRun(t *testing.T) {
	testCases := [][]string{
		{"--account", "a@b.com", "--dry-run", "chat", "dm", "send", "nope", "--text", "ping"},
		{"--account", "a@b.com", "--dry-run", "chat", "dm", "space", "nope"},
		{"--account", "a@b.com", "--dry-run", "chat", "dm", "send", "Tester <x@example.com>", "--text", "ping"},
		{"--account", "a@b.com", "--dry-run", "chat", "dm", "send", "x@example.com", "--text", "ping", "--thread", "spaces/AAA/threads/t1/extra"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[4:], "_"), func(t *testing.T) {
			result := executeWithChatTestServiceFactory(
				t,
				args,
				unexpectedChatTestService(t, "expected validation to fail before creating chat service"),
			)
			var exitErr *ExitError
			if !errors.As(result.err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}
