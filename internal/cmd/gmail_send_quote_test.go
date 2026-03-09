package cmd

import (
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestFormatQuotedMessage(t *testing.T) {
	got := formatQuotedMessage("Alice <a@example.com>", "Mon, 1 Jan 2024 00:00:00 +0000", "l1\nl2")
	wantContains := []string{
		"\n\nOn Mon, 1 Jan 2024 00:00:00 +0000, Alice <a@example.com> wrote:\n",
		"> l1\n",
		"> l2\n",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Fatalf("expected %q in output, got %q", s, got)
		}
	}
}

func TestFormatQuotedMessageHTMLWithContent_EscapesHeader_NotBody(t *testing.T) {
	out := formatQuotedMessageHTMLWithContent(`"><script>alert(1)</script>`, `<b>bad</b>`, `<b>ok</b>`)
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
