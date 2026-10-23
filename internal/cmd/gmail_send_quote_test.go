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
		"\n\nOn Mon, Jan 1, 2024 at 12:00\u202fAM Alice <a@example.com> wrote:\n",
		"> l1\n",
		"> l2\n",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Fatalf("expected %q in output, got %q", s, got)
		}
	}
}

func TestQuoteAttribution(t *testing.T) {
	const date = "Fri, 14 Aug 2026 16:01:00 -0700"
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		from      string
		date      string
		wantPlain string
		wantHTML  string
	}{
		{
			name:      "named sender",
			from:      `"Example Sender" <sender@example.com>`,
			date:      date,
			wantPlain: "On Fri, Aug 14, 2026 at 4:01\u202fPM Example Sender <sender@example.com> wrote:",
			wantHTML:  "On " + isolatedHTML("Fri, Aug 14, 2026 at 4:01\u202fPM") + " " + isolatedHTML("Example Sender") + " " + quoteAddressHTML("sender@example.com") + " wrote:",
		},
		{
			name:      "nameless sender",
			from:      "sender@example.com",
			date:      date,
			wantPlain: "On Fri, Aug 14, 2026 at 4:01\u202fPM <sender@example.com> wrote:",
			wantHTML:  "On " + isolatedHTML("Fri, Aug 14, 2026 at 4:01\u202fPM") + " " + quoteAddressHTML("sender@example.com") + " wrote:",
		},
		{
			name:      "no date",
			from:      "Example Sender <sender@example.com>",
			wantPlain: "Example Sender <sender@example.com> wrote:",
			wantHTML:  isolatedHTML("Example Sender") + " " + quoteAddressHTML("sender@example.com") + " wrote:",
		},
		{
			name:      "no sender",
			date:      date,
			wantPlain: "Original message:",
			wantHTML:  "Original message:",
		},
		{
			name:      "blank sender",
			from:      "  ",
			date:      date,
			wantPlain: "Original message:",
			wantHTML:  "Original message:",
		},
		{
			name:      "quoted local part keeps its quotes",
			from:      `Alice <"john smith"@example.com>`,
			date:      date,
			wantPlain: "On Fri, Aug 14, 2026 at 4:01\u202fPM Alice <\"john smith\"@example.com> wrote:",
			wantHTML: "On " + isolatedHTML("Fri, Aug 14, 2026 at 4:01\u202fPM") + " " + isolatedHTML("Alice") + " " +
				isolatedHTML(`&lt;<a href="mailto:%22john%20smith%22@example.com">&#34;john smith&#34;@example.com</a>&gt;`) + " wrote:",
		},
		{
			name:      "unparseable sender shown as is, unlinked",
			from:      "Example Sender, Team <sender@example.com>",
			date:      date,
			wantPlain: "On Fri, Aug 14, 2026 at 4:01\u202fPM Example Sender, Team <sender@example.com> wrote:",
			wantHTML:  "On " + isolatedHTML("Fri, Aug 14, 2026 at 4:01\u202fPM") + " " + isolatedHTML("Example Sender, Team &lt;sender@example.com&gt;") + " wrote:",
		},
		{
			name:      "markup in an unparseable sender and date is escaped",
			from:      `"><script>alert(1)</script>`,
			date:      "<b>bad</b>",
			wantPlain: `On <b>bad</b> "><script>alert(1)</script> wrote:`,
			wantHTML:  "On " + isolatedHTML("&lt;b&gt;bad&lt;/b&gt;") + " " + isolatedHTML("&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;") + " wrote:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each want is the whole attribution line, so anchor it at both ends.
			if got := formatQuotedMessage(tc.from, tc.date, "body", la); !strings.Contains(got, "\n"+tc.wantPlain+"\n") {
				t.Errorf("plain attribution:\n got %q\nwant line %q", got, tc.wantPlain)
			}
			if got := formatQuotedMessageHTMLWithContent(tc.from, tc.date, "<p>body</p>", la); !strings.Contains(got, gmailQuoteOpen+tc.wantHTML+"</div>") {
				t.Errorf("HTML attribution:\n got %q\nwant line %q", got, tc.wantHTML)
			}
		})
	}
}

func TestFormatQuoteDate(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		loc  *time.Location
		want string
	}{
		{"rfc2822 in utc", "Wed, 17 Jun 2026 00:29:51 +0000", time.UTC, "Wed, Jun 17, 2026 at 12:29\u202fAM"},
		{"converts offset into loc", "Wed, 17 Jun 2026 00:29:51 -0500", time.UTC, "Wed, Jun 17, 2026 at 5:29\u202fAM"},
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
	if !strings.Contains(html, "<p>HTML <strong>reply</strong></p>") || !strings.Contains(html, gmailQuoteOpen) {
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
	if !strings.Contains(html, `cid:reply-image`) || !strings.Contains(html, gmailQuoteOpen) {
		t.Fatalf("HTML body missing reply image or quote: %q", html)
	}
}
