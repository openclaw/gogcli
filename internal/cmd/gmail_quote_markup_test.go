package cmd

import (
	"strings"
	"testing"
	"time"
)

// TestQuoteLinesKeepTheirDirection pins the exact direction markup of a
// reply's quote and a forward's header block around a Hebrew sender whose
// address starts with digits (see gmailQuoteOpen and isolateDirection). The
// quoted message passes through untouched, and the text/plain lines carry no
// direction marks.
func TestQuoteLinesKeepTheirDirection(t *testing.T) {
	const (
		sender   = "ישראל ישראלי <0501234567@example.com>"
		date     = "Tue, 20 Oct 2026 12:30:00 -0700"
		quoted   = `<div dir="ltr">Earlier reply<br></div><div class="gmail_quote gmail_quote_container"><div dir="ltr" class="gmail_attr">On … wrote:<br></div><blockquote class="gmail_quote">Original</blockquote></div>`
		open     = `<div class="gmail_quote gmail_quote_container"><div dir="ltr" class="gmail_attr">`
		name     = `<span dir="auto">ישראל ישראלי</span>`
		address  = `<span dir="auto">&lt;<a href="mailto:0501234567@example.com">0501234567@example.com</a>&gt;</span>`
		dateHTML = `<span dir="auto">Tue, Oct 20, 2026 at 12:30` + "\u202f" + `PM</span>`
	)
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reply", func(t *testing.T) {
		html := formatQuotedMessageHTMLWithContent(sender, date, quoted, la)
		want := open + "On " + dateHTML + " " + name + " " + address + ` wrote:</div><blockquote class="gmail_quote" style="margin:0 0 0 .8ex;border-left:1px #ccc solid;padding-left:1ex">` + quoted + `</blockquote></div>`
		if html != want {
			t.Errorf("reply quote:\n got %q\nwant %q", html, want)
		}
		plain := formatQuotedMessage(sender, date, "Original", la)
		if want := "On Tue, Oct 20, 2026 at 12:30\u202fPM ישראל ישראלי <0501234567@example.com> wrote:"; !strings.Contains(plain, "\n"+want+"\n") {
			t.Errorf("plain attribution:\n got %q\nwant line %q", plain, want)
		}
	})

	t.Run("forward", func(t *testing.T) {
		headers := forwardedMessageHeaders(sender, date, "שלום!", "דנה כהן <42@example.com>, Bob <bob@example.com>", "", la)
		html := formatForwardedMessageHTML("", composeSignature{}, headers, quoted)
		want := open + forwardedSeparator + "<br>" +
			`From: <strong class="gmail_sendername" dir="auto">ישראל ישראלי</strong> ` + address + "<br>" +
			"Date: " + dateHTML + "<br>" +
			`Subject: <span dir="auto">שלום!</span><br>` +
			`To: <span dir="auto">דנה כהן</span> <span dir="auto">&lt;<a href="mailto:42@example.com">42@example.com</a>&gt;</span>, ` +
			`<span dir="auto">Bob</span> <span dir="auto">&lt;<a href="mailto:bob@example.com">bob@example.com</a>&gt;</span><br>` +
			"</div><br><br>" + quoted + "</div>"
		if html != want {
			t.Errorf("forward block:\n got %q\nwant %q", html, want)
		}
		plain := formatForwardedMessage("", composeSignature{}, headers, "Original")
		for _, line := range []string{"From: ישראל ישראלי <0501234567@example.com>", "Subject: שלום!", "To: דנה כהן <42@example.com>, Bob <bob@example.com>"} {
			if !strings.Contains(plain, "\n"+line+"\n") {
				t.Errorf("text/plain has no line %q:\n%s", line, plain)
			}
		}
	})
}

// The helpers below build expected HTML for a reply's attribution line or a
// forward's header block, independently of the code under test;
// TestQuoteLinesKeepTheirDirection pins the same markup literally. Their
// arguments are already escaped.

// isolatedHTML is text from the original (a name, a date, a subject, a value
// that doesn't parse) keeping its own direction.
func isolatedHTML(text string) string { return `<span dir="auto">` + text + "</span>" }

// forwardSenderNameHTML is a forwarded message's sender name, in bold.
func forwardSenderNameHTML(name string) string {
	return `<strong class="gmail_sendername" dir="auto">` + name + "</strong>"
}

// quoteAddressHTML is an address linked in brackets. It links the address as
// is, so it suits only addresses with nothing to percent-encode.
func quoteAddressHTML(address string) string {
	return isolatedHTML(`&lt;<a href="mailto:` + address + `">` + address + "</a>&gt;")
}
