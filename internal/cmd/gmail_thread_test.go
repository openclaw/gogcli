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

func TestBestBodyText_MimeTypeWithParams(t *testing.T) {
	plain := base64.RawURLEncoding.EncodeToString([]byte("plain"))
	p := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{MimeType: "text/plain; charset=\"utf-8\"", Body: &gmail.MessagePartBody{Data: plain}},
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
	got, err = decodeBase64URL(base64.URLEncoding.EncodeToString([]byte("ok")))
	if err != nil {
		t.Fatalf("err padded: %v", err)
	}
	if got != "ok" {
		t.Fatalf("unexpected padded: %q", got)
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

func TestExtractTextFromHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic HTML",
			input: "<p>Hello world</p>",
			want:  "Hello world",
		},
		{
			name:  "script block removed",
			input: "<script>alert('xss')</script>safe text",
			want:  "safe text",
		},
		{
			name:  "style block removed",
			input: "<style>body{color:red}</style>visible",
			want:  "visible",
		},
		{
			name:  "nested tags",
			input: "<div><span>inner</span></div>",
			want:  "inner",
		},
		{
			name:  "block elements add spaces",
			input: "<p>first</p><p>second</p>",
			want:  "first second",
		},
		{
			name:  "malformed HTML consumed safely",
			input: `<a href="https://evil.com/login>Click here</a>`,
			want:  "",
		},
		{
			name:  "entities decoded by tokenizer",
			input: "<p>a &amp; b</p>",
			want:  "a & b",
		},
		{
			name:  "complex email HTML",
			input: `<html><head><style>.x{}</style></head><body><div>Hello</div><script>track()</script><p>World</p></body></html>`,
			want:  "Hello World",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "plain text unchanged",
			input: "no tags here",
			want:  "no tags here",
		},
		{
			name:  "self closing tags",
			input: "line1<br/>line2",
			want:  "line1 line2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeExtractTextFromHTML(tt.input)
			if got != tt.want {
				t.Errorf("safeExtractTextFromHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "http URL",
			input: "visit http://example.com for info",
			want:  "visit [url removed] for info",
		},
		{
			name:  "https URL",
			input: "click https://example.com/page",
			want:  "click [url removed]",
		},
		{
			name:  "URL with query params",
			input: "track https://track.example.com/open?id=abc123&utm_source=email here",
			want:  "track [url removed] here",
		},
		{
			name:  "multiple URLs",
			input: "see https://a.com and http://b.com ok",
			want:  "see [url removed] and [url removed] ok",
		},
		{
			name:  "no URLs unchanged",
			input: "plain text with no links",
			want:  "plain text with no links",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "URL at start",
			input: "https://evil.com/phish is bad",
			want:  "[url removed] is bad",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripURLs(tt.input)
			if got != tt.want {
				t.Errorf("stripURLs(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeBodyText(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		isHTML bool
		want   string
	}{
		{
			name:   "HTML with URL",
			body:   `<p>Click <a href="https://evil.com">here</a> now</p>`,
			isHTML: true,
			want:   "Click here now",
		},
		{
			name:   "plain text with URL",
			body:   "Visit https://example.com for details",
			isHTML: false,
			want:   "Visit [url removed] for details",
		},
		{
			name:   "HTML entities decoded then URL stripped",
			body:   "check &#104;ttps://evil.com/payload here",
			isHTML: false,
			want:   "check [url removed] here",
		},
		{
			name:   "HTML with script and tracking",
			body:   `<script>fetch('https://track.com/pixel')</script><p>Hello https://phish.com</p>`,
			isHTML: true,
			want:   "Hello [url removed]",
		},
		{
			name:   "empty body",
			body:   "",
			isHTML: false,
			want:   "",
		},
		{
			name:   "plain text no URLs",
			body:   "Just a normal message",
			isHTML: false,
			want:   "Just a normal message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBodyText(tt.body, tt.isHTML)
			if got != tt.want {
				t.Errorf("sanitizeBodyText(%q, %v) = %q, want %q", tt.body, tt.isHTML, got, tt.want)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "URL in subject",
			input: "Check https://evil.com now",
			want:  "Check [url removed] now",
		},
		{
			name:  "HTML entity decoded",
			input: "a &amp; b",
			want:  "a & b",
		},
		{
			name:  "no changes needed",
			input: "Normal Subject Line",
			want:  "Normal Subject Line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeText(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClearPayloadBodies(t *testing.T) {
	p := &gmail.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: "c29tZSBkYXRh"},
			},
			{
				MimeType: "text/html",
				Body:     &gmail.MessagePartBody{Data: "PHA-aHRtbDwvcD4"},
			},
			{
				MimeType: "image/png",
				Body:     &gmail.MessagePartBody{Data: "imagedata", AttachmentId: "att1"},
			},
		},
	}
	clearPayloadBodies(p)

	if p.Parts[0].Body.Data != "" {
		t.Errorf("text/plain body should be cleared, got %q", p.Parts[0].Body.Data)
	}
	if p.Parts[1].Body.Data != "" {
		t.Errorf("text/html body should be cleared, got %q", p.Parts[1].Body.Data)
	}
	if p.Parts[2].Body.Data != "imagedata" {
		t.Errorf("image/png body should be preserved, got %q", p.Parts[2].Body.Data)
	}
}

func encodeBase64URL(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
