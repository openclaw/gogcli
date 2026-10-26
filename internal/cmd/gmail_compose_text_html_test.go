package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func TestComposeTextHTMLOptions_WithTextHTML(t *testing.T) {
	on := composeTextHTMLOptions{HTMLFromText: true}
	tests := []struct {
		name  string
		opts  composeTextHTMLOptions
		plain string
		html  string
		want  string
	}{
		{"off keeps plain-only", composeTextHTMLOptions{}, "Hi,\n\nOne paragraph.", "", ""},
		{"on builds escaped html with line breaks", on, "Hi,\n\nA & <b> line.", "", "<div dir=\"auto\">Hi,</div>\n<div dir=\"auto\"><br></div>\n<div dir=\"auto\">A &amp; &lt;b&gt; line.</div>"},
		{"on directs mixed-language lines", on, "Hello\nשלום", "", "<div dir=\"auto\">Hello</div>\n<div dir=\"auto\">שלום</div>"},
		{"on keeps a given html body", on, "Hi", "<p>Hi</p>", "<p>Hi</p>"},
		{"on ignores an empty plain body", on, " \n", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.withTextHTML(tt.plain, tt.html); got != tt.want {
				t.Fatalf("withTextHTML(%q, %q) = %q, want %q", tt.plain, tt.html, got, tt.want)
			}
		})
	}
}

func sendRawForTextHTMLTest(t *testing.T, cmd *GmailSendCmd) string {
	t.Helper()
	var rawSent string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method == http.MethodPost && path == "/users/me/messages/send" {
			var msg gmail.Message
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				t.Fatalf("decode sent message: %v", err)
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw message: %v", err)
			}
			rawSent = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "threadId": "t1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	_ = runGmailSendJSON(t, cmd, svc, nil)
	return rawSent
}

func TestGmailSendCmd_HTMLFromTextAddsHTMLAlternative(t *testing.T) {
	body := "Hi,\n\nOne long paragraph that should reflow in Gmail web instead of breaking every seventy characters.\n\nThanks"
	raw := sendRawForTextHTMLTest(t, &GmailSendCmd{
		To:                     "a@example.com",
		Subject:                "Hello",
		Body:                   body,
		composeTextHTMLOptions: composeTextHTMLOptions{HTMLFromText: true},
	})
	if !strings.Contains(raw, "multipart/alternative") ||
		!strings.Contains(raw, "Content-Type: text/plain") ||
		!strings.Contains(raw, "Content-Type: text/html") {
		t.Fatalf("expected text/plain and text/html alternatives, got:\n%s", raw)
	}
	if !strings.Contains(raw, "One long paragraph that should reflow in Gmail web instead of breaking every seventy characters.</div>") {
		t.Fatalf("expected the paragraph as one directed HTML line, got:\n%s", raw)
	}
}

func TestGmailSendCmd_PlainBodyStaysPlainByDefault(t *testing.T) {
	raw := sendRawForTextHTMLTest(t, &GmailSendCmd{To: "a@example.com", Subject: "Hello", Body: "Hi,\n\nPlain."})
	if strings.Contains(raw, "text/html") {
		t.Fatalf("expected no HTML part without --html-from-text, got:\n%s", raw)
	}
}

func TestGmailDraftsCreateCmd_HTMLFromTextFlagAndEnv(t *testing.T) {
	raw := captureDraftCreateRaw(t, []string{
		"--to", "a@example.com", "--subject", "Hello", "--body", "Hi,\n\nA draft paragraph.", "--html-from-text",
	})
	if !strings.Contains(raw, "Content-Type: text/html") || !strings.Contains(raw, `<div dir="auto">Hi,</div>`) {
		t.Fatalf("expected an HTML alternative from --html-from-text, got:\n%s", raw)
	}

	t.Setenv("GOG_GMAIL_HTML_FROM_TEXT", "1")
	raw = captureDraftCreateRaw(t, []string{"--to", "a@example.com", "--subject", "Hello", "--body", "Hi,\n\nFrom env."})
	if !strings.Contains(raw, "Content-Type: text/html") || !strings.Contains(raw, "From env.") {
		t.Fatalf("expected an HTML alternative from GOG_GMAIL_HTML_FROM_TEXT, got:\n%s", raw)
	}
}

