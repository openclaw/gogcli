package cmd

import (
	"encoding/base64"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestCollectAttachments(t *testing.T) {
	p := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{
				Filename: "a.txt",
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{AttachmentId: "att1", Size: 123},
			},
			{
				MimeType: "image/png",
				Body:     &gmail.MessagePartBody{AttachmentId: "att-inline", Size: 42},
			},
			{
				Parts: []*gmail.MessagePart{
					{
						Filename: "b.pdf",
						MimeType: "application/pdf",
						Body:     &gmail.MessagePartBody{AttachmentId: "att2", Size: 456},
					},
				},
			},
		},
	}
	atts := collectAttachments(p)
	if len(atts) != 3 {
		t.Fatalf("unexpected: %#v", atts)
	}
	if atts[0].AttachmentID == "" || atts[1].AttachmentID == "" {
		t.Fatalf("missing attachment ids: %#v", atts)
	}
	if atts[1].Filename != "attachment" {
		t.Fatalf("expected fallback filename, got: %#v", atts[1])
	}
}

func TestBestBodyTextPrefersPlain(t *testing.T) {
	plain := base64.RawURLEncoding.EncodeToString([]byte("plain"))
	html := base64.RawURLEncoding.EncodeToString([]byte("<b>html</b>"))
	p := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{MimeType: "text/html", Body: &gmail.MessagePartBody{Data: html}},
			{MimeType: "text/plain", Body: &gmail.MessagePartBody{Data: plain}},
		},
	}
	if got := bestBodyText(p); got != "plain" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestDecodeBase64URL(t *testing.T) {
	got, err := decodeBase64URL(base64.RawURLEncoding.EncodeToString([]byte("ok")))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "ok" {
		t.Fatalf("unexpected: %q", got)
	}
	if _, err := decodeBase64URL("!!!"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic HTML tags",
			input: "<p>Hello</p>",
			want:  "Hello",
		},
		{
			name:  "script block removed",
			input: "<script>alert(1)</script>text",
			want:  "text",
		},
		{
			name:  "style block removed",
			input: "<style>body{color:red}</style>content",
			want:  "content",
		},
		{
			name:  "nested tags",
			input: "<div><span>text</span></div>",
			want:  "text",
		},
		{
			name:  "plain text unchanged",
			input: "plain text",
			want:  "plain text",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace collapsed",
			input: "<p>hello</p>   <p>world</p>",
			want:  "hello world",
		},
		{
			name:  "complex HTML email",
			input: "<html><head><style>.foo{}</style></head><body><p>Hi there</p></body></html>",
			want:  "Hi there",
		},
		{
			name:  "script with attributes",
			input: `<script type="text/javascript">var x=1;</script>safe`,
			want:  "safe",
		},
		{
			name:  "multiline style block",
			input: "<style>\n  body { margin: 0; }\n  p { color: blue; }\n</style>visible",
			want:  "visible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBestBodyForDisplay(t *testing.T) {
	p := &gmail.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body: &gmail.MessagePartBody{
					Data: encodeBase64URL("plain body"),
				},
			},
			{
				MimeType: "text/html",
				Body: &gmail.MessagePartBody{
					Data: encodeBase64URL("<p>html body</p>"),
				},
			},
		},
	}

	body, isHTML := bestBodyForDisplay(p)
	if body != "plain body" || isHTML {
		t.Fatalf("expected plain body, got %q (html=%v)", body, isHTML)
	}

	htmlOnly := &gmail.MessagePart{
		MimeType: "text/html",
		Body: &gmail.MessagePartBody{
			Data: encodeBase64URL("<p>html body</p>"),
		},
	}
	body, isHTML = bestBodyForDisplay(htmlOnly)
	if body == "" || !isHTML {
		t.Fatalf("expected html body, got %q (html=%v)", body, isHTML)
	}
}

func encodeBase64URL(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
