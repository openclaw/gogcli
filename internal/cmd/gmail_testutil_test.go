package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
)

// mockReplySourceMessage returns a gmail.Message JSON payload suitable as the
// target of a reply: it has From/To/Cc/Subject/Message-ID headers and a plain
// text body.
func mockReplySourceMessage() map[string]any {
	plain := base64.RawURLEncoding.EncodeToString([]byte("Original plain body."))
	return map[string]any{
		"id":       "msg-1",
		"threadId": "thread-1",
		"payload": map[string]any{
			"mimeType": "text/plain",
			"headers": []map[string]any{
				{"name": "Message-ID", "value": "<original@example.com>"},
				{"name": "References", "value": "<root@example.com>"},
				{"name": "From", "value": `"Alice Sender" <alice@example.com>`},
				{"name": "To", "value": `"Me Person" <me@example.com>, "Other Person" <other@example.com>`},
				{"name": "Cc", "value": `"CC Person" <cc@example.com>`},
				{"name": "Date", "value": "Fri, 12 Jun 2026 10:00:00 +0000"},
				{"name": "Subject", "value": "Project update"},
			},
			"body": map[string]any{"data": plain, "size": len(plain)},
		},
	}
}

func mockForwardSourceMessage() map[string]any {
	return mockOriginalMessage(false)
}

func sendAsListHandler(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sendAs": []map[string]any{
			{"sendAsEmail": "me@example.com", "displayName": "Me Person", "isPrimary": true, "verificationStatus": "accepted"},
			{"sendAsEmail": "alias@example.com", "displayName": "Alias", "verificationStatus": "accepted"},
		},
	})
}

// writeDraftCreatedResponse writes the canned Drafts.Create response shared by
// the drafts compose tests.
func writeDraftCreatedResponse(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
}

// handleFinalizeRaw services the finalize POST shared by the send and draft
// paths: it decodes the outgoing message (from a Draft body when finalizePath is
// the drafts endpoint, otherwise from a bare Message), writes the canned finalize
// response, and returns the decoded RFC822 raw plus the stamped ThreadId.
func handleFinalizeRaw(t *testing.T, w http.ResponseWriter, r *http.Request, finalizePath string) (raw, threadID string) {
	t.Helper()
	var msg *gmail.Message
	if finalizePath == "/gmail/v1/users/me/drafts" {
		var draft gmail.Draft
		if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
			t.Fatalf("decode draft: %v", err)
		}
		msg = draft.Message
		writeDraftCreatedResponse(w)
	} else {
		var m gmail.Message
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode send: %v", err)
		}
		msg = &m
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-1", "threadId": "thread-1"})
	}
	if msg == nil {
		t.Fatalf("nil message in finalize body")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(msg.Raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	return string(decoded), msg.ThreadId
}

// captureComposeRaw runs a compose command — send, reply, or their draft
// counterparts — against a mock Gmail server and returns the raw RFC822 of the
// outgoing message plus the stamped ThreadId. finalizePath is the API path the
// command finalizes through ("/gmail/v1/users/me/messages/send" or
// "/gmail/v1/users/me/drafts"). source supplies the msg-1 payload for commands
// that fetch a reply target; a plain gmail send never requests it. The config
// home is isolated so a developer's real no-send config cannot block the
// send-side commands.
func captureComposeRaw(t *testing.T, args []string, finalizePath string, source func() map[string]any) (raw, threadID string) {
	t.Helper()
	setTestConfigHome(t)
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(source())
		case r.Method == http.MethodPost && r.URL.Path == finalizePath:
			raw, threadID = handleFinalizeRaw(t, w, r, finalizePath)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, args, svc)
	if result.err != nil {
		t.Fatalf("Execute(%v): %v", args, result.err)
	}
	return raw, threadID
}

// captureForwardRaw runs a forward-style command and returns the raw RFC822 and
// stamped ThreadId. source supplies the original-message payload; when it
// references attachmentIds (e.g. mockOriginalMessage(true)), the attachment
// bytes are served from the attachments endpoint. The config home is isolated
// so a developer's real no-send config cannot block the send-side forward.
func captureForwardRaw(t *testing.T, args []string, finalizePath string, source func() map[string]any) (raw, threadID string) {
	t.Helper()
	setTestConfigHome(t)
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/attachments/"):
			// Original message attachments (e.g. report.pdf / att-123) re-attached
			// on forward. Deterministic bytes so the parity comparison holds.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": base64.RawURLEncoding.EncodeToString([]byte("pdf-file-contents")),
				"size": 100,
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(source())
		case r.Method == http.MethodPost && r.URL.Path == finalizePath:
			raw, threadID = handleFinalizeRaw(t, w, r, finalizePath)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, args, svc)
	if result.err != nil {
		t.Fatalf("Execute(%v): %v", args, result.err)
	}
	return raw, threadID
}

