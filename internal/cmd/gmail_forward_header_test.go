package cmd

import (
	"strings"
	"testing"
	"time"
)

// forwardHeaderBlocks renders a forward of an original with these headers in
// both parts, with no note and a one-line body.
func forwardHeaderBlocks(from, date, subject, to, cc string) (plain, html string) {
	headers := forwardedMessageHeaders(from, date, subject, to, cc, time.UTC)
	plain = formatForwardedMessage("", composeSignature{}, headers, "Body.")
	html = formatForwardedMessageHTML("", composeSignature{}, headers, "<p>Body.</p>")
	return plain, html
}

// TestForwardedHeaderLines checks each header line as a reader sees it in both
// parts. The inputs are header values as the Gmail API returns them.
func TestForwardedHeaderLines(t *testing.T) {
	const (
		from    = `"Sender, Example" <sender@example.com>`
		date    = "Mon, 10 Mar 2026 09:00:00 -0400"
		subject = "Original Subject"
		to      = `Alice Example <alice@example.com>, bob@example.com, "Carol, Quoted" <carol@example.org>`
		cc      = `"Dave, Encoded" <dave@example.com>, <erin@example.com>`
	)
	cases := []struct {
		name                string
		from, date, subject string
		to, cc              string
		wantPlain, wantHTML []string // whole lines
		absent              []string // labels with no line in either part
	}{
		{
			name: "named sender and several recipients",
			from: from, subject: subject, to: to, cc: cc,
			wantPlain: []string{
				"From: Sender, Example <sender@example.com>",
				"To: Alice Example <alice@example.com>, <bob@example.com>, Carol, Quoted <carol@example.org>",
				"Cc: Dave, Encoded <dave@example.com>, <erin@example.com>",
			},
			wantHTML: []string{
				`From: <strong class="gmail_sendername">Sender, Example</strong> &lt;<a href="mailto:sender@example.com">sender@example.com</a>&gt;`,
				`To: Alice Example &lt;<a href="mailto:alice@example.com">alice@example.com</a>&gt;, &lt;<a href="mailto:bob@example.com">bob@example.com</a>&gt;, Carol, Quoted &lt;<a href="mailto:carol@example.org">carol@example.org</a>&gt;`,
				`Cc: Dave, Encoded &lt;<a href="mailto:dave@example.com">dave@example.com</a>&gt;, &lt;<a href="mailto:erin@example.com">erin@example.com</a>&gt;`,
			},
		},
		{
			name: "nameless sender, no Cc",
			from: "sender@example.com", subject: subject, to: "recipient@example.com",
			wantPlain: []string{"From: <sender@example.com>", "To: <recipient@example.com>"},
			wantHTML: []string{
				`From: &lt;<a href="mailto:sender@example.com">sender@example.com</a>&gt;`,
				`To: &lt;<a href="mailto:recipient@example.com">recipient@example.com</a>&gt;`,
			},
			absent: []string{"Cc"},
		},
		{
			name: "groups list their members, as in Gmail",
			from: from, subject: subject,
			to: "Team:a@example.com, b@example.com;, c@example.com",
			cc: "undisclosed-recipients:;, John Smith <john@example.com>",
			wantPlain: []string{
				"To: <a@example.com>, <b@example.com>, <c@example.com>",
				"Cc: John Smith <john@example.com>",
			},
			wantHTML: []string{
				`To: &lt;<a href="mailto:a@example.com">a@example.com</a>&gt;, &lt;<a href="mailto:b@example.com">b@example.com</a>&gt;, &lt;<a href="mailto:c@example.com">c@example.com</a>&gt;`,
				`Cc: John Smith &lt;<a href="mailto:john@example.com">john@example.com</a>&gt;`,
			},
		},
		{
			// Gmail Web keeps the line and leaves it empty.
			name: "a group with no members leaves the line empty",
			from: from, subject: subject, to: "undisclosed-recipients:;", cc: " ",
			wantPlain: []string{"To: "},
			wantHTML:  []string{"To: "},
			absent:    []string{"Cc"},
		},
		{
			// The Gmail API turns a CRLF inside an encoded display name into ", "
			// and leaves the name unquoted, so these do not parse.
			name: "unparseable values are shown trimmed but as is, unlinked",
			from: "Evil, Name <evil@example.com>", subject: subject,
			to: "  Evil, Name <evil@example.com>, other@example.com ",
			wantPlain: []string{
				"From: Evil, Name <evil@example.com>",
				"To: Evil, Name <evil@example.com>, other@example.com",
			},
			wantHTML: []string{
				"From: Evil, Name &lt;evil@example.com&gt;",
				"To: Evil, Name &lt;evil@example.com&gt;, other@example.com",
			},
		},
		{
			// An unparseable Date is shown as it came, so it is escaped too.
			name: "names, date, subject and addresses are escaped in HTML",
			from: `"<b>Eve</b>" <eve@example.com>`, date: "<b>soon</b> & later", subject: "<i>Hi</i> & co",
			to: `<"john smith"@example.com>, "R&D <Team>" <user?bcc=x@example.com>`,
			wantPlain: []string{
				"From: <b>Eve</b> <eve@example.com>",
				"Date: <b>soon</b> & later",
				"Subject: <i>Hi</i> & co",
				`To: <"john smith"@example.com>, R&D <Team> <user?bcc=x@example.com>`,
			},
			wantHTML: []string{
				`From: <strong class="gmail_sendername">&lt;b&gt;Eve&lt;/b&gt;</strong> &lt;<a href="mailto:eve@example.com">eve@example.com</a>&gt;`,
				"Date: &lt;b&gt;soon&lt;/b&gt; &amp; later",
				"Subject: &lt;i&gt;Hi&lt;/i&gt; &amp; co",
				`To: &lt;<a href="mailto:%22john%20smith%22@example.com">&#34;john smith&#34;@example.com</a>&gt;, R&amp;D &lt;Team&gt; &lt;<a href="mailto:user%3Fbcc%3Dx@example.com">user?bcc=x@example.com</a>&gt;`,
			},
		},
		{
			name: "a blank display name counts as none",
			from: `" " <sender@example.com>`, subject: subject, to: `" " <recipient@example.com>`,
			wantPlain: []string{"From: <sender@example.com>", "To: <recipient@example.com>"},
			wantHTML: []string{
				`From: &lt;<a href="mailto:sender@example.com">sender@example.com</a>&gt;`,
				`To: &lt;<a href="mailto:recipient@example.com">recipient@example.com</a>&gt;`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.date == "" {
				tc.date = date
			}
			plain, html := forwardHeaderBlocks(tc.from, tc.date, tc.subject, tc.to, tc.cc)
			for _, want := range tc.wantPlain {
				if !strings.Contains(plain, "\n"+want+"\n") {
					t.Errorf("text/plain has no line %q:\n%s", want, plain)
				}
			}
			for _, want := range tc.wantHTML {
				if !strings.Contains(html, "<br>"+want+"<br>") {
					t.Errorf("text/html has no line %q:\n%s", want, html)
				}
			}
			for _, label := range tc.absent {
				if strings.Contains(plain, "\n"+label+":") || strings.Contains(html, "<br>"+label+":") {
					t.Errorf("unexpected %s line:\n%s\n%s", label, plain, html)
				}
			}
		})
	}
}

