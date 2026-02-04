package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestGmailSendCmd_SignatureFileTooLarge(t *testing.T) {
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

	// Create a temp file larger than maxSignatureFileSize (1 MB).
	tmp := filepath.Join(t.TempDir(), "big_sig.html")
	data := make([]byte, maxSignatureFileSize+1)
	if writeErr := os.WriteFile(tmp, data, 0o600); writeErr != nil {
		t.Fatalf("write temp file: %v", writeErr)
	}

	cmd := &GmailSendCmd{
		To:            "a@example.com",
		Subject:       "Hello",
		Body:          "Body",
		SignatureFile: tmp,
	}
	err = cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for oversized signature file, got nil")
	}
	if !strings.Contains(err.Error(), "signature file too large") {
		t.Fatalf("expected 'signature file too large' error, got: %v", err)
	}
}

func TestIsLikelyHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "plain text", in: "Hello world", want: false},
		{name: "angle brackets no tag", in: "1 < 2 > 0", want: false},
		{name: "div tag", in: "<div>hello</div>", want: true},
		{name: "br tag", in: "line<br>break", want: true},
		{name: "p tag", in: "<p>paragraph</p>", want: true},
		{name: "anchor tag", in: `<a href="x">link</a>`, want: true},
		{name: "self-closing br", in: "text<br/>more", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLikelyHTML(tt.in)
			if got != tt.want {
				t.Errorf("isLikelyHTML(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPlainTextToHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "simple text", in: "Hello", want: "Hello"},
		{name: "newlines", in: "Line1\nLine2\nLine3", want: "Line1<br>\nLine2<br>\nLine3"},
		{name: "escapes ampersand", in: "A & B", want: "A &amp; B"},
		{name: "escapes angle brackets", in: "1 < 2 > 0", want: "1 &lt; 2 &gt; 0"},
		{name: "combined escapes and newlines", in: "A & B\nC < D", want: "A &amp; B<br>\nC &lt; D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plainTextToHTML(tt.in)
			if got != tt.want {
				t.Errorf("plainTextToHTML(%q)\ngot:  %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSignatureFileContentType_PlainText(t *testing.T) {
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

	// Write a plain-text signature file with special characters and newlines.
	tmp := filepath.Join(t.TempDir(), "sig.txt")
	if writeErr := os.WriteFile(tmp, []byte("Best & regards\nJohn Doe\n© 2024"), 0o600); writeErr != nil {
		t.Fatalf("write temp file: %v", writeErr)
	}

	cmd := &GmailSendCmd{
		To:            "a@example.com",
		Subject:       "Hello",
		Body:          "Body",
		BodyHTML:      "<p>Body</p>",
		SignatureFile: tmp,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotRaw == "" {
		t.Fatal("expected raw message")
	}

	// Plain part should contain the file verbatim (no HTML escaping).
	if !strings.Contains(gotRaw, "Best & regards") {
		t.Errorf("plain body should contain original plain text, got: %q", gotRaw)
	}

	// HTML part should have escaped entities and <br> for newlines.
	if !strings.Contains(gotRaw, "Best &amp; regards<br>") {
		t.Errorf("HTML body should contain escaped entities and <br>, got: %q", gotRaw)
	}
	if !strings.Contains(gotRaw, "© 2024") {
		t.Errorf("HTML body should contain the copyright line, got: %q", gotRaw)
	}
}

func TestSignatureFileContentType_HTML(t *testing.T) {
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

	// Write an HTML signature file.
	tmp := filepath.Join(t.TempDir(), "sig.html")
	htmlSig := `<div>John Doe</div><div><a href="https://example.com">My Site</a></div>`
	if writeErr := os.WriteFile(tmp, []byte(htmlSig), 0o600); writeErr != nil {
		t.Fatalf("write temp file: %v", writeErr)
	}

	cmd := &GmailSendCmd{
		To:            "a@example.com",
		Subject:       "Hello",
		Body:          "Body",
		BodyHTML:      "<p>Body</p>",
		SignatureFile: tmp,
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotRaw == "" {
		t.Fatal("expected raw message")
	}

	// HTML part should contain the file content verbatim (as-is).
	if !strings.Contains(gotRaw, htmlSig) {
		t.Errorf("HTML body should contain original HTML signature, got: %q", gotRaw)
	}

	// Plain part should have gone through signatureHTMLToPlain.
	// signatureHTMLToPlain converts <a> tags to "text (URL)" format.
	if !strings.Contains(gotRaw, "My Site (https://example.com)") {
		t.Errorf("plain body should contain converted anchor tag, got: %q", gotRaw)
	}
	if !strings.Contains(gotRaw, "John Doe") {
		t.Errorf("plain body should contain text from div, got: %q", gotRaw)
	}
}

func TestAppendSignaturePlain(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		signature string
		want      string
	}{
		{name: "normal case", body: "Hello", signature: "-- John", want: "Hello\n\n--\n-- John"},
		{name: "empty signature", body: "Hello", signature: "", want: "Hello"},
		{name: "whitespace-only signature", body: "Hello", signature: "   ", want: "Hello"},
		{name: "empty body with signature", body: "", signature: "Sig", want: "\n\n--\nSig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendSignaturePlain(tt.body, tt.signature)
			if got != tt.want {
				t.Errorf("appendSignaturePlain(%q, %q)\ngot:  %q\nwant: %q", tt.body, tt.signature, got, tt.want)
			}
		})
	}
}

func TestAppendSignatureHTML(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		signature string
		want      string
	}{
		{
			name:      "normal case",
			body:      "<p>Hi</p>",
			signature: "<div>Sig</div>",
			want:      "<p>Hi</p>\n\n" + `<div class="gmail_signature"><div>Sig</div></div>`,
		},
		{name: "empty signature", body: "<p>Hi</p>", signature: "", want: "<p>Hi</p>"},
		{name: "whitespace-only signature", body: "<p>Hi</p>", signature: "   ", want: "<p>Hi</p>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendSignatureHTML(tt.body, tt.signature)
			if got != tt.want {
				t.Errorf("appendSignatureHTML(%q, %q)\ngot:  %q\nwant: %q", tt.body, tt.signature, got, tt.want)
			}
		})
	}
}

func TestValidateSignatureFlags(t *testing.T) {
	tests := []struct {
		name          string
		signature     bool
		signatureName string
		signatureFile string
		wantErr       bool
	}{
		{name: "no flags set", signature: false, signatureName: "", signatureFile: "", wantErr: false},
		{name: "only --signature", signature: true, signatureName: "", signatureFile: "", wantErr: false},
		{name: "only --signature-name", signature: false, signatureName: "alias@example.com", signatureFile: "", wantErr: false},
		{name: "only --signature-file", signature: false, signatureName: "", signatureFile: "/tmp/sig.html", wantErr: false},
		{name: "--signature + --signature-name", signature: true, signatureName: "alias@example.com", signatureFile: "", wantErr: true},
		{name: "--signature + --signature-file", signature: true, signatureName: "", signatureFile: "/tmp/sig.html", wantErr: true},
		{name: "--signature-name + --signature-file", signature: false, signatureName: "alias@example.com", signatureFile: "/tmp/sig.html", wantErr: true},
		{name: "all three", signature: true, signatureName: "alias@example.com", signatureFile: "/tmp/sig.html", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSignatureFlags(tt.signature, tt.signatureName, tt.signatureFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSignatureFlags(%v, %q, %q) error = %v, wantErr = %v",
					tt.signature, tt.signatureName, tt.signatureFile, err, tt.wantErr)
			}
		})
	}
}

func TestGmailSendCmd_SignatureNameAlias(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var gotRaw string
	var gotSendAsGetPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/settings/sendAs/"):
			gotSendAsGetPath = path
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"signature":          "<div>Alias Sig</div>",
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
		To:            "recipient@example.com",
		Subject:       "Hello",
		Body:          "Body",
		SignatureName: "alias@example.com",
	}
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify the SendAs.Get call used alias@example.com, not the account email a@b.com.
	wantPath := "/users/me/settings/sendAs/alias@example.com"
	if gotSendAsGetPath != wantPath {
		t.Errorf("expected SendAs.Get path %q, got %q", wantPath, gotSendAsGetPath)
	}

	if gotRaw == "" {
		t.Fatal("expected raw message")
	}
	// The signature HTML should be converted to plain text "Alias Sig" and appended.
	if !strings.Contains(gotRaw, "Body\r\n\r\n--\r\nAlias Sig") {
		t.Errorf("expected alias signature in plain body, got: %q", gotRaw)
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
