package cmd

import (
	"regexp"
	"strings"
	"testing"
)

func TestBuildRFC822Plain(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "\r\nMessage-ID: <") {
		t.Fatalf("missing message-id: %q", s)
	}
	if !strings.Contains(s, "Content-Type: text/plain") {
		t.Fatalf("missing content-type: %q", s)
	}
	if !strings.Contains(s, "\r\n\r\nHello\r\n") {
		t.Fatalf("missing body: %q", s)
	}
}

func TestBuildRFC822HTMLOnly(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		BodyHTML: "<p>Hello</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("missing content-type: %q", s)
	}
	if strings.Contains(s, "multipart/alternative") {
		t.Fatalf("unexpected multipart/alternative: %q", s)
	}
	if !strings.Contains(s, "<p>Hello</p>") {
		t.Fatalf("missing html body: %q", s)
	}
}

func TestBuildRFC822PlainAndHTMLAlternative(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		Body:     "Plain",
		BodyHTML: "<p>HTML</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative: %q", s)
	}
	if !strings.Contains(s, "Content-Type: text/plain") || !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("expected both text/plain and text/html parts: %q", s)
	}
	if !strings.Contains(s, "\r\n\r\nPlain\r\n") || !strings.Contains(s, "<p>HTML</p>") {
		t.Fatalf("missing bodies: %q", s)
	}
}

func TestBuildRFC822WithAttachment(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
		Attachments: []mailAttachment{
			{Filename: "x.txt", MIMEType: "text/plain", Data: []byte("abc")},
		},
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "multipart/mixed") {
		t.Fatalf("expected multipart: %q", s)
	}
	if !strings.Contains(s, "Content-Disposition: attachment; filename=\"x.txt\"") {
		t.Fatalf("missing attachment header: %q", s)
	}
}

func TestBuildRFC822AlternativeWithAttachment(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		Body:     "Plain",
		BodyHTML: "<p>HTML</p>",
		Attachments: []mailAttachment{
			{Filename: "x.txt", MIMEType: "text/plain", Data: []byte("abc")},
		},
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "multipart/mixed") {
		t.Fatalf("expected multipart/mixed: %q", s)
	}
	if !strings.Contains(s, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative: %q", s)
	}
	if !strings.Contains(s, "Content-Disposition: attachment; filename=\"x.txt\"") {
		t.Fatalf("missing attachment header: %q", s)
	}
	if !strings.Contains(s, "Content-Type: text/plain") || !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("expected both text/plain and text/html parts: %q", s)
	}
}

func TestBuildRFC822UTF8Subject(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Grüße",
		Body:    "Hi",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(strings.ToLower(s), "subject: =?utf-8?") {
		t.Fatalf("expected encoded-word Subject: %q", s)
	}
}

func TestBuildRFC822UTF8FromDisplayName(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "Sérgio Bastos • Importrust <alias@domain.com>",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(strings.ToLower(s), "from: =?utf-8?") {
		t.Fatalf("expected encoded-word From header: %q", s)
	}
	if !strings.Contains(s, "<alias@domain.com>") {
		t.Fatalf("expected alias email in From header: %q", s)
	}
	if strings.Contains(s, "From: Sérgio Bastos • Importrust <alias@domain.com>") {
		t.Fatalf("expected From header to be RFC 2047 encoded: %q", s)
	}
}

func TestBuildRFC822PlainFromAddressStaysUnwrapped(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "From: a@b.com\r\n") {
		t.Fatalf("expected plain From address, got: %q", s)
	}
	if strings.Contains(s, "From: <a@b.com>\r\n") {
		t.Fatalf("unexpected wrapped From address: %q", s)
	}
}

func TestBuildRFC822ReplyToHeader(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		ReplyTo: "reply@example.com",
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "Reply-To: reply@example.com") {
		t.Fatalf("missing Reply-To header: %q", s)
	}
}

func TestBuildRFC822AdditionalHeadersMessageIDIsNotDuplicated(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
		AdditionalHeaders: map[string]string{
			"Message-ID": "<custom@id>",
		},
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	if strings.Count(s, "\r\nMessage-ID: ") != 1 {
		t.Fatalf("expected exactly one Message-ID header: %q", s)
	}
	if !strings.Contains(s, "\r\nMessage-ID: <custom@id>\r\n") {
		t.Fatalf("missing custom message-id: %q", s)
	}
}