// TestForwardedHeaderLayout checks the block's layout: the separator runs
// straight into the headers, which carry no styling beyond the sender's name
// in bold, and two blank lines separate them from a body.
func TestForwardedHeaderLayout(t *testing.T) {
	t.Run("with a body", func(t *testing.T) {
		plain, html := forwardHeaderBlocks("Alice <alice@example.com>", "Mon, 10 Mar 2026 09:00:00 -0400",
			"Original Subject", "Bob <bob@example.com>", "carol@example.com")

		// Gmail's separator text is pinned here; other tests find the block by
		// forwardedSeparator.
		if !strings.HasPrefix(plain, "---------- Forwarded message ---------\nFrom: Alice <alice@example.com>\n") {
			t.Errorf("text/plain separator is not followed by the From line:\n%s", plain)
		}
		if !strings.HasSuffix(plain, "\nCc: <carol@example.com>\n\n\nBody.\n") {
			t.Errorf("text/plain wants two blank lines between the headers and the body:\n%q", plain)
		}

		const open = `<div class="gmail_quote">`
		block, body, found := strings.Cut(strings.TrimPrefix(html, open), "</div>")
		if !strings.HasPrefix(html, open) || !found {
			t.Fatalf("text/html has no header block:\n%s", html)
		}
		if !strings.HasPrefix(block, "<div>"+forwardedSeparator+"<br>From: ") {
			t.Errorf("text/html separator is not followed by the From line:\n%s", block)
		}
		// Check only the header block: the original's own markup may be bold or styled.
		for _, markup := range []string{"<b>", "style=", "color"} {
			if strings.Contains(block, markup) {
				t.Errorf("text/html header block contains %q:\n%s", markup, block)
			}
		}
		if strings.Count(block, "<strong") != 1 || !strings.Contains(block, `From: <strong class="gmail_sendername">Alice</strong> &lt;`) {
			t.Errorf("text/html wants only the sender's name in bold:\n%s", block)
		}
		if body != "<br><br><p>Body.</p></div>" {
			t.Errorf("text/html wants two blank lines between the headers and the body, got %q", body)
		}
	})

	// An original with no body text leaves no blank lines after the headers.
	t.Run("without a body", func(t *testing.T) {
		headers := forwardedMessageHeaders("a@example.com", "", "Subject", "", "", time.UTC)
		if plain, want := formatForwardedMessage("", composeSignature{}, headers, ""), forwardedSeparator+"\nFrom: <a@example.com>\nSubject: Subject\n"; plain != want {
			t.Errorf("text/plain = %q, want %q", plain, want)
		}
		if html := formatForwardedMessageHTML("", composeSignature{}, headers, ""); !strings.HasSuffix(html, "Subject: Subject<br></div></div>") {
			t.Errorf("text/html ends with blank lines: %q", html)
		}
	})
}

