package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSanitizeGmailBody(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		isHTML bool
		want   string
	}{
		{
			name:   "html strips scripts and marks visible urls",
			body:   `<script>fetch("https://tracker.example/open")</script><p>Hello https://phish.example/login</p>`,
			isHTML: true,
			want:   "Hello [link:0]",
		},
		{
			name:   "plain decodes entity-obfuscated url",
			body:   `open &#104;ttps://evil.example/path now`,
			isHTML: false,
			want:   "open [link:0] now",
		},
		{
			name:   "html keeps link text and marks its target",
			body:   `<p>Click <a href="https://evil.example">here</a></p>`,
			isHTML: true,
			want:   "Click here [link:0]",
		},
		{
			name:   "style block removed",
			body:   `<style>body{background:url(https://tracker.example)}</style><p>Visible</p>`,
			isHTML: true,
			want:   "Visible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeGmailBody(tt.body, tt.isHTML); got != tt.want {
				t.Fatalf("sanitizeGmailBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeGmailBodyLinks(t *testing.T) {
	body := `<script>fetch("https://tracker.example/open")</script>` +
		`<p>Click <a href="https://pay.example/btn">Pay now</a></p>` +
		`<p>Visit https://info.example/page for details</p>` +
		`<p>Again <a href="https://pay.example/btn">same target</a></p>` +
		`<p>Write to <a href="mailto:billing@example.com">billing</a></p>`
	text, links := sanitizeGmailBodyLinks(body, true)
	want := "Click Pay now [link:0] Visit [link:1] for details Again same target [link:0] Write to billing [link:2]"
	if text != want {
		t.Fatalf("unexpected text:\n got %q\nwant %q", text, want)
	}
	wantLinks := []gmailLink{
		{URL: "https://pay.example/btn", Text: "Pay now"},
		{URL: "https://info.example/page", fromText: true},
		{URL: "mailto:billing@example.com", Text: "billing"},
	}
	if len(links) != len(wantLinks) {
		t.Fatalf("unexpected links: %#v", links)
	}
	for i, wantLink := range wantLinks {
		if links[i] != wantLink {
			t.Fatalf("link %d = %#v, want %#v", i, links[i], wantLink)
		}
	}
}

func TestSanitizeGmailBodyLinks_BareURLBoundaries(t *testing.T) {
	body := "See https://en.wikipedia.org/wiki/Foo_(bar) and (https://info.example/x) for details"
	text, links := sanitizeGmailBodyLinks(body, false)
	want := "See [link:0] and ([link:1] for details"
	if text != want {
		t.Fatalf("unexpected text:\n got %q\nwant %q", text, want)
	}
	// Every captured character is kept: the balanced-paren path survives whole, and the
	// prose paren after the second URL is captured rather than cut — the reader decides.
	wantLinks := []gmailLink{
		{URL: "https://en.wikipedia.org/wiki/Foo_(bar)", fromText: true},
		{URL: "https://info.example/x)", fromText: true},
	}
	if len(links) != len(wantLinks) {
		t.Fatalf("unexpected links: %#v", links)
	}
	for i, wantLink := range wantLinks {
		if links[i] != wantLink {
			t.Fatalf("link %d = %#v, want %#v", i, links[i], wantLink)
		}
	}
}

func TestSanitizeGmailBodyLinks_ImageAnchors(t *testing.T) {
	body := `<p><a href="https://promo.example/go"><img src="hero.png" alt="Start now"></a></p>` +
		`<p><a href="https://docs.example/x"><img src="icon.png"></a> then <a href="https://docs.example/x">read the docs</a></p>`
	text, links := sanitizeGmailBodyLinks(body, true)
	want := "[link:0] [link:1] then read the docs [link:1]"
	if text != want {
		t.Fatalf("unexpected text:\n got %q\nwant %q", text, want)
	}
	wantLinks := []gmailLink{
		// No visible text: the wrapped image's alt stands in.
		{URL: "https://promo.example/go", Text: "Start now"},
		// The first site is textless; the second site's text names the link.
		{URL: "https://docs.example/x", Text: "read the docs"},
	}
	if len(links) != len(wantLinks) {
		t.Fatalf("unexpected links: %#v", links)
	}
	for i, wantLink := range wantLinks {
		if links[i] != wantLink {
			t.Fatalf("link %d = %#v, want %#v", i, links[i], wantLink)
		}
	}
}

func TestGmailGetCmd_SanitizeContent_JSONUsesSafeEnvelope(t *testing.T) {
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<html><body><script>fetch("https://tracker.example/open")</script><p>Hello https://phish.example/login</p></body></html>`,
	))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "m1",
			"threadId":     "t1",
			"labelIds":     []string{"INBOX"},
			"snippet":      "snippet https://snippet.example",
			"internalDate": "1766743200000",
			"payload": map[string]any{
				"mimeType": "text/html",
				"body":     map[string]any{"data": htmlBody},
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Subject", "value": "Visit https://evil.example now"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
					{"name": "List-Unsubscribe", "value": "<https://unsub.example.com>"},
				},
			},
		})
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "get", "m1", "--sanitize-content"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if strings.Contains(result.stdout, "https://") || strings.Contains(result.stdout, "tracker.example") || strings.Contains(result.stdout, htmlBody) {
		t.Fatalf("sanitized JSON leaked unsafe content: %s", result.stdout)
	}
	if strings.Contains(result.stdout, "payload") || strings.Contains(result.stdout, "unsubscribe") {
		t.Fatalf("sanitized JSON should not expose raw Gmail payload/unsubscribe: %s", result.stdout)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("decode JSON envelope: %v", err)
	}
	if len(envelope) != 1 || envelope["message"] == nil {
		t.Fatalf("sanitized JSON should contain the message once, got: %s", result.stdout)
	}

	var parsed struct {
		ID      string            `json:"id"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(envelope["message"], &parsed); err != nil {
		t.Fatalf("decode sanitized message: %v", err)
	}
	if parsed.Body != "Hello [link:0]" {
		t.Fatalf("unexpected body: %q", parsed.Body)
	}
	if parsed.Headers["subject"] != "Visit [url removed] now" {
		t.Fatalf("unexpected sanitized subject: %#v", parsed.Headers)
	}

	result = executeWithGmailTestService(
		t,
		[]string{"--json", "--results-only", "--account", "a@b.com", "gmail", "get", "m1", "--sanitize-content"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute with --results-only: %v\nstderr=%q", result.err, result.stderr)
	}
	var direct gmailSanitizedMessageOutput
	if err := json.Unmarshal([]byte(result.stdout), &direct); err != nil {
		t.Fatalf("decode direct sanitized message: %v", err)
	}
	if direct.ID != parsed.ID || direct.Body != parsed.Body || direct.Headers["subject"] != parsed.Headers["subject"] {
		t.Fatalf("--results-only did not unwrap the sanitized message: %s", result.stdout)
	}
}

func TestGmailGetCmd_SanitizeContentRejectsRaw(t *testing.T) {
	result := executeWithTestRuntime(t, []string{"--account", "a@b.com", "gmail", "get", "m1", "--format", "raw", "--sanitize-content"}, nil)
	if result.err == nil || !strings.Contains(result.err.Error(), "--sanitize-content cannot be used with --format raw") {
		t.Fatalf("expected raw/sanitize usage error, got: %v", result.err)
	}
}

func TestGmailThreadGet_SanitizeContent_JSONUsesSafeEnvelope(t *testing.T) {
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<style>.x{background:url(https://tracker.example)}</style><p>Hello https://phish.example/login</p>`,
	))
	secondBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<p>Bye <a href="https://other.example/x">there</a></p>`,
	))
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "Check https://evil.example now"},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
						{"name": "List-Unsubscribe", "value": "<https://unsub.example.com>"},
					},
					"mimeType": "text/html",
					"body":     map[string]any{"data": htmlBody},
				},
			},
			{
				"id":       "m2",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "b@example.com"},
						{"name": "Subject", "value": "Re: Check"},
					},
					"mimeType": "text/html",
					"body":     map[string]any{"data": secondBody},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method == http.MethodGet && path == "/users/me/threads/t1" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "thread", "get", "t1", "--sanitize-content"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if strings.Contains(result.stdout, "https://") || strings.Contains(result.stdout, "tracker.example") || strings.Contains(result.stdout, htmlBody) {
		t.Fatalf("sanitized thread JSON leaked unsafe content: %s", result.stdout)
	}
	if strings.Contains(result.stdout, "payload") || strings.Contains(result.stdout, "unsubscribe") {
		t.Fatalf("sanitized thread JSON should not expose raw Gmail payload/unsubscribe: %s", result.stdout)
	}
	var parsed struct {
		Thread struct {
			Messages []gmailSanitizedMessageOutput `json:"messages"`
		} `json:"thread"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(parsed.Thread.Messages) != 2 {
		t.Fatalf("unexpected messages: %#v", parsed.Thread.Messages)
	}
	if got := parsed.Thread.Messages[0].Body; got != "Hello [link:0]" {
		t.Fatalf("unexpected body: %q", got)
	}
	// Marker numbering restarts per message.
	if got := parsed.Thread.Messages[1].Body; got != "Bye there [link:0]" {
		t.Fatalf("unexpected second body: %q", got)
	}
}
