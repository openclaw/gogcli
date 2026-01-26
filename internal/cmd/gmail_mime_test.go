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
	if !strings.Contains(s, "Subject: =?utf-8?") {
		t.Fatalf("expected encoded-word Subject: %q", s)
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
	if got == "Grüße" || !strings.Contains(got, "=?utf-8?") {
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

func TestEncodeAddressHeader(t *testing.T) {
	// Plain email without display name - unchanged
	if got := encodeAddressHeader("test@example.com"); got != "test@example.com" {
		t.Fatalf("plain email changed: %q", got)
	}

	// ASCII display name - unchanged
	if got := encodeAddressHeader("John Doe <john@example.com>"); got != "John Doe <john@example.com>" {
		t.Fatalf("ASCII name changed: %q", got)
	}

	// UTF-8 display name - should be encoded
	got := encodeAddressHeader("Jørgen Østergaard <jorgen@example.com>")
	if strings.Contains(got, "Jørgen") {
		t.Fatalf("UTF-8 name not encoded: %q", got)
	}
	if !strings.Contains(got, "=?utf-8?q?") {
		t.Fatalf("expected MIME encoding: %q", got)
	}
	if !strings.Contains(got, "<jorgen@example.com>") {
		t.Fatalf("email address missing: %q", got)
	}

	// Invalid address - returned as-is
	if got := encodeAddressHeader("not a valid address"); got != "not a valid address" {
		t.Fatalf("invalid address changed: %q", got)
	}
}

func TestBuildRFC822UTF8FromDisplayName(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "Jørgen Østergaard <jorgen@example.com>",
		To:      []string{"recipient@example.com"},
		Subject: "Test",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	s := string(raw)
	// The display name should be MIME-encoded, not raw UTF-8
	if strings.Contains(s, "From: Jørgen") {
		t.Fatalf("From header contains unencoded UTF-8: %q", s)
	}
	// Should contain encoded-word format (may be quoted by mail.Address.String)
	if !strings.Contains(s, "=?utf-8?q?") {
		t.Fatalf("expected MIME-encoded From display name: %q", s)
	}
	// Should still contain the email address
	if !strings.Contains(s, "<jorgen@example.com>") {
		t.Fatalf("missing email address in From header: %q", s)
	}
}
