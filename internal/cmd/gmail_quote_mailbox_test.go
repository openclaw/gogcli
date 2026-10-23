package cmd

import (
	"net/mail"
	"net/url"
	"strings"
	"testing"
)

func TestFormatQuoteMailboxHTMLEscapesNameAndAddress(t *testing.T) {
	got := formatQuoteMailboxHTML(`"<b>Eve</b>" <"x\"onclick=\"alert(1)"@example.com>`, plainName)
	want := isolatedHTML("&lt;b&gt;Eve&lt;/b&gt;") + " " +
		isolatedHTML(`&lt;<a href="mailto:%22x%5C%22onclick%3D%5C%22alert%281%29%22@example.com">&#34;x\&#34;onclick=\&#34;alert(1)&#34;@example.com</a>&gt;`)
	if got != want {
		t.Fatalf("formatQuoteMailboxHTML:\n got %q\nwant %q", got, want)
	}
}

func TestQuoteMailboxLinkResolvesToTheSameMailbox(t *testing.T) {
	for _, from := range []string{
		"first.last+tag@example.com",
		"user?bcc=copy@example.com",
		"support#us@example.com",
		"100%@example.com",
		"jörg@example.com",
		`Alice <"not@me"@example.org>`,
		`<"a,b"@example.com>`,
		`<"john smith"@example.com>`,
		`<"oh\\no"@example.org>`,
	} {
		t.Run(from, func(t *testing.T) {
			want, err := mail.ParseAddress(from)
			if err != nil {
				t.Fatal(err)
			}
			_, addrSpec, ok := parseQuoteMailbox(from)
			if !ok {
				t.Fatalf("parseQuoteMailbox(%q) failed", from)
			}
			href := mailtoURI(addrSpec)
			u, err := url.Parse(href)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", href, err)
			}
			if u.Scheme != "mailto" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.Opaque, ",") {
				t.Fatalf("%q: link carries more than the mailbox (query, fragment, or second address): %+v", href, u)
			}
			target, err := url.PathUnescape(u.Opaque)
			if err != nil {
				t.Fatalf("%q: %v", href, err)
			}
			got, err := mail.ParseAddress(target)
			if err != nil || got.Address != want.Address {
				t.Fatalf("%q targets %q (%v), want mailbox %q", href, target, err, want.Address)
			}
		})
	}
	for _, tc := range []struct{ in, want string }{
		// Ordinary addresses, including plus-addressed ones, keep a readable href.
		{"first.last+tag@example.com", "mailto:first.last+tag@example.com"},
		// The first two examples in RFC 6068 section 6.2.
		{`"not@me"@example.org`, "mailto:%22not%40me%22@example.org"},
		{`"oh\\no"@example.org`, "mailto:%22oh%5C%5Cno%22@example.org"},
	} {
		if got := mailtoURI(tc.in); got != tc.want {
			t.Fatalf("mailtoURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
