package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type mailAttachment struct {
	Path     string
	Filename string
	MIMEType string
	Data     []byte
}

type rfc822Config struct {
	allowMissingTo bool
}

type mailOptions struct {
	From              string
	To                []string
	Cc                []string
	Bcc               []string
	ReplyTo           string
	Subject           string
	Body              string
	BodyHTML          string
	InReplyTo         string
	References        string
	AdditionalHeaders map[string]string
	Attachments       []mailAttachment
}

func buildRFC822(opts mailOptions, cfg *rfc822Config) ([]byte, error) {
	allowMissingTo := cfg != nil && cfg.allowMissingTo

	if strings.TrimSpace(opts.From) == "" {
		return nil, errors.New("missing From")
	}
	if len(opts.To) == 0 && !allowMissingTo {
		return nil, errors.New("missing To")
	}
	if strings.TrimSpace(opts.Subject) == "" {
		return nil, errors.New("missing Subject")
	}

	var b bytes.Buffer

	if err := validateHeaderValue(opts.From); err != nil {
		return nil, fmt.Errorf("invalid From: %w", err)
	}
	for _, a := range append(append([]string{}, opts.To...), append(opts.Cc, opts.Bcc...)...) {
		if err := validateHeaderValue(a); err != nil {
			return nil, fmt.Errorf("invalid address: %w", err)
		}
	}

	writeHeader(&b, "From", formatAddressHeader(opts.From))
	if len(opts.To) > 0 {
		writeHeader(&b, "To", formatAddressHeaders(opts.To))
	}
	if len(opts.Cc) > 0 {
		writeHeader(&b, "Cc", formatAddressHeaders(opts.Cc))
	}
	if len(opts.Bcc) > 0 {
		writeHeader(&b, "Bcc", formatAddressHeaders(opts.Bcc))
	}
	if strings.TrimSpace(opts.ReplyTo) != "" {
		if err := validateHeaderValue(opts.ReplyTo); err != nil {
			return nil, fmt.Errorf("invalid Reply-To: %w", err)
		}
		writeHeader(&b, "Reply-To", formatAddressHeader(opts.ReplyTo))
	}
	if err := validateHeaderValue(opts.Subject); err != nil {
		return nil, fmt.Errorf("invalid Subject: %w", err)
	}
	writeHeader(&b, "Subject", encodeHeaderIfNeeded(opts.Subject))
	writeHeader(&b, "Date", time.Now().Format(time.RFC1123Z))
	if !hasHeader(opts.AdditionalHeaders, "Message-ID") && !hasHeader(opts.AdditionalHeaders, "Message-Id") {
		messageID, err := randomMessageID(opts.From)
		if err != nil {
			return nil, err
		}
		writeHeader(&b, "Message-ID", messageID)
	}
	writeHeader(&b, "MIME-Version", "1.0")
	if strings.TrimSpace(opts.InReplyTo) != "" {
		if err := validateHeaderValue(opts.InReplyTo); err != nil {
			return nil, fmt.Errorf("invalid In-Reply-To: %w", err)
		}
		writeHeader(&b, "In-Reply-To", strings.TrimSpace(opts.InReplyTo))
	}
	if strings.TrimSpace(opts.References) != "" {
		if err := validateHeaderValue(opts.References); err != nil {
			return nil, fmt.Errorf("invalid References: %w", err)
		}
		writeHeader(&b, "References", strings.TrimSpace(opts.References))
	}
	for k, v := range opts.AdditionalHeaders {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			if err := validateHeaderValue(v); err != nil {
				return nil, fmt.Errorf("invalid header %s: %w", k, err)
			}
			writeHeader(&b, k, v)
		}
	}

	plainBody := normalizeCRLF(opts.Body)
	htmlBody := normalizeCRLF(opts.BodyHTML)
	hasPlain := strings.TrimSpace(plainBody) != ""
	hasHTML := strings.TrimSpace(htmlBody) != ""

	if len(opts.Attachments) == 0 {
		switch {
		case hasPlain && hasHTML:
			altBoundary, err := randomBoundary()
			if err != nil {
				return nil, err
			}
			writeHeader(&b, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", altBoundary))
			b.WriteString("\r\n")

			writeTextPart(&b, altBoundary, "text/plain; charset=\"utf-8\"", plainBody)
			writeTextPart(&b, altBoundary, "text/html; charset=\"utf-8\"", htmlBody)
			b.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))
			return b.Bytes(), nil
		case hasHTML && !hasPlain:
			writeHeader(&b, "Content-Type", "text/html; charset=\"utf-8\"")
			writeHeader(&b, "Content-Transfer-Encoding", "7bit")
			b.WriteString("\r\n")
			writeBodyWithTrailingCRLF(&b, htmlBody)
			return b.Bytes(), nil
		default:
			writeHeader(&b, "Content-Type", "text/plain; charset=\"utf-8\"")
			writeHeader(&b, "Content-Transfer-Encoding", "7bit")
			b.WriteString("\r\n")
			writeBodyWithTrailingCRLF(&b, plainBody)
			return b.Bytes(), nil
		}
	}

	mixedBoundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}

	writeHeader(&b, "Content-Type", fmt.Sprintf("multipart/mixed; boundary=%q", mixedBoundary))
	b.WriteString("\r\n")

	// Body part
	b.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
	switch {
	case hasPlain && hasHTML:
		altBoundary, err := randomBoundary()
		if err != nil {
			return nil, err
		}
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n\r\n", altBoundary))
		writeTextPart(&b, altBoundary, "text/plain; charset=\"utf-8\"", plainBody)
		writeTextPart(&b, altBoundary, "text/html; charset=\"utf-8\"", htmlBody)
		b.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))
	case hasHTML && !hasPlain:
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		writeBodyWithTrailingCRLF(&b, htmlBody)
	default:
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		writeBodyWithTrailingCRLF(&b, plainBody)
	}

	// Attachments
	for _, a := range opts.Attachments {
		if a.Filename == "" {
			a.Filename = filepath.Base(a.Path)
		}
		if a.MIMEType == "" {
			a.MIMEType = mime.TypeByExtension(strings.ToLower(filepath.Ext(a.Filename)))
			if a.MIMEType == "" {
				a.MIMEType = "application/octet-stream"
			}
		}
		if len(a.Data) == 0 {
			data, err := os.ReadFile(a.Path)
			if err != nil {
				return nil, err
			}
			a.Data = data
		}

		b.WriteString(fmt.Sprintf("\r\n--%s\r\n", mixedBoundary))
		b.WriteString(fmt.Sprintf("Content-Type: %s\r\n", a.MIMEType))
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString(fmt.Sprintf("Content-Disposition: attachment; %s\r\n\r\n", contentDispositionFilename(a.Filename)))
		b.WriteString(wrapBase64(a.Data))
		b.WriteString("\r\n")
	}

	b.WriteString(fmt.Sprintf("--%s--\r\n", mixedBoundary))
	return b.Bytes(), nil
}