func TestBuildRFC822ReplyToRejectsNewlines(t *testing.T) {
	_, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		ReplyTo: "a@b.com\r\nBcc: evil@evil.com",
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestEncodeHeaderIfNeeded(t *testing.T) {
	if got := encodeHeaderIfNeeded("Hello"); got != "Hello" {
		t.Fatalf("unexpected: %q", got)
	}
	got := encodeHeaderIfNeeded("Grüße")
	if got == "Grüße" || !strings.Contains(strings.ToLower(got), "=?utf-8?") {
		t.Fatalf("expected encoded-word, got: %q", got)
	}
}

func TestContentDispositionFilename(t *testing.T) {
	if got := contentDispositionFilename("a.txt"); got != "filename=\"a.txt\"" {
		t.Fatalf("unexpected: %q", got)
	}
	got := contentDispositionFilename("Grüße.txt")
	if !strings.HasPrefix(got, "filename*=UTF-8''") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestNormalizeCRLF(t *testing.T) {
	if got := normalizeCRLF(""); got != "" {
		t.Fatalf("unexpected: %q", got)
	}

	got := normalizeCRLF("a\nb\r\nc\rd")
	if got != "a\r\nb\r\nc\r\nd" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestHasHeader(t *testing.T) {
	if hasHeader(nil, "Message-ID") {
		t.Fatalf("expected false")
	}
	if hasHeader(map[string]string{}, "Message-ID") {
		t.Fatalf("expected false")
	}
	if !hasHeader(map[string]string{"message-id": "x"}, "Message-ID") {
		t.Fatalf("expected true")
	}
	if !hasHeader(map[string]string{"Message-Id": "x"}, "message-id") {
		t.Fatalf("expected true")
	}
}

func TestRandomMessageID(t *testing.T) {
	id, err := randomMessageID("A <a@b.com>")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !regexp.MustCompile(`^<[A-Za-z0-9_-]+@b\.com>$`).MatchString(id) {
		t.Fatalf("unexpected: %q", id)
	}

	id, err = randomMessageID("not-an-email")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !regexp.MustCompile(`^<[A-Za-z0-9_-]+@gogcli\.local>$`).MatchString(id) {
		t.Fatalf("unexpected: %q", id)
	}
}

func TestFormatAddressHeaderUnparseable(t *testing.T) {
	input := "not an email at all"
	got := formatAddressHeader(input)
	if got != input {
		t.Fatalf("expected unparseable input returned unchanged, got: %q", got)
	}
}

func TestFormatAddressHeadersMixed(t *testing.T) {
	input := []string{"Alice <a@b.com>", "c@d.com", "Sérgio Bastos <s@b.com>"}
	got := formatAddressHeaders(input)

	// Should contain all three addresses comma-separated.
	parts := strings.SplitN(got, ", ", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 comma-separated parts, got %d: %q", len(parts), got)
	}

	// First part: display name "Alice" with address a@b.com.
	if !strings.Contains(parts[0], "Alice") || !strings.Contains(parts[0], "a@b.com") {
		t.Fatalf("unexpected first part: %q", parts[0])
	}

	// Second part: plain address, no angle brackets.
	if parts[1] != "c@d.com" {
		t.Fatalf("expected plain address c@d.com, got: %q", parts[1])
	}

	// Third part: non-ASCII name must be RFC 2047 encoded.
	if !strings.Contains(strings.ToLower(parts[2]), "=?utf-8?") {
		t.Fatalf("expected RFC 2047 encoded name in third part, got: %q", parts[2])
	}
	if !strings.Contains(parts[2], "s@b.com") {
		t.Fatalf("expected address s@b.com in third part, got: %q", parts[2])
	}
}

func TestFormatAddressHeadersFiltersEmpty(t *testing.T) {
	got := formatAddressHeaders([]string{"a@b.com", "", "b@c.com"})
	expected := "a@b.com, b@c.com"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestFormatAddressHeadersParsesCommaSeparatedList(t *testing.T) {
	got := formatAddressHeaders([]string{"Alice <a@b.com>, Bob <b@c.com>"})
	parts := strings.SplitN(got, ", ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 2 comma-separated parts, got %q", got)
	}
	if !strings.Contains(parts[0], "a@b.com") || !strings.Contains(parts[1], "b@c.com") {
		t.Fatalf("expected both addresses in output, got %q", got)
	}
}
