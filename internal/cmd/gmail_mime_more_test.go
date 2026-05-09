package cmd

import (
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildRFC822_MissingFields(t *testing.T) {
	if _, err := buildRFC822(mailOptions{To: []string{"c@d.com"}, Subject: "Hi"}, nil); err == nil {
		t.Fatalf("expected missing From error")
	}
	if _, err := buildRFC822(mailOptions{From: "a@b.com", Subject: "Hi"}, nil); err == nil {
		t.Fatalf("expected missing To error")
	}
	if _, err := buildRFC822(mailOptions{From: "a@b.com", To: []string{"c@d.com"}}, nil); err == nil {
		t.Fatalf("expected missing Subject error")
	}
}

func TestBuildRFC822_AllowMissingTo(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		Subject: "Hi",
		Body:    "Hello",
	}, &rfc822Config{allowMissingTo: true})
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}
	if strings.Contains(string(raw), "\r\nTo:") {
		t.Fatalf("expected no To header")
	}
}

func TestBuildRFC822_DateHeaderUsesConfiguredTimezone(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	t.Cleanup(func() { time.Local = oldLocal })
	t.Setenv("GOG_TIMEZONE", "UTC")

	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	dateHeader := msg.Header.Get("Date")
	if !strings.HasSuffix(dateHeader, "+0000") {
		t.Fatalf("expected UTC Date header, got %q", dateHeader)
	}
}

func TestBuildRFC822_InvalidHeaders(t *testing.T) {
	if _, err := buildRFC822(mailOptions{
		From:    "a@b.com\r\nBcc: evil@evil.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
	}, nil); err == nil {
		t.Fatalf("expected invalid From error")
	}
	if _, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com\r\n"},
		Subject: "Hi",
	}, nil); err == nil {
		t.Fatalf("expected invalid address error")
	}
	if _, err := buildRFC822(mailOptions{
		From:      "a@b.com",
		To:        []string{"c@d.com"},
		Subject:   "Hi",
		ReplyTo:   "reply@ex\r\nample.com",
		Body:      "Hello",
		InReplyTo: "<id>\r\n",
	}, nil); err == nil {
		t.Fatalf("expected invalid Reply-To error")
	}
	if _, err := buildRFC822(mailOptions{
		From:       "a@b.com",
		To:         []string{"c@d.com"},
		Subject:    "Hi",
		References: "<id>\r\n",
		Body:       "Hello",
	}, nil); err == nil {
		t.Fatalf("expected invalid References error")
	}
	if _, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi\r\n",
		Body:    "Hello",
	}, nil); err == nil {
		t.Fatalf("expected invalid Subject error")
	}
	if _, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		AdditionalHeaders: map[string]string{
			"X-Test": "bad\r\nvalue",
		},
	}, nil); err == nil {
		t.Fatalf("expected invalid header value error")
	}
}

func TestBuildRFC822_AttachmentFromPath_DefaultMime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.unknownext")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
		Attachments: []mailAttachment{
			{Path: path},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "application/octet-stream") {
		t.Fatalf("expected default mime type, got: %q", s)
	}
	if !strings.Contains(s, "Content-Disposition: attachment; filename=\"file.unknownext\"") {
		t.Fatalf("expected attachment header, got: %q", s)
	}
}

func TestWrapBase64_LongLine(t *testing.T) {
	data := make([]byte, 80)
	out := wrapBase64(data)
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("expected wrapped base64")
	}
}
