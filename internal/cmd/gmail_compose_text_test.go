package cmd

import (
	"strings"
	"testing"
	"time"
)

// A quoted reply given only plain text keeps that text exactly in the plain
// part, and shows it escaped, trimmed and with its line breaks kept in the HTML
// part, through the same composeText rules a forward note uses.
func TestApplyQuoteToBodiesShowsPlainReplyInBothParts(t *testing.T) {
	const body = "Thanks.\n\nSecond <b> line.\n"
	plain, html := applyQuoteToBodies(body, "", true, &replyInfo{Body: "Original."}, time.UTC)
	if !strings.HasPrefix(plain, body+"\n\n") {
		t.Errorf("plain part should open with the reply exactly as given; got:\n%q", plain)
	}
	const wantHTML = "Thanks.<br>\n<br>\nSecond &lt;b&gt; line."
	if !strings.HasPrefix(html, wantHTML) || strings.HasPrefix(html, wantHTML+"<br>\n") {
		t.Errorf("HTML part should open with the escaped, trimmed reply; want prefix %q, got:\n%q", wantHTML, html)
	}
}
