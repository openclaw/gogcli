package cmd

import (
	"encoding/base64"
	"strings"
	"testing"
)

func encodeForwardPart(s string) map[string]any {
	data := base64.RawURLEncoding.EncodeToString([]byte(s))
	return map[string]any{"data": data, "size": len(data)}
}

func forwardOriginalHeaders(subject string) []map[string]any {
	return []map[string]any{
		{"name": "From", "value": "Alice <alice@example.com>"},
		{"name": "Date", "value": "Mon, 10 Mar 2026 09:00:00 -0400"},
		{"name": "Subject", "value": subject},
	}
}

func mockHTMLOnlyForwardSource() map[string]any {
	return map[string]any{
		"id": "orig-msg-1", "threadId": "thread-1",
		"payload": map[string]any{
			"mimeType": "text/html",
			"headers":  forwardOriginalHeaders("HTML original"),
			"body":     encodeForwardPart("<p>Quarterly <b>numbers</b> attached.</p>"),
		},
	}
}

func mockBlankPlainForwardSource() map[string]any {
	return map[string]any{
		"id": "orig-msg-1", "threadId": "thread-1",
		"payload": map[string]any{
			"mimeType": "multipart/alternative",
			"headers":  forwardOriginalHeaders("Blank plain original"),
			"parts": []map[string]any{
				{"mimeType": "text/plain", "body": encodeForwardPart(" \r\n")},
				{"mimeType": "text/html", "body": encodeForwardPart("<p>Quarterly <b>numbers</b> attached.</p>")},
			},
		},
	}
}

// When the original has no usable plain text, the forward's plain part carries
// the HTML body as text, as a reply's plain quote already does.
func TestGmailForward_PlainPartCarriesHTMLOnlyOriginal(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	for _, cmd := range forwardCommands {
		for _, src := range []struct {
			name   string
			source func() map[string]any
		}{
			{name: "HTML-only original", source: mockHTMLOnlyForwardSource},
			{name: "blank plain part", source: mockBlankPlainForwardSource},
		} {
			t.Run(cmd.name+"/"+src.name, func(t *testing.T) {
				plain, html := forwardTextParts(t, forwardArgs(cmd.verb), cmd.finalizePath, src.source)
				_, body, _ := strings.Cut(plain, forwardedSeparator)
				if !strings.Contains(body, "Quarterly numbers attached.") {
					t.Errorf("plain part lacks the original body:\n%s", plain)
				}
				if strings.Contains(body, "<b>") {
					t.Errorf("plain part carries raw HTML:\n%s", plain)
				}
				if !strings.Contains(html, "<p>Quarterly <b>numbers</b> attached.</p>") {
					t.Errorf("HTML part lacks the original:\n%s", html)
				}
			})
		}
	}
}
