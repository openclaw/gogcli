package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/gmailcontent"
	"github.com/openclaw/gogcli/internal/ui"
)

const maxComposeSignatureFileBytes = 1 << 20

type composeSignature struct {
	Plain string
	HTML  string
}

func (s composeSignature) empty() bool {
	return strings.TrimSpace(s.Plain) == "" && strings.TrimSpace(s.HTML) == ""
}

// composeSignatureOptions holds the shared signature flags used by the send and
// reply compose commands.
type composeSignatureOptions struct {
	Signature     bool   `name:"signature" help:"Append the Gmail signature from the active send-as address"`
	SignatureFrom string `name:"signature-from" help:"Append the Gmail signature from this send-as email address"`
	SignatureFile string `name:"signature-file" help:"Append a local signature file (plain text or HTML)"`
}

func (c *composeSignatureOptions) signatureRequested() bool {
	return c.Signature || strings.TrimSpace(c.SignatureFrom) != "" || strings.TrimSpace(c.SignatureFile) != ""
}

func (c *composeSignatureOptions) validateSignatureOptions() error {
	if strings.TrimSpace(c.SignatureFile) != "" && (c.Signature || strings.TrimSpace(c.SignatureFrom) != "") {
		return usage("use only one of --signature/--signature-from or --signature-file")
	}
	return nil
}

// plainBlock is the signature as appended to a plain-text body, or "" when the
// signature has no plain text (for example an image-only signature).
func (s composeSignature) plainBlock() string {
	if strings.TrimSpace(s.Plain) == "" {
		return ""
	}
	return "--\n" + strings.TrimSpace(s.Plain)
}

// gmailSignatureOpen opens the block that holds a signature in an HTML body,
// with Gmail's class. dir="auto" gives the signature the direction of its own
// first letter, whatever the body's.
const gmailSignatureOpen = `<div dir="auto" class="gmail_signature">`

// htmlBlock is the signature as appended to an HTML body, or "" when empty.
func (s composeSignature) htmlBlock() string {
	if strings.TrimSpace(s.HTML) == "" {
		return ""
	}
	return gmailSignatureOpen + strings.TrimSpace(s.HTML) + `</div>`
}

// requestedSignature resolves the signature the signature flags ask for. It
// returns the zero value when none was requested or the resolved one is empty;
// an empty one also warns on stderr, so callers can append the result as-is.
func (c *composeSignatureOptions) requestedSignature(ctx context.Context, svc *gmail.Service, sendingEmail string) (composeSignature, error) {
	if !c.signatureRequested() {
		return composeSignature{}, nil
	}
	signature, source, err := c.resolveComposeSignature(ctx, svc, sendingEmail)
	if err != nil {
		return composeSignature{}, err
	}
	if signature.empty() {
		ui.FromContext(ctx).Err().Linef("Warning: no signature configured for %s", source)
		return composeSignature{}, nil
	}
	return signature, nil
}

func (c *composeSignatureOptions) resolveComposeSignature(ctx context.Context, svc *gmail.Service, sendingEmail string) (composeSignature, string, error) {
	if path := strings.TrimSpace(c.SignatureFile); path != "" {
		signature, err := readComposeSignatureFile(path)
		return signature, path, err
	}

	email := strings.TrimSpace(c.SignatureFrom)
	if email == "" {
		email = strings.TrimSpace(sendingEmail)
	}
	if email == "" {
		return composeSignature{}, "", usage("missing send-as email for --signature")
	}

	sendAs, err := svc.Users.Settings.SendAs.Get("me", email).Context(ctx).Do()
	if err != nil {
		return composeSignature{}, email, fmt.Errorf("fetch signature for %s: %w", email, err)
	}
	htmlSignature := strings.TrimSpace(sendAs.Signature)
	return composeSignature{
		Plain: htmlToPlainText(htmlSignature),
		HTML:  htmlSignature,
	}, email, nil
}

func readComposeSignatureFile(path string) (composeSignature, error) {
	resolved, err := config.ExpandPath(path)
	if err != nil {
		return composeSignature{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return composeSignature{}, fmt.Errorf("read signature file: %w", err)
	}
	if info.Size() > maxComposeSignatureFileBytes {
		return composeSignature{}, fmt.Errorf("signature file too large: %s", resolved)
	}
	// #nosec G304 -- --signature-file is an explicit user-provided path.
	data, err := os.ReadFile(resolved)
	if err != nil {
		return composeSignature{}, fmt.Errorf("read signature file: %w", err)
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		return composeSignature{}, nil
	}
	if gmailcontent.LooksLikeHTMLFragment(value) {
		return composeSignature{
			Plain: htmlToPlainText(value),
			HTML:  value,
		}, nil
	}
	return composeSignature{
		Plain: value,
		HTML:  directedTextToHTML(value),
	}, nil
}

func appendComposeSignature(plainBody, htmlBody string, signature composeSignature) (string, string) {
	signatureOnly := strings.TrimSpace(plainBody) == "" && strings.TrimSpace(htmlBody) == ""
	if block := signature.plainBlock(); block != "" && (strings.TrimSpace(plainBody) != "" || signatureOnly) {
		plainBody = appendBodyBlock(plainBody, block)
	}
	if block := signature.htmlBlock(); block != "" && (strings.TrimSpace(htmlBody) != "" || signatureOnly) {
		htmlBody = appendBodyBlock(htmlBody, block)
	}
	return plainBody, htmlBody
}

func appendBodyBlock(body, block string) string {
	body = strings.TrimRight(body, "\r\n")
	return body + "\n\n" + block
}
