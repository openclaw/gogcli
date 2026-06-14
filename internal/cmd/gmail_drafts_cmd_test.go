package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/mailmime"
)

func TestGmailDraftsListCmd_TextAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"drafts": []map[string]any{
					{"id": "d1", "message": map[string]any{"id": "m1", "threadId": "t1"}},
					{"id": "d2"},
				},
				"nextPageToken": "next",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	var textOut bytes.Buffer
	textCtx := withGmailTestService(newCmdRuntimeOutputContext(t, &textOut, io.Discard), svc)
	if err := runKong(t, &GmailDraftsListCmd{}, []string{}, textCtx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(textOut.String(), "ID") || !strings.Contains(textOut.String(), "d1") {
		t.Fatalf("unexpected text: %q", textOut.String())
	}

	var jsonOut bytes.Buffer
	jsonCtx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
	if err := runKong(t, &GmailDraftsListCmd{}, []string{}, jsonCtx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var parsed struct {
		Drafts []struct {
			ID        string `json:"id"`
			MessageID string `json:"messageId"`
			ThreadID  string `json:"threadId"`
		} `json:"drafts"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Drafts) != 2 || parsed.Drafts[0].ID != "d1" || parsed.NextPageToken != "next" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsGetCmd_Text(t *testing.T) {
	payloadText := base64.RawURLEncoding.EncodeToString([]byte("Hello"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
					"payload": map[string]any{
						"mimeType": "multipart/mixed",
						"headers": []map[string]any{
							{"name": "To", "value": "a@example.com"},
							{"name": "Cc", "value": "b@example.com"},
							{"name": "Subject", "value": "Draft"},
						},
						"parts": []map[string]any{
							{"mimeType": "text/plain", "body": map[string]any{"data": payloadText}},
							{
								"filename": "file.txt",
								"mimeType": "text/plain",
								"body":     map[string]any{"attachmentId": "att1", "size": 10},
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	var out bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	if err := runKong(t, &GmailDraftsGetCmd{}, []string{"d1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(out.String(), "Draft-ID:") || !strings.Contains(out.String(), "Subject:") {
		t.Fatalf("unexpected output: %q", out.String())
	}
	if !strings.Contains(out.String(), "Attachments:") || !strings.Contains(out.String(), "file.txt") {
		t.Fatalf("expected attachment output: %q", out.String())
	}
	if !strings.Contains(out.String(), "attachment\tfile.txt\t10 B\ttext/plain\tatt1") {
		t.Fatalf("expected attachment line output: %q", out.String())
	}
}

func TestGmailDraftsDeleteCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com", Force: true}

	var jsonOut bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
	if err := runKong(t, &GmailDraftsDeleteCmd{}, []string{"d1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var parsed struct {
		Deleted bool   `json:"deleted"`
		DraftID string `json:"draftId"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if !parsed.Deleted || parsed.DraftID != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsCreateCmd_InvalidHeadersAreUsageErrorsBeforeDryRun(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com", DryRun: true}

	for _, cmd := range []GmailDraftsCreateCmd{
		{To: "bad\ncc:evil@example.com", Subject: "S", Body: "B"},
		{To: "a@example.com", ReplyTo: "bad\ncc:evil@example.com", Subject: "S", Body: "B"},
		{To: "a@example.com", Subject: "S\nInjected: yes", Body: "B"},
	} {
		err := cmd.Run(ctx, flags)
		if err == nil {
			t.Fatal("expected invalid header error")
		}
		if got := ExitCode(err); got != 2 {
			t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
		}
	}
}

func TestGmailDraftsUpdateCmd_InvalidHeadersAreUsageErrorsBeforeDryRun(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com", DryRun: true}
	validTo := "a@example.com"
	badTo := "bad\ncc:evil@example.com"

	for _, cmd := range []GmailDraftsUpdateCmd{
		{DraftID: "d1", To: &badTo, Subject: "S", Body: "B"},
		{DraftID: "d1", To: &validTo, ReplyTo: "bad\ncc:evil@example.com", Subject: "S", Body: "B"},
		{DraftID: "d1", To: &validTo, Subject: "S\nInjected: yes", Body: "B"},
	} {
		err := cmd.Run(ctx, flags)
		if err == nil {
			t.Fatal("expected invalid header error")
		}
		if got := ExitCode(err); got != 2 {
			t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
		}
	}
}

func TestGmailDraftsSendCmd_Text(t *testing.T) {
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

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	var out bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	if err := runKong(t, &GmailDraftsSendCmd{}, []string{"d1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(out.String(), "message_id\tm1") || !strings.Contains(out.String(), "thread_id\tt1") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestGmailDraftsCreateCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	var jsonOut bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
	if err := runKong(t, &GmailDraftsCreateCmd{}, []string{"--to", "a@example.com", "--subject", "S", "--body", "Hello"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var parsed struct {
		DraftID  string `json:"draftId"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.DraftID != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsCreateCmd_BodyHTMLFile(t *testing.T) {
	htmlPath := filepath.Join(t.TempDir(), "body.html")
	if err := os.WriteFile(htmlPath, []byte("<h1>Hello</h1>\n<p>from file</p>"), 0o600); err != nil {
		t.Fatalf("write html: %v", err)
	}

	var rawCreated string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost {
			var draft gmail.Draft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Fatalf("decode draft: %v", err)
			}
			if draft.Message == nil {
				t.Fatalf("expected message in create")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			rawCreated = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d-html",
				"message": map[string]any{"id": "m-html"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--to", "a@example.com",
		"--subject", "Hello",
		"--body-html-file", htmlPath,
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(rawCreated, "Content-Type: text/html") ||
		!strings.Contains(rawCreated, "<h1>Hello</h1>") {
		t.Fatalf("expected HTML file body in created draft, got:\n%s", rawCreated)
	}
}

func TestGmailDraftsCreateCmd_BodyHTMLFileConflict(t *testing.T) {
	cmd := &GmailDraftsCreateCmd{
		To:           "a@example.com",
		Subject:      "Hello",
		BodyHTML:     "<p>inline</p>",
		BodyHTMLFile: "/tmp/body.html",
	}

	err := cmd.Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "--body-html") || !strings.Contains(err.Error(), "--body-html-file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailDraftsCreateCmd_NoTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in create")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if strings.Contains(s, "\r\nTo:") {
				t.Fatalf("unexpected To header in raw:\n%s", s)
			}
			if !strings.Contains(s, "Subject: S\r\n") {
				t.Fatalf("missing Subject in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m1",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsCreateCmd{}, []string{"--subject", "S", "--body", "Hello"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestGmailDraftsCreateCmd_WithFromAndReply(t *testing.T) {
	attachPath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(attachPath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write attach: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/settings/sendAs/alias@example.com") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "Alias",
				"verificationStatus": "accepted",
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("format"); got != gmailFormatMetadata {
				t.Fatalf("expected format=%s, got %q", gmailFormatMetadata, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<msg@id>"},
						{"name": "References", "value": "<ref@id>"},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m2",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var jsonOut bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
	if err := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--to", "a@example.com",
		"--subject", "S",
		"--body", "Hello",
		"--from", "alias@example.com",
		"--reply-to-message-id", "m1",
		"--attach", attachPath,
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var parsed struct {
		Attachments []mailmime.AttachmentMetadata `json:"attachments"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].Filename != "note.txt" || parsed.Attachments[0].Size != 5 {
		t.Fatalf("unexpected attachment metadata: %#v", parsed.Attachments)
	}
}

func TestGmailDraftsCreateCmd_WithQuote(t *testing.T) {
	originalPlain := "Original plain line"
	originalHTML := "<p>Original <b>HTML</b></p>"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"mimeType": "multipart/alternative",
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<msg@id>"},
						{"name": "References", "value": "<ref@id>"},
						{"name": "From", "value": "Alice <alice@example.com>"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte(originalPlain)),
							},
						},
						{
							"mimeType": "text/html",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte(originalHTML)),
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in create")
			}
			if draft.Message.ThreadId != "t1" {
				t.Fatalf("expected threadId t1, got %q", draft.Message.ThreadId)
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "Hello reply") {
				t.Fatalf("missing body in raw:\n%s", s)
			}
			if !strings.Contains(s, "On Mon, 1 Jan 2024 00:00:00 +0000, Alice <alice@example.com> wrote:") {
				t.Fatalf("missing quoted attribution in raw:\n%s", s)
			}
			if !strings.Contains(s, "> Original plain line") {
				t.Fatalf("missing quoted plain body in raw:\n%s", s)
			}
			if !strings.Contains(s, "gmail_quote") {
				t.Fatalf("missing quoted html block in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id":       "m2",
					"threadId": "t1",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--to", "a@example.com",
		"--subject", "S",
		"--body", "Hello reply",
		"--reply-to-message-id", "m1",
		"--quote",
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestGmailDraftsCreateCmd_WithFromWorkspaceAliasNoVerificationStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/settings/sendAs/workspace-alias@example.com") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail": "workspace-alias@example.com",
				"displayName": "Workspace Alias",
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in create draft request")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			if !strings.Contains(string(raw), "From: \"Workspace Alias\" <workspace-alias@example.com>\r\n") {
				t.Fatalf("missing workspace alias From header in raw:\n%s", string(raw))
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d-workspace",
				"message": map[string]any{
					"id": "m-workspace",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--to", "a@example.com",
		"--subject", "S",
		"--body", "Hello",
		"--from", "workspace-alias@example.com",
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestGmailDraftsUpdateCmd_JSON(t *testing.T) {
	attData := []byte("attachment")
	attachPath := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(attachPath, attData, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m1", "threadId": "t1"},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("format"); got != gmailFormatMetadata {
				t.Fatalf("expected format=%s, got %q", gmailFormatMetadata, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1@example.com>"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "From: a@b.com\r\n") {
				t.Fatalf("missing From in raw:\n%s", s)
			}
			if !strings.Contains(s, "To: a@example.com\r\n") {
				t.Fatalf("missing To in raw:\n%s", s)
			}
			if !strings.Contains(s, "Cc: cc@example.com\r\n") {
				t.Fatalf("missing Cc in raw:\n%s", s)
			}
			if !strings.Contains(s, "Bcc: bcc@example.com\r\n") {
				t.Fatalf("missing Bcc in raw:\n%s", s)
			}
			if !strings.Contains(s, "Subject: Updated\r\n") {
				t.Fatalf("missing Subject in raw:\n%s", s)
			}
			if !strings.Contains(s, "Reply-To: reply@example.com\r\n") {
				t.Fatalf("missing Reply-To in raw:\n%s", s)
			}
			if !strings.Contains(s, "Hello") {
				t.Fatalf("missing body in raw:\n%s", s)
			}
			if !strings.Contains(s, "Content-Disposition: attachment; filename=\"note.txt\"") {
				t.Fatalf("missing attachment header in raw:\n%s", s)
			}
			if !strings.Contains(s, base64.StdEncoding.EncodeToString(attData)) {
				t.Fatalf("missing attachment data in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	var jsonOut bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard), svc)
	if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--to", "a@example.com",
		"--cc", "cc@example.com",
		"--bcc", "bcc@example.com",
		"--subject", "Updated",
		"--body", "Hello",
		"--reply-to", "reply@example.com",
		"--attach", attachPath,
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var parsed struct {
		DraftID     string                        `json:"draftId"`
		ThreadID    string                        `json:"threadId"`
		Attachments []mailmime.AttachmentMetadata `json:"attachments"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.DraftID != "d1" || parsed.ThreadID != "t1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].Filename != "note.txt" || parsed.Attachments[0].Size != int64(len(attData)) {
		t.Fatalf("unexpected attachment metadata: %#v", parsed.Attachments)
	}
}

func TestGmailDraftsUpdateCmd_BodyHTMLFileFromStdin(t *testing.T) {
	var rawUpdated string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m1", "threadId": "t1"},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1@example.com>"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			var draft gmail.Draft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Fatalf("decode draft: %v", err)
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			rawUpdated = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	ctx := withGmailTestService(
		newCmdRuntimeIOContext(t, strings.NewReader("<h1>Updated</h1>\n<p>from stdin</p>"), io.Discard, io.Discard),
		svc,
	)
	if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--to", "a@example.com",
		"--subject", "Updated",
		"--body-html-file", "-",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(rawUpdated, "Content-Type: text/html") ||
		!strings.Contains(rawUpdated, "<h1>Updated</h1>") {
		t.Fatalf("expected HTML stdin body in updated draft, got:\n%s", rawUpdated)
	}
}

func TestGmailDraftsUpdateCmd_BodyHTMLFileConflict(t *testing.T) {
	validTo := "a@example.com"
	cmd := &GmailDraftsUpdateCmd{
		DraftID:      "d1",
		To:           &validTo,
		Subject:      "Hello",
		BodyHTML:     "<p>inline</p>",
		BodyHTMLFile: "/tmp/body.html",
	}

	err := cmd.Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "--body-html") || !strings.Contains(err.Error(), "--body-html-file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailDraftsUpdateCmd_PreservesToWhenNotProvided(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id":       "m1",
					"threadId": "t1",
					"payload": map[string]any{
						"headers": []map[string]any{
							{"name": "To", "value": "keep@example.com"},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1@example.com>"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "To: keep@example.com\r\n") {
				t.Fatalf("expected preserved To in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--subject", "Updated",
		"--body", "Hello",
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestGmailDraftsUpdateCmd_WithQuoteFromExistingThread(t *testing.T) {
	originalPlain := "Original thread message"
	originalHTML := "<div>Original <i>thread</i> HTML</div>"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id":       "m-draft",
					"threadId": "t1",
					"payload": map[string]any{
						"headers": []map[string]any{
							{"name": "To", "value": "keep@example.com"},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":           "m1",
						"threadId":     "t1",
						"internalDate": "1000",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1@example.com>"},
								{"name": "From", "value": "Bob <bob@example.com>"},
							},
						},
					},
					{
						"id":           "m-self",
						"threadId":     "t1",
						"internalDate": "3000",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m-self@example.com>"},
								{"name": "From", "value": "a@b.com"},
							},
						},
					},
					{
						"id":           "m-draft",
						"threadId":     "t1",
						"internalDate": "4000",
						"labelIds":     []string{"DRAFT"},
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m-draft@example.com>"},
								{"name": "From", "value": "a@b.com"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("format"); got != gmailFormatFull {
				t.Fatalf("expected format=%s, got %q", gmailFormatFull, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"mimeType": "multipart/alternative",
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<m1@example.com>"},
						{"name": "References", "value": "<ref@example.com>"},
						{"name": "From", "value": "Bob <bob@example.com>"},
						{"name": "Date", "value": "Tue, 2 Jan 2024 03:04:05 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte(originalPlain)),
							},
						},
						{
							"mimeType": "text/html",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte(originalHTML)),
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") && r.Method == http.MethodGet:
			t.Fatalf("unexpected message fetch path: %s", r.URL.Path)
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			if draft.Message.ThreadId != "t1" {
				t.Fatalf("expected threadId t1, got %q", draft.Message.ThreadId)
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "To: keep@example.com\r\n") {
				t.Fatalf("missing preserved To in raw:\n%s", s)
			}
			if !strings.Contains(s, "Updated body") {
				t.Fatalf("missing body in raw:\n%s", s)
			}
			if !strings.Contains(s, "On Tue, 2 Jan 2024 03:04:05 +0000, Bob <bob@example.com> wrote:") {
				t.Fatalf("missing quoted attribution in raw:\n%s", s)
			}
			if !strings.Contains(s, "> Original thread message") {
				t.Fatalf("missing quoted plain body in raw:\n%s", s)
			}
			if !strings.Contains(s, "gmail_quote") {
				t.Fatalf("missing quoted html block in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--subject", "Updated",
		"--body", "Updated body",
		"--quote",
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestGmailDraftsUpdateCmd_QuoteRequiresNonDraftNonSelfThreadMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id":       "m-draft",
					"threadId": "t1",
					"payload": map[string]any{
						"headers": []map[string]any{
							{"name": "To", "value": "keep@example.com"},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":           "m-self",
						"threadId":     "t1",
						"internalDate": "3000",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m-self@example.com>"},
								{"name": "From", "value": "a@b.com"},
							},
						},
					},
					{
						"id":           "m-draft",
						"threadId":     "t1",
						"internalDate": "4000",
						"labelIds":     []string{"DRAFT"},
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m-draft@example.com>"},
								{"name": "From", "value": "a@b.com"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") && r.Method == http.MethodGet:
			t.Fatalf("unexpected message fetch path: %s", r.URL.Path)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--subject", "Updated",
		"--body", "Updated body",
		"--quote",
	}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--quote requires --reply-to-message-id or existing draft thread with a non-draft, non-self message") {
		t.Fatalf("expected quote target validation error, got %v", err)
	}
}

func TestGmailDraftsUpdateCmd_WithQuoteAndReplyToMessageID(t *testing.T) {
	originalPlain := "Quoted from explicit message id"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("format"); got != gmailFormatFull {
				t.Fatalf("expected format=%s, got %q", gmailFormatFull, got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"mimeType": "multipart/alternative",
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<m1@example.com>"},
						{"name": "References", "value": "<ref@example.com>"},
						{"name": "From", "value": "Carol <carol@example.com>"},
						{"name": "Date", "value": "Wed, 3 Jan 2024 06:07:08 +0000"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte(originalPlain)),
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			// Update now reads the existing draft to preserve attachments; this
			// draft has none, so preservation is a no-op.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "dm1", "threadId": "t1"},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			var draft gmail.Draft
			if unmarshalErr := json.Unmarshal(body, &draft); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v body=%q", unmarshalErr, string(body))
			}
			if draft.Message == nil {
				t.Fatalf("expected message in update")
			}
			if draft.Message.ThreadId != "t1" {
				t.Fatalf("expected threadId t1, got %q", draft.Message.ThreadId)
			}
			raw, err := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			s := string(raw)
			if !strings.Contains(s, "To: keep@example.com\r\n") {
				t.Fatalf("missing To in raw:\n%s", s)
			}
			if !strings.Contains(s, "Updated body") {
				t.Fatalf("missing body in raw:\n%s", s)
			}
			if !strings.Contains(s, "On Wed, 3 Jan 2024 06:07:08 +0000, Carol <carol@example.com> wrote:") {
				t.Fatalf("missing quoted attribution in raw:\n%s", s)
			}
			if !strings.Contains(s, "> Quoted from explicit message id") {
				t.Fatalf("missing quoted plain body in raw:\n%s", s)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m2", "threadId": "t1"},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}

	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--to", "keep@example.com",
		"--subject", "Updated",
		"--body", "Updated body",
		"--reply-to-message-id", "m1",
		"--quote",
	}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestGmailDraftsUpdateCmd_QuoteRequiresReplyContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "m-draft",
					"payload": map[string]any{
						"headers": []map[string]any{
							{"name": "To", "value": "keep@example.com"},
						},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--subject", "Updated",
		"--body", "Updated body",
		"--quote",
	}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--quote requires --reply-to-message-id or existing draft thread") {
		t.Fatalf("expected quote/reply context validation error, got %v", err)
	}
}

// --thread-id on drafts create anchors In-Reply-To/References to the thread's
// latest message and stamps the draft's threadId (parity with `gmail send`).
func TestGmailDraftsCreateCmd_WithThreadID(t *testing.T) {
	var posted gmail.Draft
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id": "old", "threadId": "t1", "internalDate": "1000",
						"payload": map[string]any{"headers": []map[string]any{
							{"name": "Message-ID", "value": "<old@id>"},
						}},
					},
					{
						"id": "latest", "threadId": "t1", "internalDate": "2000",
						"payload": map[string]any{"headers": []map[string]any{
							{"name": "Message-ID", "value": "<latest@id>"},
							{"name": "References", "value": "<r1@id>"},
						}},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts") && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if unmarshalErr := json.Unmarshal(body, &posted); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v", unmarshalErr)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m2", "threadId": "t1"}})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if runErr := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--to", "a@example.com", "--subject", "Re: hi", "--body", "Hello", "--thread-id", "t1",
	}, ctx, flags); runErr != nil {
		t.Fatalf("execute: %v", runErr)
	}

	if posted.Message == nil {
		t.Fatal("no draft posted")
	}
	if posted.Message.ThreadId != "t1" {
		t.Fatalf("expected draft threadId t1, got %q", posted.Message.ThreadId)
	}
	raw, decErr := base64.RawURLEncoding.DecodeString(posted.Message.Raw)
	if decErr != nil {
		t.Fatalf("decode raw: %v", decErr)
	}
	s := string(raw)
	if !strings.Contains(s, "In-Reply-To: <latest@id>") {
		t.Fatalf("In-Reply-To not anchored to latest thread message:\n%s", s)
	}
	if !strings.Contains(s, "References:") || !strings.Contains(s, "<latest@id>") {
		t.Fatalf("References not built from latest thread message:\n%s", s)
	}
}

// --thread-id and --reply-to-message-id are mutually exclusive on drafts create.
func TestGmailDraftsCreateCmd_ThreadIDAndMessageIDMutuallyExclusive(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	err := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--subject", "S", "--body", "B", "--reply-to-message-id", "m1", "--thread-id", "t1",
	}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "use only one of --reply-to-message-id or --thread-id") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

// A caller-provided --thread-id on drafts update overrides the draft's own
// existing thread when anchoring reply headers.
func TestGmailDraftsUpdateCmd_WithThreadIDOverridesExisting(t *testing.T) {
	var posted gmail.Draft
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "m1", "threadId": "te"},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id": "latest", "threadId": "t1", "internalDate": "2000",
						"payload": map[string]any{"headers": []map[string]any{
							{"name": "Message-ID", "value": "<latest@id>"},
							{"name": "Subject", "value": "Original"},
						}},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			if unmarshalErr := json.Unmarshal(body, &posted); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v", unmarshalErr)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m2", "threadId": "t1"}})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if runErr := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1", "--to", "a@example.com", "--body", "Hello", "--thread-id", "t1",
	}, ctx, flags); runErr != nil {
		t.Fatalf("execute: %v", runErr)
	}

	if posted.Message == nil {
		t.Fatal("no draft posted")
	}
	if posted.Message.ThreadId != "t1" {
		t.Fatalf("expected caller thread t1 to override existing te, got %q", posted.Message.ThreadId)
	}
	raw, decErr := base64.RawURLEncoding.DecodeString(posted.Message.Raw)
	if decErr != nil {
		t.Fatalf("decode raw: %v", decErr)
	}
	if !strings.Contains(string(raw), "In-Reply-To: <latest@id>") {
		t.Fatalf("In-Reply-To not anchored to caller thread's latest message:\n%s", string(raw))
	}
	if !strings.Contains(string(raw), "Subject: Re: Original") {
		t.Fatalf("subject not auto-filled from caller thread:\n%s", string(raw))
	}
}

// draftUpdateAttachmentServer stubs a draft (with one attachment) get, the
// attachment bytes endpoint, and the update PUT (capturing the posted draft).
func draftUpdateAttachmentServer(t *testing.T, posted *gmail.Draft, attBytesHit *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id": "dm1",
					"payload": map[string]any{
						"mimeType": "multipart/mixed",
						"headers":  []map[string]any{{"name": "To", "value": "keep@example.com"}},
						"parts": []map[string]any{
							{"mimeType": "text/plain", "body": map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("old body"))}},
							{"filename": "report.pdf", "mimeType": "application/pdf", "body": map[string]any{"attachmentId": "att1", "size": 5}},
							{"filename": "empty.bin", "mimeType": "application/octet-stream", "body": map[string]any{"attachmentId": "att-empty", "size": 0}},
							{"filename": "inline.txt", "mimeType": "text/plain", "body": map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("INLINE")), "size": 6}},
							{"filename": "zero.txt", "mimeType": "text/plain", "body": map[string]any{"data": "", "size": 0}},
						},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/dm1/attachments/att1") && r.Method == http.MethodGet:
			*attBytesHit = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("HELLO")), "size": 5})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/dm1/attachments/att-empty") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "", "size": 0})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			if unmarshalErr := json.Unmarshal(body, posted); unmarshalErr != nil {
				t.Fatalf("unmarshal: %v", unmarshalErr)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m2", "threadId": "t1"}})
		default:
			http.NotFound(w, r)
		}
	}))
}

func runDraftUpdate(t *testing.T, srv *httptest.Server, args []string) string {
	t.Helper()
	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	if runErr := runKong(t, &GmailDraftsUpdateCmd{}, args, ctx, flags); runErr != nil {
		t.Fatalf("execute: %v", runErr)
	}
	return out.String()
}

// Omitting --attach on update preserves the draft's existing attachments.
func TestGmailDraftsUpdateCmd_PreservesAttachmentsWhenOmitted(t *testing.T) {
	var posted gmail.Draft
	attBytesHit := false
	srv := draftUpdateAttachmentServer(t, &posted, &attBytesHit)
	defer srv.Close()

	jsonOut := runDraftUpdate(t, srv, []string{"d1", "--to", "keep@example.com", "--subject", "S", "--body", "new body"})

	if !attBytesHit {
		t.Fatal("expected attachment bytes to be fetched for preservation")
	}
	raw, err := base64.RawURLEncoding.DecodeString(posted.Message.Raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, `filename="report.pdf"`) {
		t.Fatalf("preserved attachment filename missing from rebuilt draft:\n%s", s)
	}
	if !strings.Contains(s, base64.StdEncoding.EncodeToString([]byte("HELLO"))) {
		t.Fatalf("preserved attachment bytes missing from rebuilt draft:\n%s", s)
	}
	if !strings.Contains(s, `filename="empty.bin"`) {
		t.Fatalf("zero-byte fetched attachment filename missing from rebuilt draft:\n%s", s)
	}
	if !strings.Contains(s, `filename="inline.txt"`) {
		t.Fatalf("inline attachment filename missing from rebuilt draft:\n%s", s)
	}
	if !strings.Contains(s, base64.StdEncoding.EncodeToString([]byte("INLINE"))) {
		t.Fatalf("inline attachment bytes missing from rebuilt draft:\n%s", s)
	}
	if !strings.Contains(s, `filename="zero.txt"`) {
		t.Fatalf("zero-byte inline attachment filename missing from rebuilt draft:\n%s", s)
	}
	var parsed struct {
		Attachments []mailmime.AttachmentMetadata `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	want := []mailmime.AttachmentMetadata{
		{Filename: "report.pdf", Size: 5},
		{Filename: "empty.bin", Size: 0},
		{Filename: "inline.txt", Size: 6},
		{Filename: "zero.txt", Size: 0},
	}
	if len(parsed.Attachments) != len(want) {
		t.Fatalf("unexpected attachment metadata: %#v", parsed.Attachments)
	}
	for i := range want {
		if parsed.Attachments[i] != want[i] {
			t.Fatalf("attachments[%d] = %#v, want %#v", i, parsed.Attachments[i], want[i])
		}
	}
}

// --clear-attachments drops the draft's existing attachments and skips the byte fetch.
func TestGmailDraftsUpdateCmd_ClearAttachmentsRemovesThem(t *testing.T) {
	var posted gmail.Draft
	attBytesHit := false
	srv := draftUpdateAttachmentServer(t, &posted, &attBytesHit)
	defer srv.Close()

	jsonOut := runDraftUpdate(t, srv, []string{"d1", "--to", "keep@example.com", "--subject", "S", "--body", "new body", "--clear-attachments"})

	if attBytesHit {
		t.Fatal("did not expect attachment bytes to be fetched when clearing")
	}
	raw, err := base64.RawURLEncoding.DecodeString(posted.Message.Raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if strings.Contains(string(raw), "report.pdf") {
		t.Fatalf("attachment should have been cleared:\n%s", string(raw))
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if _, ok := parsed["attachments"]; ok {
		t.Fatalf("attachments should be omitted after clear: %s", jsonOut)
	}
}

// --attach and --clear-attachments are mutually exclusive.
func TestGmailDraftsUpdateCmd_AttachAndClearMutuallyExclusive(t *testing.T) {
	attachPath := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(attachPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write attach: %v", err)
	}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@b.com"}
	err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1", "--subject", "S", "--body", "B", "--attach", attachPath, "--clear-attachments",
	}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "use only one of --attach or --clear-attachments") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}
