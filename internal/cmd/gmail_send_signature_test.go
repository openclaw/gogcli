package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailSendCmd_Run_WithSendAsSignature(t *testing.T) {
	raw := runGmailSendWithSignatureServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			writeSendAsList(t, w, "a@b.com", "Primary User")
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs/a@b.com":
			writeSendAsGet(t, w, "a@b.com", `<div>Kind regards<br>Primary User</div>`)
		default:
			http.NotFound(w, r)
		}
	}, &GmailSendCmd{
		To:        "recipient@example.com",
		Subject:   "Hello",
		Body:      "Body",
		BodyHTML:  "<p>Body</p>",
		Signature: true,
	})

	if !strings.Contains(raw, "Body\r\n\r\n--\r\nKind regards\r\nPrimary User") {
		t.Fatalf("plain signature missing from raw message:\n%s", raw)
	}
	if !strings.Contains(raw, `<div class="gmail_signature"><div>Kind regards<br>Primary User</div></div>`) {
		t.Fatalf("html signature missing from raw message:\n%s", raw)
	}
}

func TestGmailSendCmd_Run_SignatureFromAlias(t *testing.T) {
	raw := runGmailSendWithSignatureServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{"sendAsEmail": "a@b.com", "displayName": "Primary User", "verificationStatus": "accepted", "isPrimary": true},
					{"sendAsEmail": "alias@example.com", "displayName": "Alias", "verificationStatus": "accepted"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs/alias@example.com":
			writeSendAsGet(t, w, "alias@example.com", "<p>Alias signature</p>")
		default:
			http.NotFound(w, r)
		}
	}, &GmailSendCmd{
		To:            "recipient@example.com",
		Subject:       "Hello",
		Body:          "Body",
		From:          "alias@example.com",
		SignatureFrom: "alias@example.com",
	})

	if !strings.Contains(raw, `From: "Alias" <alias@example.com>`) {
		t.Fatalf("alias From header missing from raw message:\n%s", raw)
	}
	if !strings.Contains(raw, "Body\r\n\r\n--\r\nAlias signature") {
		t.Fatalf("alias signature missing from raw message:\n%s", raw)
	}
}

func TestGmailSendCmd_Run_WithSignatureFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signature.txt")
	if err := os.WriteFile(path, []byte("Local Sig\nhttps://example.com"), 0o600); err != nil {
		t.Fatalf("write signature file: %v", err)
	}

	raw := runGmailSendWithSignatureServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			writeSendAsList(t, w, "a@b.com", "Primary User")
		default:
			http.NotFound(w, r)
		}
	}, &GmailSendCmd{
		To:            "recipient@example.com",
		Subject:       "Hello",
		Body:          "Body",
		BodyHTML:      "<p>Body</p>",
		SignatureFile: path,
	})

	if !strings.Contains(raw, "Body\r\n\r\n--\r\nLocal Sig\r\nhttps://example.com") {
		t.Fatalf("plain file signature missing from raw message:\n%s", raw)
	}
	if !strings.Contains(raw, "Local Sig<br>\r\nhttps://example.com") {
		t.Fatalf("html file signature missing from raw message:\n%s", raw)
	}
}

func TestGmailSendCmd_Run_EmptySignatureWarnsAndSends(t *testing.T) {
	var raw string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			writeSendAsList(t, w, "a@b.com", "Primary User")
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs/a@b.com":
			writeSendAsGet(t, w, "a@b.com", "")
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
			writeGmailSendResponse(t, w, r, &raw)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	stubGmailServiceForTest(t, svc)

	var stderr strings.Builder
	ctx := newGmailSendSignatureTestContext(t, io.Discard, &stderr)
	err := (&GmailSendCmd{
		To:        "recipient@example.com",
		Subject:   "Hello",
		Body:      "Body",
		Signature: true,
	}).Run(ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(stderr.String(), "Warning: no signature configured for a@b.com") {
		t.Fatalf("expected warning, got %q", stderr.String())
	}
	if raw == "" {
		t.Fatal("expected message send to continue")
	}
}

func TestGmailSendCmd_Run_SignatureOptionConflict(t *testing.T) {
	err := (&GmailSendCmd{
		To:            "recipient@example.com",
		Subject:       "Hello",
		Body:          "Body",
		Signature:     true,
		SignatureFile: "sig.txt",
	}).Run(context.Background(), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "use only one of") {
		t.Fatalf("expected signature option conflict, got %v", err)
	}
}

func runGmailSendWithSignatureServer(t *testing.T, handler http.HandlerFunc, cmd *GmailSendCmd) string {
	t.Helper()

	var raw string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send" {
			writeGmailSendResponse(t, w, r, &raw)
			return
		}
		handler(w, r)
	})
	defer cleanup()
	stubGmailServiceForTest(t, svc)

	if err := cmd.Run(newGmailSendSignatureTestContext(t, io.Discard, io.Discard), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return raw
}

func newGmailSendSignatureTestContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: stdout, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func writeSendAsList(t *testing.T, w http.ResponseWriter, email, displayName string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sendAs": []map[string]any{
			{"sendAsEmail": email, "displayName": displayName, "verificationStatus": "accepted", "isPrimary": true},
		},
	})
}

func writeSendAsGet(t *testing.T, w http.ResponseWriter, email, signature string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sendAsEmail":        email,
		"verificationStatus": "accepted",
		"signature":          signature,
	})
}

func writeGmailSendResponse(t *testing.T, w http.ResponseWriter, r *http.Request, rawOut *string) {
	t.Helper()

	var msg gmail.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		t.Fatalf("decode sent message: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
	if err != nil {
		t.Fatalf("decode raw message: %v", err)
	}
	*rawOut = string(raw)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":       "m1",
		"threadId": "t1",
	})
}
