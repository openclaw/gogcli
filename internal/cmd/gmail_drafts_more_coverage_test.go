package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGmailDraftsList_Empty(t *testing.T) {
	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "drafts", "list"},
		newGmailEmptyListTestService(t, "/gmail/v1/users/me/drafts", "drafts"),
	)
	if result.err != nil {
		t.Fatalf("list: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "No drafts") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestGmailDraftsGet_EmptyDraft_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "drafts", "get", "d1"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("get: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "Empty draft") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestGmailDraftsGet_JSON_DownloadNoAttachments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	payloadText := base64.RawURLEncoding.EncodeToString([]byte("Body"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
					"payload": map[string]any{
						"mimeType": "text/plain",
						"body":     map[string]any{"data": payloadText},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "drafts", "get", "d1", "--download"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("get: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "\"downloaded\"") {
		t.Fatalf("unexpected json: %q", result.stdout)
	}
}

func TestGmailDraftsGet_JSON_IndexedAttachments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
					"payload": map[string]any{"parts": []map[string]any{{
						"filename": "note.txt",
						"mimeType": "text/plain",
						"body":     map[string]any{"attachmentId": "opaque-id", "size": 7},
					}}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com", "gmail", "drafts", "get", "d1", "--use-indexed-attachment-ids",
	}, newGmailServiceFromServer(t, srv))
	if result.err != nil {
		t.Fatalf("get: %v\nstderr=%q", result.err, result.stderr)
	}
	var payload struct {
		Attachments []struct {
			AttachmentIndex *int `json:"attachmentIndex"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode: %v\nout=%q", err, result.stdout)
	}
	if len(payload.Attachments) != 1 || payload.Attachments[0].AttachmentIndex == nil || *payload.Attachments[0].AttachmentIndex != 0 {
		t.Fatalf("unexpected indexed attachments: %#v", payload.Attachments)
	}
	if strings.Contains(result.stdout, "opaque-id") || strings.Contains(result.stdout, "attachmentId") {
		t.Fatalf("indexed draft output leaked opaque id: %q", result.stdout)
	}
}

func TestGmailDraftsSend_Text(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_GMAIL_NO_SEND", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/send") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "drafts", "send", "d1"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("send: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "message_id") || !strings.Contains(result.stdout, "thread_id") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
}

func TestGmailDraftsCreate_ValidationErrors_More(t *testing.T) {
	result := executeWithTestRuntime(t, []string{"--account", "a@b.com", "gmail", "drafts", "create"}, nil)
	if result.err == nil {
		t.Fatalf("expected missing subject error")
	}
	result = executeWithTestRuntime(t, []string{"--account", "a@b.com", "gmail", "drafts", "create", "--subject", "S"}, nil)
	if result.err == nil {
		t.Fatalf("expected missing body error")
	}
}

func TestGmailDraftsCreate_FromUnverified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/sendAs/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"verificationStatus": "pending",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{
			"--plain", "--account", "a@b.com",
			"gmail", "drafts", "create",
			"--to", "a@b.com",
			"--subject", "S",
			"--body", "B",
			"--from", "alias@example.com",
		},
		newGmailServiceFromServer(t, srv),
	)
	if result.err == nil {
		t.Fatalf("expected unverified send-as error")
	}
}
