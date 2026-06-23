package cmd

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
)

func TestFormatQuotedMessage(t *testing.T) {
	got := formatQuotedMessage("Alice <a@example.com>", "Mon, 1 Jan 2024 00:00:00 +0000", "l1\nl2", time.UTC)
	wantContains := []string{
		"\n\nOn Mon, Jan 1, 2024 at 12:00 AM, Alice <a@example.com> wrote:\n",
		"> l1\n",
		"> l2\n",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Fatalf("expected %q in output, got %q", s, got)
		}
	}
}

func TestFormatQuoteDate(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		loc  *time.Location
		want string
	}{
		{"rfc2822 in utc", "Wed, 17 Jun 2026 00:29:51 +0000", time.UTC, "Wed, Jun 17, 2026 at 12:29 AM"},
		{"converts offset into loc", "Wed, 17 Jun 2026 00:29:51 -0500", time.UTC, "Wed, Jun 17, 2026 at 5:29 AM"},
		{"empty stays empty", "", time.UTC, ""},
		{"unparseable returns trimmed raw", "  not a date  ", time.UTC, "not a date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatQuoteDate(tc.raw, tc.loc); got != tc.want {
				t.Fatalf("formatQuoteDate(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}

	// A nil loc falls back to time.Local rather than panicking.
	if got := formatQuoteDate("Wed, 17 Jun 2026 00:29:51 +0000", nil); got == "" {
		t.Fatal("formatQuoteDate with nil loc returned empty")
	}
}

func TestFormatQuotedMessageHTMLWithContent_EscapesHeader_NotBody(t *testing.T) {
	out := formatQuotedMessageHTMLWithContent(`"><script>alert(1)</script>`, `<b>bad</b>`, `<b>ok</b>`, time.UTC)
	if strings.Contains(out, "<script>") {
		t.Fatalf("expected script tag to be escaped, got %q", out)
	}
	if strings.Contains(out, "<b>bad</b>") {
		t.Fatalf("expected date to be escaped, got %q", out)
	}
	if !strings.Contains(out, "<b>ok</b>") {
		t.Fatalf("expected htmlContent to be preserved, got %q", out)
	}
}

func TestFormatQuotedMessageHTMLWithContent_Date(t *testing.T) {
	// A parseable date is reformatted to Gmail style, converted into loc.
	out := formatQuotedMessageHTMLWithContent("Alice <a@example.com>", "Wed, 17 Jun 2026 00:29:51 +0000", "<p>body</p>", time.UTC)
	if !strings.Contains(out, "On Wed, Jun 17, 2026 at 12:29 AM, Alice wrote:") {
		t.Fatalf("expected reformatted attribution date, got %q", out)
	}

	// An empty date falls back to "an earlier date".
	out = formatQuotedMessageHTMLWithContent("Alice <a@example.com>", "", "<p>body</p>", time.UTC)
	if !strings.Contains(out, "On an earlier date, Alice wrote:") {
		t.Fatalf("expected 'an earlier date' fallback, got %q", out)
	}
}

func TestReplyInfoFromMessage_IncludeBody_DoesNotTreatHTMLAsPlain(t *testing.T) {
	htmlLikePlain := "<html><body>hi</body></html>"
	msg := &gmail.Message{
		ThreadId: "t1",
		Payload: &gmail.MessagePart{
			MimeType: "multipart/alternative",
			Headers: []*gmail.MessagePartHeader{
				{Name: "Message-ID", Value: "<m1>"},
				{Name: "From", Value: "sender@example.com"},
			},
			Parts: []*gmail.MessagePart{
				{
					MimeType: "text/plain",
					Body: &gmail.MessagePartBody{
						Data: base64.RawURLEncoding.EncodeToString([]byte(htmlLikePlain)),
					},
				},
				{
					MimeType: "text/html",
					Body: &gmail.MessagePartBody{
						Data: base64.RawURLEncoding.EncodeToString([]byte("<p>real html</p>")),
					},
				},
			},
		},
	}

	info := replyInfoFromMessage(msg, true)
	if info.Body != "" {
		t.Fatalf("expected plain Body to be empty (html-like), got %q", info.Body)
	}
	if info.BodyHTML == "" {
		t.Fatalf("expected BodyHTML to be set")
	}
}

func TestApplyQuoteToBodiesDerivesPlainReplyFromHTML(t *testing.T) {
	plain, html := applyQuoteToBodies(
		"",
		"<p>HTML <strong>reply</strong></p>",
		true,
		&replyInfo{
			FromAddr: "sender@example.com",
			Date:     "Mon, 1 Jan 2024 00:00:00 +0000",
			Body:     "Original plain",
			BodyHTML: "<p>Original HTML</p>",
		},
		time.UTC,
	)

	if !strings.Contains(plain, "HTML reply") {
		t.Fatalf("plain body omitted derived reply text: %q", plain)
	}
	if !strings.Contains(plain, "> Original plain") {
		t.Fatalf("plain body omitted quoted original: %q", plain)
	}
	if !strings.Contains(html, "<p>HTML <strong>reply</strong></p>") || !strings.Contains(html, "gmail_quote") {
		t.Fatalf("HTML body missing reply or quote: %q", html)
	}
}

func TestApplyQuoteToBodiesOmitsNonVisibleHTMLFromPlainReply(t *testing.T) {
	plain, _ := applyQuoteToBodies(
		"",
		`<!doctype html><html><head><title>Hidden title</title><style>.secret { color: red; }</style><script>alert("hidden")</script></head><body><span style="display:none">Hidden preheader</span><span hidden>Hidden attribute</span><span aria-hidden="true">Hidden aria</span><p>Visible reply</p></body></html>`,
		true,
		&replyInfo{
			Body:     "Original plain",
			BodyHTML: "<p>Original HTML</p>",
		},
		time.UTC,
	)

	if !strings.Contains(plain, "Visible reply") || !strings.Contains(plain, "> Original plain") {
		t.Fatalf("plain body missing visible reply or quote: %q", plain)
	}
	for _, hidden := range []string{"Hidden title", ".secret", `alert("hidden")`, "Hidden preheader", "Hidden attribute", "Hidden aria"} {
		if strings.Contains(plain, hidden) {
			t.Fatalf("plain body included non-visible HTML %q: %q", hidden, plain)
		}
	}
}

func TestApplyQuoteToBodiesDerivesPlainQuoteFromHTMLOriginal(t *testing.T) {
	plain, html := applyQuoteToBodies(
		"",
		"<p>HTML reply</p>",
		true,
		&replyInfo{
			FromAddr: "sender@example.com",
			BodyHTML: "<p>HTML-only original</p>",
		},
		time.UTC,
	)

	if !strings.Contains(plain, "HTML reply") || !strings.Contains(plain, "> HTML-only original") {
		t.Fatalf("plain body missing derived reply or quote: %q", plain)
	}
	if !strings.Contains(html, "<p>HTML reply</p>") || !strings.Contains(html, "HTML-only original") {
		t.Fatalf("HTML body missing reply or quote: %q", html)
	}
}

func TestApplyQuoteToBodiesKeepsImageOnlyReplyHTMLOnly(t *testing.T) {
	plain, html := applyQuoteToBodies(
		"",
		`<img src="cid:reply-image">`,
		true,
		&replyInfo{
			Body:     "Original plain",
			BodyHTML: "<p>Original HTML</p>",
		},
		time.UTC,
	)

	if strings.TrimSpace(plain) != "" {
		t.Fatalf("image-only HTML reply produced quote-only plain alternative: %q", plain)
	}
	if !strings.Contains(html, `cid:reply-image`) || !strings.Contains(html, "gmail_quote") {
		t.Fatalf("HTML body missing reply image or quote: %q", html)
	}
}
