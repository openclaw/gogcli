package mailmime

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var (
	errMissingFrom              = errors.New("missing From")
	errMissingTo                = errors.New("missing To")
	errMissingSubject           = errors.New("missing Subject")
	errDateLocationRequired     = errors.New("date location is required")
	errClockRequired            = errors.New("clock is required")
	errRandomSourceRequired     = errors.New("random source is required")
	errAttachmentReaderRequired = errors.New("attachment file reader is required")
	errHeaderValueNewline       = errors.New("header value contains newline")
)

// Attachment describes an RFC822 attachment from bytes or a file path.
type Attachment struct {
	Path     string
	Filename string
	MIMEType string
	Data     []byte
	DataSet  bool
}

// AttachmentMetadata describes a prepared attachment.
type AttachmentMetadata struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// Config supplies RFC822 policy and runtime dependencies.
type Config struct {
	AllowMissingTo bool
	DateLocation   *time.Location
	Now            func() time.Time
	Random         io.Reader
	ReadFile       func(string) ([]byte, error)
}

// Options contains the fields used to construct an RFC822 message.
type Options struct {
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
	Attachments       []Attachment
}

// BuildRFC822 constructs an RFC822 message with explicit runtime dependencies.
func BuildRFC822(opts Options, cfg Config) ([]byte, error) {
	if strings.TrimSpace(opts.From) == "" {
		return nil, errMissingFrom
	}

	if len(opts.To) == 0 && !cfg.AllowMissingTo {
		return nil, errMissingTo
	}

	if strings.TrimSpace(opts.Subject) == "" {
		return nil, errMissingSubject
	}

	if cfg.DateLocation == nil {
		return nil, errDateLocationRequired
	}

	if cfg.Now == nil {
		return nil, errClockRequired
	}

	if cfg.Random == nil {
		return nil, errRandomSourceRequired
	}

	var b bytes.Buffer

	if err := ValidateHeaderValue(opts.From); err != nil {
		return nil, fmt.Errorf("invalid From: %w", err)
	}

	for _, a := range append(append([]string{}, opts.To...), append(opts.Cc, opts.Bcc...)...) {
		if err := ValidateHeaderValue(a); err != nil {
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
		if err := ValidateHeaderValue(opts.ReplyTo); err != nil {
			return nil, fmt.Errorf("invalid Reply-To: %w", err)
		}

		writeHeader(&b, "Reply-To", formatAddressHeader(opts.ReplyTo))
	}

	if err := ValidateHeaderValue(opts.Subject); err != nil {
		return nil, fmt.Errorf("invalid Subject: %w", err)
	}

	writeHeader(&b, "Subject", encodeHeaderIfNeeded(opts.Subject))
	writeHeader(&b, "Date", cfg.Now().In(cfg.DateLocation).Format(time.RFC1123Z))

	if !hasHeader(opts.AdditionalHeaders, "Message-ID") && !hasHeader(opts.AdditionalHeaders, "Message-Id") {
		messageID, err := randomMessageID(opts.From, cfg.Random)
		if err != nil {
			return nil, err
		}

		writeHeader(&b, "Message-ID", messageID)
	}

	writeHeader(&b, "MIME-Version", "1.0")

	if strings.TrimSpace(opts.InReplyTo) != "" {
		if err := ValidateHeaderValue(opts.InReplyTo); err != nil {
			return nil, fmt.Errorf("invalid In-Reply-To: %w", err)
		}

		writeHeader(&b, "In-Reply-To", strings.TrimSpace(opts.InReplyTo))
	}

	if strings.TrimSpace(opts.References) != "" {
		if err := ValidateHeaderValue(opts.References); err != nil {
			return nil, fmt.Errorf("invalid References: %w", err)
		}

		writeHeader(&b, "References", strings.TrimSpace(opts.References))
	}

	for k, v := range opts.AdditionalHeaders {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			if err := ValidateHeaderValue(v); err != nil {
				return nil, fmt.Errorf("invalid header %s: %w", k, err)
			}

			writeHeader(&b, k, v)
		}
	}

	plainBody := normalizeCRLF(opts.Body)
	htmlBody := normalizeCRLF(opts.BodyHTML)
	hasPlain := strings.TrimSpace(plainBody) != ""
	hasHTML := strings.TrimSpace(htmlBody) != ""

	attachments, _, prepareErr := PrepareAttachments(opts.Attachments, cfg.ReadFile)
	if prepareErr != nil {
		return nil, prepareErr
	}

	if len(opts.Attachments) == 0 {
		switch {
		case hasPlain && hasHTML:
			altBoundary, err := randomBoundary(cfg.Random)
			if err != nil {
				return nil, err
			}

			writeHeader(&b, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", altBoundary))
			b.WriteString("\r\n")

			writeTextPart(&b, altBoundary, "text/plain; charset=\"utf-8\"", plainBody)
			writeTextPart(&b, altBoundary, "text/html; charset=\"utf-8\"", htmlBody)
			fmt.Fprintf(&b, "--%s--\r\n", altBoundary)

			return b.Bytes(), nil
		case hasHTML && !hasPlain:
			writeHeader(&b, "Content-Type", "text/html; charset=\"utf-8\"")
			writeHeader(&b, "Content-Transfer-Encoding", textTransferEncoding(htmlBody))
			b.WriteString("\r\n")
			writeBodyWithTrailingCRLF(&b, htmlBody)

			return b.Bytes(), nil
		default:
			writeHeader(&b, "Content-Type", "text/plain; charset=\"utf-8\"")
			writeHeader(&b, "Content-Transfer-Encoding", "quoted-printable")
			b.WriteString("\r\n")
			writeQuotedPrintableBody(&b, plainBody)

			return b.Bytes(), nil
		}
	}

	mixedBoundary, err := randomBoundary(cfg.Random)
	if err != nil {
		return nil, err
	}

	writeHeader(&b, "Content-Type", fmt.Sprintf("multipart/mixed; boundary=%q", mixedBoundary))
	b.WriteString("\r\n")

	// Body part
	fmt.Fprintf(&b, "--%s\r\n", mixedBoundary)

	switch {
	case hasPlain && hasHTML:
		altBoundary, err := randomBoundary(cfg.Random)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", altBoundary)
		writeTextPart(&b, altBoundary, "text/plain; charset=\"utf-8\"", plainBody)
		writeTextPart(&b, altBoundary, "text/html; charset=\"utf-8\"", htmlBody)
		fmt.Fprintf(&b, "--%s--\r\n", altBoundary)
	case hasHTML && !hasPlain:
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		fmt.Fprintf(&b, "Content-Transfer-Encoding: %s\r\n\r\n", textTransferEncoding(htmlBody))
		writeBodyWithTrailingCRLF(&b, htmlBody)
	default:
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		writeQuotedPrintableBody(&b, plainBody)
	}

	// Attachments
	for _, a := range attachments {
		fmt.Fprintf(&b, "\r\n--%s\r\n", mixedBoundary)
		fmt.Fprintf(&b, "Content-Type: %s\r\n", a.MIMEType)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; %s\r\n\r\n", contentDispositionFilename(a.Filename))
		b.WriteString(wrapBase64(a.Data))
		b.WriteString("\r\n")
	}

	fmt.Fprintf(&b, "--%s--\r\n", mixedBoundary)

	return b.Bytes(), nil
}

// PrepareAttachments resolves filenames, MIME types, bytes, and metadata.
func PrepareAttachments(attachments []Attachment, readFile func(string) ([]byte, error)) ([]Attachment, []AttachmentMetadata, error) {
	if len(attachments) == 0 {
		return nil, nil, nil
	}

	prepared := make([]Attachment, 0, len(attachments))

	metadata := make([]AttachmentMetadata, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Filename == "" {
			attachment.Filename = filepath.Base(attachment.Path)
		}

		if attachment.MIMEType == "" {
			attachment.MIMEType = mime.TypeByExtension(strings.ToLower(filepath.Ext(attachment.Filename)))
			if attachment.MIMEType == "" {
				attachment.MIMEType = "application/octet-stream"
			}
		}

		if len(attachment.Data) == 0 && !attachment.DataSet {
			if readFile == nil {
				return nil, nil, errAttachmentReaderRequired
			}

			data, err := readFile(attachment.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("read attachment %q: %w", attachment.Path, err)
			}
			attachment.Data = data
			attachment.DataSet = true
		}

		prepared = append(prepared, attachment)
		metadata = append(metadata, AttachmentMetadata{
			Filename: attachment.Filename,
			Size:     int64(len(attachment.Data)),
		})
	}

	return prepared, metadata, nil
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

func writeQuotedPrintableBody(b *bytes.Buffer, body string) {
	qpw := quotedprintable.NewWriter(b)
	_, _ = qpw.Write([]byte(body))
	_ = qpw.Close()
	// Ensure trailing CRLF after the encoded body.
	if !bytes.HasSuffix(b.Bytes(), []byte("\r\n")) {
		b.WriteString("\r\n")
	}
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
	if strings.HasPrefix(contentType, "text/plain") {
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		writeQuotedPrintableBody(b, body)
	} else {
		_, _ = fmt.Fprintf(b, "Content-Transfer-Encoding: %s\r\n\r\n", textTransferEncoding(body))
		writeBodyWithTrailingCRLF(b, body)
	}
}

func textTransferEncoding(body string) string {
	if isASCII(body) {
		return "7bit"
	}

	return "8bit"
}

func randomBoundary(random io.Reader) (string, error) {
	var b [18]byte
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", fmt.Errorf("generate MIME boundary: %w", err)
	}

	return "gogcli_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// ValidateHeaderValue rejects CR/LF header injection.
func ValidateHeaderValue(v string) error {
	if strings.Contains(v, "\r") || strings.Contains(v, "\n") {
		return errHeaderValueNewline
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

func randomMessageID(from string, random io.Reader) (string, error) {
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
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", fmt.Errorf("generate message ID: %w", err)
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
