package cmd

import (
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

// ---------------------------------------------------------------------------
// forwardSubject
// ---------------------------------------------------------------------------

func TestForwardSubject(t *testing.T) {
	tests := []struct {
		name     string
		original string
		want     string
	}{
		{name: "empty string", original: "", want: "Fwd: (no subject)"},
		{name: "whitespace only", original: "   ", want: "Fwd: (no subject)"},
		{name: "plain subject", original: "Hello", want: "Fwd: Hello"},
		{name: "already Fwd:", original: "Fwd: Hello", want: "Fwd: Hello"},
		{name: "already Fw:", original: "Fw: Hello", want: "Fw: Hello"},
		{name: "already FWD: uppercase", original: "FWD: Hello", want: "FWD: Hello"},
		{name: "already fwd: lowercase", original: "fwd: Hello", want: "fwd: Hello"},
		{name: "already fw: lowercase", original: "fw: Hello", want: "fw: Hello"},
		{name: "Re: subject gets Fwd:", original: "Re: Something", want: "Fwd: Re: Something"},
		{name: "leading whitespace trimmed", original: "  Hello  ", want: "Fwd: Hello"},
		{name: "Fwd: with extra spaces", original: "  Fwd: Hello  ", want: "Fwd: Hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := forwardSubject(tt.original)
			if got != tt.want {
				t.Fatalf("forwardSubject(%q) = %q, want %q", tt.original, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers to build gmail.MessagePart fixtures
// ---------------------------------------------------------------------------

func makePart(mimeType string, body string, headers map[string]string) *gmail.MessagePart {
	p := &gmail.MessagePart{
		MimeType: mimeType,
		Headers:  make([]*gmail.MessagePartHeader, 0, len(headers)),
	}
	for k, v := range headers {
		p.Headers = append(p.Headers, &gmail.MessagePartHeader{Name: k, Value: v})
	}
	if body != "" {
		p.Body = &gmail.MessagePartBody{
			Data: base64.RawURLEncoding.EncodeToString([]byte(body)),
		}
	}
	return p
}

func makeMultipartPayload(headers map[string]string, parts ...*gmail.MessagePart) *gmail.MessagePart {
	p := &gmail.MessagePart{
		MimeType: "multipart/alternative",
		Headers:  make([]*gmail.MessagePartHeader, 0, len(headers)),
		Parts:    parts,
	}
	for k, v := range headers {
		p.Headers = append(p.Headers, &gmail.MessagePartHeader{Name: k, Value: v})
	}
	return p
}

// ---------------------------------------------------------------------------
// forwardHeaderPlain
// ---------------------------------------------------------------------------

func TestForwardHeaderPlain(t *testing.T) {
	t.Run("all headers present", func(t *testing.T) {
		p := makePart("text/plain", "", map[string]string{
			"From":    "alice@example.com",
			"Date":    "Mon, 1 Jan 2024 10:00:00 +0000",
			"Subject": "Test",
			"To":      "bob@example.com",
			"Cc":      "carol@example.com",
		})
		got := forwardHeaderPlain(p)

		if !strings.HasPrefix(got, "---------- Forwarded message ----------") {
			t.Fatalf("missing forwarded message banner:\n%s", got)
		}
		for _, want := range []string{
			"From: alice@example.com",
			"Date: Mon, 1 Jan 2024 10:00:00 +0000",
			"Subject: Test",
			"To: bob@example.com",
			"Cc: carol@example.com",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("missing Cc header omitted", func(t *testing.T) {
		p := makePart("text/plain", "", map[string]string{
			"From":    "alice@example.com",
			"Date":    "Mon, 1 Jan 2024 10:00:00 +0000",
			"Subject": "Test",
			"To":      "bob@example.com",
		})
		got := forwardHeaderPlain(p)

		if strings.Contains(got, "Cc:") {
			t.Fatalf("expected no Cc line, got:\n%s", got)
		}
	})

	t.Run("no headers at all", func(t *testing.T) {
		p := makePart("text/plain", "", map[string]string{})
		got := forwardHeaderPlain(p)

		if got != "---------- Forwarded message ----------" {
			t.Fatalf("expected only banner, got:\n%s", got)
		}
	})
}

// ---------------------------------------------------------------------------
// forwardHeaderHTML
// ---------------------------------------------------------------------------

func TestForwardHeaderHTML(t *testing.T) {
	t.Run("all headers present", func(t *testing.T) {
		p := makePart("text/html", "", map[string]string{
			"From":    "alice@example.com",
			"Date":    "Mon, 1 Jan 2024 10:00:00 +0000",
			"Subject": "Test",
			"To":      "bob@example.com",
			"Cc":      "carol@example.com",
		})
		got := forwardHeaderHTML(p)

		if !strings.Contains(got, "---------- Forwarded message ----------") {
			t.Fatalf("missing forwarded message banner:\n%s", got)
		}
		for _, want := range []string{
			"<b>From:</b> alice@example.com",
			"<b>Date:</b> Mon, 1 Jan 2024 10:00:00 +0000",
			"<b>Subject:</b> Test",
			"<b>To:</b> bob@example.com",
			"<b>Cc:</b> carol@example.com",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("uses br separator", func(t *testing.T) {
		p := makePart("text/html", "", map[string]string{
			"From": "alice@example.com",
			"To":   "bob@example.com",
		})
		got := forwardHeaderHTML(p)

		// Lines are joined by <br>, not newlines.
		if strings.Contains(got, "\n") {
			t.Fatalf("expected <br> separators, not newlines:\n%s", got)
		}
		if !strings.Contains(got, "<br>") {
			t.Fatalf("expected <br> separators:\n%s", got)
		}
	})

	t.Run("HTML-escapes special characters", func(t *testing.T) {
		p := makePart("text/html", "", map[string]string{
			"From": "Alice <alice@example.com>",
		})
		got := forwardHeaderHTML(p)

		if !strings.Contains(got, "&lt;alice@example.com&gt;") {
			t.Fatalf("expected HTML-escaped angle brackets in:\n%s", got)
		}
	})

	t.Run("missing Cc header omitted", func(t *testing.T) {
		p := makePart("text/html", "", map[string]string{
			"From":    "alice@example.com",
			"Subject": "Test",
			"To":      "bob@example.com",
		})
		got := forwardHeaderHTML(p)

		if strings.Contains(got, "Cc:") {
			t.Fatalf("expected no Cc line, got:\n%s", got)
		}
	})
}

// ---------------------------------------------------------------------------
// buildForwardBodies
// ---------------------------------------------------------------------------

func TestBuildForwardBodies(t *testing.T) {
	t.Run("plain only original", func(t *testing.T) {
		payload := makeMultipartPayload(
			map[string]string{
				"From":    "alice@example.com",
				"Subject": "Test",
			},
			makePart("text/plain", "Hello, world!", map[string]string{}),
		)
		plain, htmlBody := buildForwardBodies(payload, "")

		if !strings.Contains(plain, "---------- Forwarded message ----------") {
			t.Fatalf("plain missing forwarded header:\n%s", plain)
		}
		if !strings.Contains(plain, "Hello, world!") {
			t.Fatalf("plain missing original body:\n%s", plain)
		}
		if htmlBody != "" {
			t.Fatalf("expected empty HTML body for plain-only original, got:\n%s", htmlBody)
		}
	})

	t.Run("HTML only original generates plain fallback", func(t *testing.T) {
		payload := makeMultipartPayload(
			map[string]string{
				"From":    "alice@example.com",
				"Subject": "Test",
			},
			makePart("text/html", "<p>Hello</p>", map[string]string{}),
		)
		plain, htmlBody := buildForwardBodies(payload, "")

		// Plain body should exist (stripped from HTML).
		if !strings.Contains(plain, "Hello") {
			t.Fatalf("plain missing stripped HTML content:\n%s", plain)
		}
		if !strings.Contains(plain, "---------- Forwarded message ----------") {
			t.Fatalf("plain missing forwarded header:\n%s", plain)
		}
		// HTML body should exist.
		if htmlBody == "" {
			t.Fatalf("expected non-empty HTML body for HTML original")
		}
		if !strings.Contains(htmlBody, "<p>Hello</p>") {
			t.Fatalf("HTML body missing original content:\n%s", htmlBody)
		}
	})

	t.Run("both plain and HTML original", func(t *testing.T) {
		payload := makeMultipartPayload(
			map[string]string{
				"From":    "alice@example.com",
				"Subject": "Test",
			},
			makePart("text/plain", "Plain text", map[string]string{}),
			makePart("text/html", "<b>Rich text</b>", map[string]string{}),
		)
		plain, htmlBody := buildForwardBodies(payload, "")

		if !strings.Contains(plain, "Plain text") {
			t.Fatalf("plain missing original text:\n%s", plain)
		}
		if !strings.Contains(htmlBody, "<b>Rich text</b>") {
			t.Fatalf("HTML missing original HTML:\n%s", htmlBody)
		}
	})

	t.Run("with preface text", func(t *testing.T) {
		payload := makeMultipartPayload(
			map[string]string{
				"From":    "alice@example.com",
				"Subject": "Test",
			},
			makePart("text/plain", "Original body", map[string]string{}),
			makePart("text/html", "<p>Original body</p>", map[string]string{}),
		)
		plain, htmlBody := buildForwardBodies(payload, "FYI see below")

		// Preface should appear before the forwarded header.
		prefaceIdx := strings.Index(plain, "FYI see below")
		headerIdx := strings.Index(plain, "---------- Forwarded message ----------")
		if prefaceIdx < 0 || headerIdx < 0 || prefaceIdx >= headerIdx {
			t.Fatalf("preface should appear before forwarded header in plain:\n%s", plain)
		}

		// HTML body should also include the preface.
		if !strings.Contains(htmlBody, "FYI see below") {
			t.Fatalf("HTML body missing preface:\n%s", htmlBody)
		}
	})

	t.Run("without preface", func(t *testing.T) {
		payload := makeMultipartPayload(
			map[string]string{
				"From":    "alice@example.com",
				"Subject": "Test",
			},
			makePart("text/plain", "Body text", map[string]string{}),
		)
		plain, _ := buildForwardBodies(payload, "")

		// Should start with the forwarded header (no preface).
		if !strings.HasPrefix(strings.TrimSpace(plain), "---------- Forwarded message ----------") {
			t.Fatalf("expected plain to start with forwarded header:\n%s", plain)
		}
	})

	t.Run("empty original body", func(t *testing.T) {
		payload := makeMultipartPayload(
			map[string]string{
				"From":    "alice@example.com",
				"Subject": "Test",
			},
		)
		plain, htmlBody := buildForwardBodies(payload, "")

		if !strings.Contains(plain, "---------- Forwarded message ----------") {
			t.Fatalf("plain missing forwarded header:\n%s", plain)
		}
		if htmlBody != "" {
			t.Fatalf("expected empty HTML body when original has no content, got:\n%s", htmlBody)
		}
	})
}

// ---------------------------------------------------------------------------
// plainToHTML
// ---------------------------------------------------------------------------

func TestPlainToHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple text", input: "Hello", want: "Hello"},
		{name: "newlines to br", input: "Line 1\nLine 2", want: "Line 1<br>Line 2"},
		{name: "CRLF to br", input: "Line 1\r\nLine 2", want: "Line 1<br>Line 2"},
		{name: "bare CR to br", input: "Line 1\rLine 2", want: "Line 1<br>Line 2"},
		{name: "escapes ampersand", input: "A & B", want: "A &amp; B"},
		{name: "escapes angle brackets", input: "<script>alert(1)</script>", want: "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{name: "escapes quotes", input: `She said "hello"`, want: "She said &#34;hello&#34;"},
		{name: "empty string", input: "", want: ""},
		{name: "multiple newlines", input: "A\n\nB", want: "A<br><br>B"},
		{name: "mixed line endings", input: "A\r\nB\rC\nD", want: "A<br>B<br>C<br>D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plainToHTML(tt.input)
			if got != tt.want {
				t.Fatalf("plainToHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// joinForwardSections
// ---------------------------------------------------------------------------

func TestJoinForwardSections(t *testing.T) {
	tests := []struct {
		name  string
		sep   string
		parts []string
		want  string
	}{
		{
			name:  "all non-empty",
			sep:   "\n\n",
			parts: []string{"A", "B", "C"},
			want:  "A\n\nB\n\nC",
		},
		{
			name:  "skips empty",
			sep:   "\n\n",
			parts: []string{"A", "", "C"},
			want:  "A\n\nC",
		},
		{
			name:  "skips whitespace-only",
			sep:   "\n\n",
			parts: []string{"A", "   ", "C"},
			want:  "A\n\nC",
		},
		{
			name:  "all empty returns empty",
			sep:   "\n\n",
			parts: []string{"", "", ""},
			want:  "",
		},
		{
			name:  "single part",
			sep:   "\n\n",
			parts: []string{"A"},
			want:  "A",
		},
		{
			name:  "no parts",
			sep:   "\n\n",
			parts: []string{},
			want:  "",
		},
		{
			name:  "HTML separator",
			sep:   "<br><br>",
			parts: []string{"Hello", "World"},
			want:  "Hello<br><br>World",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinForwardSections(tt.sep, tt.parts...)
			if got != tt.want {
				t.Fatalf("joinForwardSections(%q, %v) = %q, want %q", tt.sep, tt.parts, got, tt.want)
			}
		})
	}
}