func writeHeader(b *bytes.Buffer, name, value string) {
	b.WriteString(name)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\r\n")
}

func formatAddressHeader(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return trimmed
	}
	if strings.TrimSpace(addr.Name) == "" {
		return addr.Address
	}
	return addr.String()
}

func formatAddressHeaders(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	if len(parts) == 0 {
		return ""
	}

	// Prefer parsing the full comma-separated list so callers can pass either
	// repeated flags or a single comma-separated string.
	if addrs, err := mail.ParseAddressList(strings.Join(parts, ", ")); err == nil {
		formatted := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			if strings.TrimSpace(addr.Name) == "" {
				formatted = append(formatted, addr.Address)
			} else {
				formatted = append(formatted, addr.String())
			}
		}
		return strings.Join(formatted, ", ")
	}

	// Fallback: per-part parsing; keep unparseable parts unchanged.
	formatted := make([]string, 0, len(parts))
	for _, p := range parts {
		formatted = append(formatted, formatAddressHeader(p))
	}
	return strings.Join(formatted, ", ")
}

func wrapBase64(b []byte) string {
	s := base64.StdEncoding.EncodeToString(b)
	const width = 76
	var out strings.Builder
	for len(s) > width {
		out.WriteString(s[:width])
		out.WriteString("\r\n")
		s = s[width:]
	}
	if len(s) > 0 {
		out.WriteString(s)
	}
	return out.String()
}

func writeBodyWithTrailingCRLF(b *bytes.Buffer, body string) {
	b.WriteString(body)
	if !strings.HasSuffix(body, "\r\n") {
		b.WriteString("\r\n")
	}
}

func writeTextPart(b *bytes.Buffer, boundary string, contentType string, body string) {
	_, _ = fmt.Fprintf(b, "--%s\r\n", boundary)
	_, _ = fmt.Fprintf(b, "Content-Type: %s\r\n", contentType)
	b.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
	writeBodyWithTrailingCRLF(b, body)
}

func randomBoundary() (string, error) {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "gogcli_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func validateHeaderValue(v string) error {
	if strings.Contains(v, "\r") || strings.Contains(v, "\n") {
		return errors.New("header value contains newline")
	}
	return nil
}

func hasHeader(headers map[string]string, name string) bool {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

func randomMessageID(from string) (string, error) {
	domain := "gogcli.local"
	if addr, err := mail.ParseAddress(strings.TrimSpace(from)); err == nil && addr != nil {
		if at := strings.LastIndex(addr.Address, "@"); at != -1 && at+1 < len(addr.Address) {
			domain = strings.TrimSpace(addr.Address[at+1:])
		}
	} else if at := strings.LastIndex(from, "@"); at != -1 && at+1 < len(from) {
		domain = strings.TrimSpace(from[at+1:])
		domain = strings.Trim(domain, " >")
	}

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	local := base64.RawURLEncoding.EncodeToString(b[:])
	return fmt.Sprintf("<%s@%s>", local, domain), nil
}

func encodeHeaderIfNeeded(v string) string {
	if isASCII(v) {
		return v
	}
	return mime.QEncoding.Encode("utf-8", v)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func normalizeCRLF(s string) string {
	// Normalize to CRLF for RFC 5322 / MIME messages.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func contentDispositionFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return `filename="attachment"`
	}
	if isASCII(filename) {
		return fmt.Sprintf("filename=%q", filename)
	}
	// RFC 5987 / RFC 2231 style.
	return "filename*=UTF-8''" + rfc5987Encode(filename)
}

func rfc5987Encode(s string) string {
	// url.QueryEscape uses '+' for spaces; RFC 5987 wants %20.
	esc := url.QueryEscape(s)
	return strings.ReplaceAll(esc, "+", "%20")
}