// rawMessageHeader extracts a single RFC822 header value from a raw MIME
// message (the headers precede the first blank line). It returns "" when the
// header is absent.
func rawMessageHeader(t *testing.T, raw, name string) string {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse raw message: %v\nraw:\n%s", err, raw)
	}
	return msg.Header.Get(name)
}

// wantAddr is one expected recipient (display name + address) for
// assertHeaderRecipients.
type wantAddr struct {
	name    string
	address string
}

// assertHeaderRecipients parses header from raw as an RFC822 address list and
// asserts it contains exactly want, in order, with display names intact. It
// proves a comma inside a quoted display name did not split a recipient in two.
func assertHeaderRecipients(t *testing.T, raw, header string, want []wantAddr) {
	t.Helper()
	value := rawMessageHeader(t, raw, header)
	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		t.Fatalf("parse %s header %q: %v", header, value, err)
	}
	if len(addrs) != len(want) {
		t.Fatalf("%s: expected exactly %d recipients, got %d from %q", header, len(want), len(addrs), value)
	}
	for i, w := range want {
		if addrs[i].Name != w.name || addrs[i].Address != w.address {
			t.Errorf("%s[%d] = %q <%s>, want %q <%s>", header, i, addrs[i].Name, addrs[i].Address, w.name, w.address)
		}
	}
}

// assertDryRunRequestList decodes a --json --dry-run result and asserts that
// request[field] is exactly want (a single-element-per-recipient list). This
// locks the invariant that the dry-run dict reports recipients parsed the same
// way buildGmailMessage builds them — a display-name comma must not split into
// an extra element.
func assertDryRunRequestList(t *testing.T, stdout, field string, want []string) {
	t.Helper()
	var payload struct {
		DryRun  bool           `json:"dry_run"`
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout)
	}
	if !payload.DryRun {
		t.Fatalf("expected dry_run=true, got:\n%s", stdout)
	}
	raw, ok := payload.Request[field].([]any)
	if !ok {
		t.Fatalf("request[%q] is not a list: %#v", field, payload.Request[field])
	}
	got := make([]string, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("request[%q][%d] is not a string: %#v", field, i, v)
		}
		got[i] = s
	}
	if len(got) != len(want) {
		t.Fatalf("request[%q] = %#v, want %#v", field, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("request[%q][%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}

func gmailSearchTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "/users/me/threads") && !strings.Contains(path, "/users/me/threads/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{{"id": "t1"}}, "nextPageToken": "npt",
			})
		case strings.Contains(path, "/users/me/threads/t1"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1", "messages": []map[string]any{{
					"id": "m1", "labelIds": []string{"INBOX"},
					"payload": map[string]any{"headers": []map[string]any{
						{"name": "From", "value": "Me <me@example.com>"},
						{"name": "Subject", "value": "Hello"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					}},
				}},
			})
		case strings.Contains(path, "/users/me/labels"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{{"id": "INBOX", "name": "INBOX", "type": "system"}},
			})
		default:
			http.NotFound(w, r)
		}
	})
}

func newGmailEmptyListTestService(t *testing.T, path, key string) *gmail.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, path) || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{key: []map[string]any{}})
	})
	svc, closeServer := newGoogleTestService(t, handler, gmail.NewService)
	t.Cleanup(closeServer)
	return svc
}

type gmailTestHeader struct {
	Name  string
	Value string
}

type gmailTestMessage struct {
	ThreadID string
	Headers  []gmailTestHeader
}

func newGmailMessagesTestService(t *testing.T, messages map[string]gmailTestMessage) *gmail.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		message, ok := messages[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		headers := make([]map[string]any, 0, len(message.Headers))
		for _, header := range message.Headers {
			headers = append(headers, map[string]any{"name": header.Name, "value": header.Value})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "threadId": message.ThreadID, "payload": map[string]any{"headers": headers},
		})
	})
	svc, closeServer := newGoogleTestService(t, handler, gmail.NewService)
	t.Cleanup(closeServer)
	return svc
}

func newGmailServiceForTest(t *testing.T, h http.HandlerFunc) (*gmail.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, gmail.NewService)
}

func newGmailServiceFromServer(t *testing.T, srv *httptest.Server) *gmail.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", gmail.NewService)
}

func withGmailTestService(ctx context.Context, svc *gmail.Service) context.Context {
	return withGmailTestServiceFactory(ctx, func(context.Context, string) (*gmail.Service, error) {
		return svc, nil
	})
}

func withGmailTestServiceFactory(ctx context.Context, factory app.GmailServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Gmail = factory
	return app.WithRuntime(ctx, runtime)
}

func executeWithGmailTestService(t *testing.T, args []string, svc *gmail.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil },
	}})
}