func TestExecuteGmailReplyHTMLFromText(t *testing.T) {
	var rawSent string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{
				{"sendAsEmail": "me@example.com", "isPrimary": true, "verificationStatus": "accepted"},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-3":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg-3",
				"threadId": "thread-3",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<original-3@example.com>"},
						{"name": "From", "value": "alice@example.com"},
						{"name": "To", "value": "me@example.com"},
						{"name": "Subject", "value": "Original"},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
			var msg gmail.Message
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				t.Fatalf("decode sent message: %v", err)
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw message: %v", err)
			}
			rawSent = string(raw)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-3", "threadId": "thread-3"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "reply", "msg-3",
		"--body", "Thanks,\n\nSee you then.",
		"--no-quote",
		"--html-from-text",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(rawSent, "multipart/alternative") ||
		!strings.Contains(rawSent, "Content-Type: text/html") ||
		!strings.Contains(rawSent, `<div dir="auto">Thanks,</div>`) {
		t.Fatalf("expected the reply to carry an HTML alternative, got:\n%s", rawSent)
	}
}

func TestGmailRawModeHTMLFromTextEnvironment(t *testing.T) {
	t.Setenv("GOG_GMAIL_HTML_FROM_TEXT", "1")
	sendPath := writeRawSendEML(t)
	draftPath := writeRawDraftFixture(t, testRawDraftEML)
	for name, args := range map[string][]string{
		"send":   {"--dry-run", "--json", "gmail", "send", "--raw-file", sendPath},
		"create": append(rawDraftArgs("create", draftPath), "--dry-run", "--json"),
		"update": append(rawDraftArgs("update", draftPath), "--dry-run", "--json"),
	} {
		t.Run(name, func(t *testing.T) {
			result := executeWithTestRuntime(t, args, &app.Runtime{})
			if result.err == nil || !strings.Contains(result.err.Error(), "--html-from-text") {
				t.Fatalf("GOG_GMAIL_HTML_FROM_TEXT should conflict with --raw-file: %v", result.err)
			}
			result = executeWithTestRuntime(t, append(args, "--html-from-text=false"), &app.Runtime{})
			if result.err != nil {
				t.Fatalf("explicit false should allow --raw-file: %v", result.err)
			}
		})
	}
}

func TestGmailDraftsUpdate_HTMLFromTextPreservesPlainBody(t *testing.T) {
	const body = "A & <b> paragraph that is longer than seventy characters and must stay one logical line.\n\nNext line."
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "generated", true: "explicit HTML"}[explicit], func(t *testing.T) {
			srv, raw := newDraftUpdateCaptureServer(t, "a@example.com")
			defer srv.Close()
			ctx := withGmailTestService(newCmdRuntimeIOContext(t, nil, io.Discard, io.Discard), newGmailServiceFromServer(t, srv))
			args := []string{"d1", "--subject", "Updated", "--body", body, "--html-from-text"}
			wantHTML := directedHTML(directedTextToHTML(body))
			if explicit {
				const explicitHTML = "<p>Explicit HTML</p>"
				wantHTML = directedHTML(explicitHTML)
				args = append(args, "--body-html", explicitHTML)
			}
			if err := runKong(t, &GmailDraftsUpdateCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			msg, err := mail.ReadMessage(strings.NewReader(*raw))
			if err != nil {
				t.Fatal(err)
			}
			media, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
			if err != nil || media != "multipart/alternative" {
				t.Fatalf("media=%q err=%v", media, err)
			}
			reader := multipart.NewReader(msg.Body, params["boundary"])
			for _, want := range []string{body, wantHTML} {
				part, err := reader.NextPart()
				if err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(part)
				if err != nil {
					t.Fatal(err)
				}
				if strings.ReplaceAll(string(got), "\r\n", "\n") != want {
					t.Fatalf("body=%q, want %q", got, want)
				}
			}
		})
	}
}
