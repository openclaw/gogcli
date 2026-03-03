package cmd

import (
	"context"
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
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailDraftsListCmd_TextAndJSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailDraftsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(textOut, "ID") || !strings.Contains(textOut, "d1") {
		t.Fatalf("unexpected text: %q", textOut)
	}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailDraftsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Drafts []struct {
			ID        string `json:"id"`
			MessageID string `json:"messageId"`
			ThreadID  string `json:"threadId"`
		} `json:"drafts"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Drafts) != 2 || parsed.Drafts[0].ID != "d1" || parsed.NextPageToken != "next" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsGetCmd_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailDraftsGetCmd{}
		if err := runKong(t, cmd, []string{"d1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "Draft-ID:") || !strings.Contains(out, "Subject:") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "Attachments:") || !strings.Contains(out, "file.txt") {
		t.Fatalf("expected attachment output: %q", out)
	}
	if !strings.Contains(out, "attachment\tfile.txt\t10 B\ttext/plain\tatt1") {
		t.Fatalf("expected attachment line output: %q", out)
	}
}

func TestGmailDraftsDeleteCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailDraftsDeleteCmd{}
		if err := runKong(t, cmd, []string{"d1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Deleted bool   `json:"deleted"`
		DraftID string `json:"draftId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if !parsed.Deleted || parsed.DraftID != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsSendCmd_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailDraftsSendCmd{}
		if err := runKong(t, cmd, []string{"d1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "message_id\tm1") || !strings.Contains(out, "thread_id\tt1") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGmailDraftsCreateCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsCreateCmd{}, []string{"--to", "a@example.com", "--subject", "S", "--body", "Hello"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		DraftID  string `json:"draftId"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.DraftID != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsCreateCmd_NoTo(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsCreateCmd{}, []string{"--subject", "S", "--body", "Hello"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

func TestGmailDraftsCreateCmd_WithFromAndReply(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
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
	})
}

func TestGmailDraftsCreateCmd_WithQuote(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsCreateCmd{}, []string{
			"--to", "a@example.com",
			"--subject", "S",
			"--body", "Hello reply",
			"--reply-to-message-id", "m1",
			"--quote",
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

func TestGmailDraftsUpdateCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

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
	})

	var parsed struct {
		DraftID  string `json:"draftId"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.DraftID != "d1" || parsed.ThreadID != "t1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestGmailDraftsUpdateCmd_PreservesToWhenNotProvided(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
			"d1",
			"--subject", "Updated",
			"--body", "Hello",
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

func TestGmailDraftsUpdateCmd_WithQuoteFromExistingThread(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		if err := runKong(t, &GmailDraftsUpdateCmd{}, []string{
			"d1",
			"--subject", "Updated",
			"--body", "Updated body",
			"--quote",
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

func TestGmailDraftsUpdateCmd_QuoteRequiresNonDraftNonSelfThreadMessage(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = runKong(t, &GmailDraftsUpdateCmd{}, []string{
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
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

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
	})
}

func TestGmailDraftsUpdateCmd_QuoteRequiresReplyContext(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = runKong(t, &GmailDraftsUpdateCmd{}, []string{
		"d1",
		"--subject", "Updated",
		"--body", "Updated body",
		"--quote",
	}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--quote requires --reply-to-message-id or existing draft thread") {
		t.Fatalf("expected quote/reply context validation error, got %v", err)
	}
}
