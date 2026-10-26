package cmd

import (
	"strings"
	"testing"
	"time"
)

// A quoted reply given only plain text keeps that text exactly in the plain
// part, and shows it escaped and trimmed, a line to a div, in the HTML part,
// through the same composeText rules a forward note uses.
func TestApplyQuoteToBodiesShowsPlainReplyInBothParts(t *testing.T) {
	const body = "Thanks.\n\nSecond <b> line.\n"
	plain, html := applyQuoteToBodies(body, "", true, &replyInfo{Body: "Original."}, time.UTC)
	if !strings.HasPrefix(plain, body+"\n\n") {
		t.Errorf("plain part should open with the reply exactly as given; got:\n%q", plain)
	}
	const wantHTML = "<div dir=\"auto\">Thanks.</div>\n<div dir=\"auto\"><br></div>\n<div dir=\"auto\">Second &lt;b&gt; line.</div><br>"
	if !strings.HasPrefix(html, wantHTML+gmailQuoteOpen) {
		t.Errorf("HTML part should open with the escaped, trimmed reply, a div per line, then the quote; want prefix %q, got:\n%q", wantHTML, html)
	}
}

// A quoted reply with no text, such as an attachment-only send --quote, has no
// block above the quote, so it takes two <br> there, as a document does.
func TestApplyQuoteToBodiesWithoutTextOpensWithTwoBreaks(t *testing.T) {
	_, html := applyQuoteToBodies("", "", true, &replyInfo{Body: "Original."}, time.UTC)
	if !strings.HasPrefix(html, "<br><br>"+gmailQuoteOpen) {
		t.Errorf("HTML part should open with two <br> and the quote; got:\n%q", html)
	}
}

// TestComposedTextKeepsItsOwnDirection checks what comes before the quote in
// the HTML part of a sent message, a draft and a reply: plain text a line to a
// <div dir="auto">, given HTML in one, each exactly once, a signature in a
// block of its own after it, then one <br> before the quote. A whole HTML
// document isn't wrapped and gets two.
func TestComposedTextKeepsItsOwnDirection(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	const (
		plain     = "\r\nHi Dana,\r\n \r\nתודה! <PDF> עד יום שלישי.\r\n" // as a text file from Windows might have it
		plainHTML = "<div dir=\"auto\">Hi Dana,</div>\n<div dir=\"auto\"><br></div>\n<div dir=\"auto\">תודה! &lt;PDF&gt; עד יום שלישי.</div>"
		given     = "\n<p>שלום <b>דנה</b>!</p>\n"
		givenHTML = `<div dir="auto"><p>שלום <b>דנה</b>!</p></div>`
		document  = "<!DOCTYPE html><html><body><p>שלום</p></body></html>"
		signature = `<div dir="auto" class="gmail_signature"><div>Kind regards<br>Me Person</div></div>`
	)
	cases := []struct {
		name   string
		args   []string
		quoted bool
		want   string // the HTML part above the quote, or all of it when there's none
	}{
		{"send", []string{"send", "--to", "a@example.com", "--subject", "Hi", "--body-html", given, "--signature"}, false, givenHTML + "\n\n" + signature},
		{"drafts create, plain body", []string{"drafts", "create", "--to", "a@example.com", "--subject", "Hi", "--body", plain, "--signature"}, false, plainHTML + "\n\n" + signature},
		{"send --quote, plain body", []string{"send", "--to", "a@example.com", "--reply-to-message-id", "orig-msg-1", "--quote", "--body", plain}, true, plainHTML + "<br>"},
		{"reply", []string{"reply", "orig-msg-1", "--body-html", given, "--signature"}, true, givenHTML + "\n\n" + signature + "<br>"},
		{"reply, HTML document", []string{"reply", "orig-msg-1", "--body-html", document}, true, document + "<br><br>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, got, cleanup := newSignatureComposeService(t, primarySignature(), nil)
			defer cleanup()
			args := append([]string{"--account", "me@example.com", "gmail"}, tc.args...)
			if result := executeWithGmailTestService(t, args, svc); result.err != nil {
				t.Fatal(result.err)
			}
			above, _, quoted := strings.Cut(composedHTML(t, got.raw), gmailQuoteOpen)
			if quoted != tc.quoted {
				t.Errorf("quote in the HTML part = %v, want %v", quoted, tc.quoted)
			}
			if above != tc.want {
				t.Errorf("HTML part above the quote:\n got %q\nwant %q", above, tc.want)
			}
		})
	}
}

// A whole HTML document, or HTML that opens with what belongs in a document's
// head, isn't wrapped; anything else is a fragment, wrapped once and trimmed,
// including a <header> element or text before an <html> tag.
func TestHTMLDocumentsAreNotWrapped(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string // the HTML part; "" for the HTML as given
	}{
		{"doctype", "<!DOCTYPE html><html><body><p>Hi</p></body></html>", ""},
		{"XML prolog", `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE html><html><body><p>Hi</p></body></html>`, ""},
		{"html", "<html><body><p>Hi</p></body></html>", ""},
		{"head, in capitals", "<HEAD><title>Hi</title></HEAD><p>Hi</p>", ""},
		{"body after a byte order mark and a comment", byteOrderMark + "<!-- template -->\n<body><p>Hi</p></body>", ""},
		{"style", "<style>p { color: red; }</style><p>Hi</p>", ""},
		{"meta", `<meta charset="utf-8"><p>Hi</p>`, ""},
		{"self-closing link", `<link rel="stylesheet" href="https://example.com/mail.css"/><p>Hi</p>`, ""},
		{"header element", "<header>Hi</header><p>Body</p>", `<div dir="auto"><header>Hi</header><p>Body</p></div>`},
		{"text before an html tag", "Hi <html><body>there</body></html>", `<div dir="auto">Hi <html><body>there</body></html></div>`},
		{"paragraph from a file with a byte order mark", byteOrderMark + "<p>Hi</p>\n", `<div dir="auto"><p>Hi</p></div>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, got, cleanup := newSignatureComposeService(t, nil, nil)
			defer cleanup()
			args := []string{"--account", "me@example.com", "gmail", "drafts", "create", "--to", "a@example.com", "--subject", "Hi", "--body-html", tc.html}
			if result := executeWithGmailTestService(t, args, svc); result.err != nil {
				t.Fatal(result.err)
			}
			want := tc.want
			if want == "" {
				want = tc.html
			}
			if html := composedHTML(t, got.raw); html != want {
				t.Errorf("HTML part:\n got %q\nwant %q", html, want)
			}
		})
	}
}
