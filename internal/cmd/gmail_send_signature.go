package cmd

import (
	"context"
	"fmt"
	stdhtml "html"
	"os"
	"strings"

	nethtml "golang.org/x/net/html"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
)

const maxComposeSignatureFileBytes = 1 << 20

type composeSignature struct {
	Plain string
	HTML  string
}

func (s composeSignature) empty() bool {
	return strings.TrimSpace(s.Plain) == "" && strings.TrimSpace(s.HTML) == ""
}

func (c *GmailSendCmd) signatureRequested() bool {
	return c.Signature || strings.TrimSpace(c.SignatureFrom) != "" || strings.TrimSpace(c.SignatureFile) != ""
}

func (c *GmailSendCmd) validateSignatureOptions() error {
	if strings.TrimSpace(c.SignatureFile) != "" && (c.Signature || strings.TrimSpace(c.SignatureFrom) != "") {
		return usage("use only one of --signature/--signature-from or --signature-file")
	}
	return nil
}

func (c *GmailSendCmd) resolveComposeSignature(ctx context.Context, svc *gmail.Service, sendingEmail string) (composeSignature, string, error) {
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
		Plain: signatureHTMLToText(htmlSignature),
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
	if looksLikeHTML(value) {
		return composeSignature{
			Plain: signatureHTMLToText(value),
			HTML:  value,
		}, nil
	}
	return composeSignature{
		Plain: value,
		HTML:  escapeTextToHTML(value),
	}, nil
}

func appendComposeSignature(plainBody, htmlBody string, signature composeSignature) (string, string) {
	if strings.TrimSpace(signature.Plain) != "" && strings.TrimSpace(plainBody) != "" {
		plainBody = appendBodyBlock(plainBody, "--\n"+strings.TrimSpace(signature.Plain))
	}
	if strings.TrimSpace(signature.HTML) != "" && strings.TrimSpace(htmlBody) != "" {
		htmlBody = appendBodyBlock(htmlBody, `<div class="gmail_signature">`+strings.TrimSpace(signature.HTML)+`</div>`)
	}
	return plainBody, htmlBody
}

func appendBodyBlock(body, block string) string {
	body = strings.TrimRight(body, "\r\n")
	return body + "\n\n" + block
}

func signatureHTMLToText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	doc, err := nethtml.Parse(strings.NewReader(value))
	if err != nil {
		return stripHTMLTags(value)
	}

	var out strings.Builder
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n == nil {
			return
		}
		switch n.Type {
		case nethtml.TextNode:
			out.WriteString(n.Data)
		case nethtml.ElementNode:
			switch strings.ToLower(n.Data) {
			case "br":
				writeSignatureNewline(&out)
				return
			case "div", "p", "li":
				writeSignatureNewline(&out)
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				writeSignatureNewline(&out)
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	lines := strings.Split(stdhtml.UnescapeString(out.String()), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, "\n")
}

func writeSignatureNewline(out *strings.Builder) {
	if out.Len() == 0 {
		return
	}
	if !strings.HasSuffix(out.String(), "\n") {
		out.WriteByte('\n')
	}
}