// TestGmailForward_HeaderBlockLikeGmail runs both forward commands and checks
// that the original's headers reach both decoded parts of the message they
// build: the plain block exactly, as a plain-text reader sees it, and in HTML
// each header line in order inside the block, then the original's body.
func TestGmailForward_HeaderBlockLikeGmail(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	source := func() map[string]any {
		msg := mockOriginalMessage(false)
		payload := msg["payload"].(map[string]any)
		payload["headers"] = []map[string]any{
			{"name": "From", "value": `"Sender, Example" <sender@example.com>`},
			{"name": "To", "value": `Alice Example <alice@example.com>, bob@example.com`},
			{"name": "Cc", "value": `"Carol, Quoted" <carol@example.org>`},
			{"name": "Date", "value": "Mon, 10 Mar 2026 09:00:00 -0400"},
			{"name": "Subject", "value": "Original Subject"},
		}
		return msg
	}
	wantPlain := forwardedSeparator + "\n" +
		"From: Sender, Example <sender@example.com>\n" +
		"Date: Tue, Mar 10, 2026 at 1:00\u202fPM\n" +
		"Subject: Original Subject\n" +
		"To: Alice Example <alice@example.com>, <bob@example.com>\n" +
		"Cc: Carol, Quoted <carol@example.org>\n\n\n" +
		"Hello, this is the body."
	wantHTML := []string{
		`From: <strong class="gmail_sendername">Sender, Example</strong> &lt;<a href="mailto:sender@example.com">sender@example.com</a>&gt;`,
		"Date: Tue, Mar 10, 2026 at 1:00\u202fPM",
		"Subject: Original Subject",
		`To: Alice Example &lt;<a href="mailto:alice@example.com">alice@example.com</a>&gt;, &lt;<a href="mailto:bob@example.com">bob@example.com</a>&gt;`,
		`Cc: Carol, Quoted &lt;<a href="mailto:carol@example.org">carol@example.org</a>&gt;`,
	}
	for _, cmd := range forwardCommands {
		t.Run(cmd.name, func(t *testing.T) {
			plain, html := forwardTextParts(t, forwardArgs(cmd.verb, "--note", "FYI"), cmd.finalizePath, source)
			if !strings.Contains(plain, wantPlain) {
				t.Errorf("text/plain header block:\n got %q\nwant %q", plain, wantPlain)
			}
			_, rest, found := strings.Cut(html, "<div>"+forwardedSeparator)
			block, after, closed := strings.Cut(rest, "</div>")
			if !found || !closed {
				t.Fatalf("text/html has no header block:\n%s", html)
			}
			for _, line := range wantHTML {
				_, next, found := strings.Cut(block, "<br>"+line+"<br>")
				if !found {
					t.Fatalf("text/html header block has no line %q after the lines before it:\n%s", line, html)
				}
				block = "<br>" + next
			}
			if block != "<br>" || !strings.HasPrefix(after, "<br><br><p>Hello, this is the body.</p>") {
				t.Errorf("text/html wants the header block, two blank lines, then the original's body:\n%s", html)
			}
		})
	}
}
