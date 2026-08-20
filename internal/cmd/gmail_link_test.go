package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGmailLinkTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<p>Click <a href="https://pay.example/btn">Pay now</a> or visit https://info.example/page</p>`,
	))
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "m1",
			"threadId": "t1",
			"payload": map[string]any{
				"mimeType": "text/html",
				"body":     map[string]any{"data": htmlBody},
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "Subject", "value": "Pay up"},
				},
			},
		})
	}))
}

func TestGmailLinkCmd_ResolvesMarkerIndex(t *testing.T) {
	srv := newGmailLinkTestServer(t)
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "link", "m1", "0"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var link struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &link); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if link.URL != "https://pay.example/btn" || link.Text != "Pay now" {
		t.Fatalf("unexpected link: %#v", link)
	}

	result = executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "link", "m1", "1"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if err := json.Unmarshal([]byte(result.stdout), &link); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if link.URL != "https://info.example/page" {
		t.Fatalf("unexpected link: %#v", link)
	}
	// A bare URL has no anchor text, so the field is absent entirely.
	if strings.Contains(result.stdout, "\"text\"") {
		t.Fatalf("bare url should have no text field: %s", result.stdout)
	}
}

func TestGmailLinkCmd_IndexParityWithSanitizedGet(t *testing.T) {
	srv := newGmailLinkTestServer(t)
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "get", "m1", "--sanitize-content"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var envelope struct {
		Message struct {
			Body string `json:"body"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.Message.Body != "Click Pay now [link:0] or visit [link:1]" {
		t.Fatalf("unexpected sanitized body: %q", envelope.Message.Body)
	}
}

func TestGmailLinkCmd_TrimPunctuation(t *testing.T) {
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<p>Details (https://info.example/x) here, button: <a href="https://pay.example/a_(b)">Pay</a></p>`,
	))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "m1", "threadId": "t1",
			"payload": map[string]any{
				"mimeType": "text/html",
				"body":     map[string]any{"data": htmlBody},
			},
		})
	}))
	defer srv.Close()

	// Default: url is the exact capture, prose ")" included.
	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "link", "m1", "0"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var resolved struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &resolved); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if resolved.URL != "https://info.example/x)" {
		t.Fatalf("unexpected url: %q", resolved.URL)
	}

	// --trim-punctuation strips the prose suffix from the text-captured URL.
	result = executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "link", "m1", "0", "--trim-punctuation"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if err := json.Unmarshal([]byte(result.stdout), &resolved); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if resolved.URL != "https://info.example/x" {
		t.Fatalf("unexpected trimmed url: %q", resolved.URL)
	}

	// The anchor href is byte-exact; its balanced ")" is data even with the flag.
	result = executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "link", "m1", "1", "--trim-punctuation"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if err := json.Unmarshal([]byte(result.stdout), &resolved); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if resolved.URL != "https://pay.example/a_(b)" {
		t.Fatalf("unexpected anchor url: %q", resolved.URL)
	}
}

func TestGmailLinkTrimmedURL(t *testing.T) {
	tests := []struct {
		name string
		link gmailLink
		want string
	}{
		{name: "prose paren dropped", link: gmailLink{URL: "https://x.example/y)", fromText: true}, want: "https://x.example/y"},
		{name: "sentence period dropped", link: gmailLink{URL: "https://x.example/y.", fromText: true}, want: "https://x.example/y"},
		{name: "comma dropped", link: gmailLink{URL: "https://x.example/y,", fromText: true}, want: "https://x.example/y"},
		{name: "mixed trailing run dropped", link: gmailLink{URL: "https://x.example/y.)!,", fromText: true}, want: "https://x.example/y"},
		{name: "balanced parens kept", link: gmailLink{URL: "https://x.example/wiki/Foo_(bar)", fromText: true}, want: ""},
		{name: "balanced parens then period", link: gmailLink{URL: "https://x.example/wiki/Foo_(bar).", fromText: true}, want: "https://x.example/wiki/Foo_(bar)"},
		{name: "clean url untouched", link: gmailLink{URL: "https://x.example/y", fromText: true}, want: ""},
		{name: "nothing left after scheme", link: gmailLink{URL: "https://...", fromText: true}, want: ""},
		{name: "anchor href never trimmed", link: gmailLink{URL: "https://x.example/y."}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.link.trimmedURL(); got != tt.want {
				t.Fatalf("trimmedURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGmailLinkCmd_EscapesControlCharactersInLineOutput(t *testing.T) {
	// Entities in the href decode to a raw newline and tab in the captured URL.
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<p><a href="https://evil.example/a&#10;url&#9;forged">click</a></p>`,
	))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "m1", "threadId": "t1",
			"payload": map[string]any{
				"mimeType": "text/html",
				"body":     map[string]any{"data": htmlBody},
			},
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--account", "a@b.com", "gmail", "link", "m1", "0"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, `"https://evil.example/a\nurl\tforged"`) {
		t.Fatalf("control characters should be quoted: %q", result.stdout)
	}
	if strings.Contains(result.stdout, "evil.example/a\nurl") {
		t.Fatalf("raw control characters leaked to line output: %q", result.stdout)
	}
}

func TestGmailLinkCmd_UsageErrors(t *testing.T) {
	srv := newGmailLinkTestServer(t)
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--account", "a@b.com", "gmail", "link", "m1", "x"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err == nil || !strings.Contains(result.err.Error(), "0-based index") {
		t.Fatalf("expected non-numeric index usage error, got: %v", result.err)
	}

	result = executeWithGmailTestService(
		t,
		[]string{"--account", "a@b.com", "gmail", "link", "m1", "5"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err == nil || !strings.Contains(result.err.Error(), "link index 5 out of range: message has 2 link(s)") {
		t.Fatalf("expected out-of-range usage error, got: %v", result.err)
	}
}
