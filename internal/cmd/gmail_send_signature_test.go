package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailSendCmd_SignaturePlain(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var gotRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/a@b.com":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "a@b.com",
				"signature":          "<div>Sig</div>",
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			var payload struct {
				Raw string `json:"raw"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			decoded, err := base64.RawURLEncoding.DecodeString(payload.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			gotRaw = string(decoded)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailSendCmd{
		To:        "a@example.com",
		Subject:   "Hello",
		Body:      "Body",
		Signature: true,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotRaw == "" {
		t.Fatalf("expected raw message")
	}
	if !strings.Contains(gotRaw, "Body\r\n\r\n--\r\nSig") {
		t.Fatalf("expected signature in body, got: %q", gotRaw)
	}
}

func TestGmailSendCmd_SignatureHTML(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var gotRaw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/a@b.com":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "a@b.com",
				"signature":          "<div>Sig</div>",
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			var payload struct {
				Raw string `json:"raw"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			decoded, err := base64.RawURLEncoding.DecodeString(payload.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			gotRaw = string(decoded)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailSendCmd{
		To:        "a@example.com",
		Subject:   "Hello",
		BodyHTML:  "<p>Hello</p>",
		Signature: true,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotRaw == "" {
		t.Fatalf("expected raw message")
	}
	if !strings.Contains(gotRaw, "<p>Hello</p>\r\n\r\n<div class=\"gmail_signature\"><div>Sig</div></div>") {
		t.Fatalf("expected HTML signature wrapper, got: %q", gotRaw)
	}
}

func TestGmailSendCmd_SignatureFileMissing(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	svc, err := gmail.NewService(context.Background(), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailSendCmd{
		To:            "a@example.com",
		Subject:       "Hello",
		Body:          "Body",
		SignatureFile: "/no/such/signature.txt",
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil || !strings.Contains(err.Error(), "signature file") {
		t.Fatalf("expected signature file error, got: %v", err)
	}
}

func TestSignatureHTMLToPlain(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "empty string",
			html: "",
			want: "",
		},
		{
			name: "whitespace only",
			html: "   \t\n  ",
			want: "",
		},
		{
			name: "simple div",
			html: "<div>Sig</div>",
			want: "Sig",
		},
		{
			name: "p tags with line breaks",
			html: "<p>Line1</p><p>Line2</p>",
			want: "Line1\nLine2",
		},
		{
			name: "anchor tag converted to text with URL",
			html: `<a href="https://example.com">My Site</a>`,
			want: "My Site (https://example.com)",
		},
		{
			name: "anchor tag where text equals URL",
			html: `<a href="https://example.com">https://example.com</a>`,
			want: "https://example.com",
		},
		{
			name: "anchor tag with no text",
			html: `<a href="https://example.com"></a>`,
			want: "https://example.com",
		},
		{
			name: "table with rows",
			html: "<table><tr><td>Row1</td></tr><tr><td>Row2</td></tr></table>",
			want: "Row1\nRow2",
		},
		{
			name: "br tag",
			html: "Line1<br>Line2",
			want: "Line1\nLine2",
		},
		{
			name: "br self-closing no space",
			html: "Line1<br/>Line2",
			want: "Line1\nLine2",
		},
		{
			name: "br self-closing with space",
			html: "Line1<br />Line2",
			want: "Line1\nLine2",
		},
		{
			name: "multiple consecutive newlines collapsed",
			html: "<p>A</p><p></p><p></p><p></p><p>B</p>",
			want: "A\n\nB",
		},
		{
			name: "mixed tags",
			html: `<div>John Doe</div><div><a href="https://example.com">example.com</a></div><div>555-1234</div>`,
			want: "John Doe\nexample.com (https://example.com)\n555-1234",
		},
		{
			name: "trailing whitespace trimmed",
			html: "<div>Hello   </div>",
			want: "Hello",
		},
		{
			name: "plain text passthrough",
			html: "Just plain text",
			want: "Just plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signatureHTMLToPlain(tt.html)
			if got != tt.want {
				t.Errorf("signatureHTMLToPlain(%q)\ngot:  %q\nwant: %q", tt.html, got, tt.want)
			}
		})
	}
}
