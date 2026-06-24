package gmailcontent

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/ianaindex"
	"google.golang.org/api/gmail/v1"
)

var (
	scriptPattern     = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stylePattern      = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlTagPattern    = regexp.MustCompile(`<[^>]*>`)
	whitespacePattern = regexp.MustCompile(`\s+`)
)

// StripHTMLTags removes script, style, and markup for plain-text presentation.
func StripHTMLTags(value string) string {
	value = scriptPattern.ReplaceAllString(value, "")
	value = stylePattern.ReplaceAllString(value, "")
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = whitespacePattern.ReplaceAllString(value, " ")

	return strings.TrimSpace(value)
}

// BestBodyText prefers a plain body and falls back to HTML.
func BestBodyText(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}

	plain := FindPartBody(part, "text/plain")
	if plain != "" {
		return plain
	}

	return FindPartBody(part, "text/html")
}

// BestBodyHTML prefers an HTML body and falls back to plain text.
func BestBodyHTML(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}

	html := FindPartBody(part, "text/html")
	if html != "" {
		return html
	}

	return FindPartBody(part, "text/plain")
}

// BestBodyForDisplay returns the preferred display body and whether it is HTML.
func BestBodyForDisplay(part *gmail.MessagePart) (string, bool) {
	if part == nil {
		return "", false
	}

	plain := FindPartBody(part, "text/plain")
	if plain != "" {
		return plain, LooksLikeHTML(plain)
	}

	html := FindPartBody(part, "text/html")
	if html == "" {
		return "", false
	}

	return html, true
}

// FindPartBody finds and decodes the first nested part matching mimeType.
func FindPartBody(part *gmail.MessagePart, mimeType string) string {
	if part == nil {
		return ""
	}

	if mimeTypeMatches(part.MimeType, mimeType) && part.Body != nil && part.Body.Data != "" {
		body, err := decodePartBody(part)
		if err == nil {
			return body
		}
	}

	for _, child := range part.Parts {
		if body := FindPartBody(child, mimeType); body != "" {
			return body
		}
	}

	return ""
}

func mimeTypeMatches(partType string, want string) bool {
	return normalizeMimeType(partType) == normalizeMimeType(want)
}

func normalizeMimeType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil && mediaType != "" {
		return strings.ToLower(mediaType)
	}

	if idx := strings.Index(value, ";"); idx != -1 {
		return strings.TrimSpace(value[:idx])
	}

	return value
}

// LooksLikeHTML reports whether value appears to contain an HTML document.
func LooksLikeHTML(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return false
	}

	return strings.HasPrefix(trimmed, "<!doctype") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<head") ||
		strings.HasPrefix(trimmed, "<body") ||
		strings.HasPrefix(trimmed, "<meta") ||
		strings.Contains(trimmed, "<html")
}

// LooksLikeHTMLFragment reports whether value appears to contain an HTML
// document or recognized fragment. Use it only where both plain text and HTML
// fragments are part of the input contract.
func LooksLikeHTMLFragment(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return false
	}

	return LooksLikeHTML(trimmed) || htmlFragmentPattern.MatchString(trimmed)
}

// htmlFragmentPattern matches real HTML tags and comments, not angle-bracketed plain text
// like <path>, <project>, or <a@example.com>. After the tag name, a real HTML boundary
// is required: > (close tag), / (self-closing), or whitespace (start of attributes).
// Punctuation like @, ., : does not count as a boundary.
var htmlFragmentPattern = regexp.MustCompile(
	`<!--|</?(?:p|br|div|span|table|tr|td|section|blockquote|a|img|font|style)[\s/>]`,
)

func decodePartBody(part *gmail.MessagePart) (string, error) {
	if part == nil || part.Body == nil || part.Body.Data == "" {
		return "", nil
	}

	raw, err := DecodeBase64URLBytes(part.Body.Data)
	if err != nil {
		return "", err
	}

	decoded := raw
	if encoding := strings.TrimSpace(headerValue(part, "Content-Transfer-Encoding")); encoding != "" {
		decoded = DecodeTransferEncoding(decoded, encoding)
	}

	contentType := strings.TrimSpace(headerValue(part, "Content-Type"))
	if contentType == "" {
		contentType = strings.TrimSpace(part.MimeType)
	}

	if contentType != "" {
		decoded = DecodeBodyCharset(decoded, contentType)
	}

	return string(decoded), nil
}

func headerValue(part *gmail.MessagePart, name string) string {
	if part == nil {
		return ""
	}

	for _, header := range part.Headers {
		if header != nil && strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}

	return ""
}

// DecodeTransferEncoding conservatively decodes MIME transfer encoding.
func DecodeTransferEncoding(data []byte, encoding string) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		if !looksLikeBase64(data) {
			return data
		}

		if decoded, err := decodeAnyBase64(data); err == nil {
			return decoded
		}
	case "quoted-printable":
		if !looksLikeQuotedPrintable(data) {
			return data
		}

		if decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data))); err == nil {
			return decoded
		}
	}

	return data
}

// DecodeBodyCharset decodes a MIME body according to contentType.
func DecodeBodyCharset(data []byte, contentType string) []byte {
	charsetLabel := charsetLabelFromContentType(contentType)
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(charsetLabel), "_", "-"))

	if charsetLabel == "" || normalized == "utf-8" || normalized == "utf8" {
		return data
	}
	// Gmail may normalize body.data to UTF-8 while retaining the source charset.
	// ISO-2022 payloads remain ASCII-valid and require escape-sequence detection.
	if utf8.Valid(data) && (!strings.HasPrefix(normalized, "iso-2022-") || !bytes.ContainsRune(data, '\x1b')) {
		return data
	}

	if decoded, ok := decodeWithCharsetLabel(data, charsetLabel); ok {
		return decoded
	}

	return data
}

func charsetLabelFromContentType(contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err == nil {
		if label := strings.TrimSpace(params["charset"]); label != "" {
			return label
		}
	}
	lower := strings.ToLower(contentType)
	idx := strings.Index(lower, "charset=")

	if idx == -1 {
		return ""
	}

	label := contentType[idx+len("charset="):]
	label = strings.TrimLeft(label, " \t")

	if cut := strings.IndexAny(label, "; \t"); cut != -1 {
		label = label[:cut]
	}

	return strings.Trim(label, "\"'")
}

func decodeWithCharsetLabel(data []byte, charsetLabel string) ([]byte, bool) {
	label := strings.TrimSpace(charsetLabel)
	if label == "" {
		return nil, false
	}

	if decoded, ok := decodeWithEncodingIndex(data, label); ok {
		return decoded, true
	}

	if strings.Contains(label, "_") {
		if decoded, ok := decodeWithEncodingIndex(data, strings.ReplaceAll(label, "_", "-")); ok {
			return decoded, true
		}
	}

	return nil, false
}

func decodeWithEncodingIndex(data []byte, charsetLabel string) ([]byte, bool) {
	if encoding, err := ianaindex.MIME.Encoding(charsetLabel); err == nil && encoding != nil {
		if decoded, err := encoding.NewDecoder().Bytes(data); err == nil {
			return decoded, true
		}
	}

	reader, err := charset.NewReaderLabel(charsetLabel, bytes.NewReader(data))
	if err != nil {
		return nil, false
	}

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, false
	}

	return decoded, true
}

func looksLikeBase64(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}

	for _, b := range trimmed {
		switch {
		case b >= 'A' && b <= 'Z':
		case b >= 'a' && b <= 'z':
		case b >= '0' && b <= '9':
		case b == '+', b == '/', b == '=', b == '-', b == '_':
		case b == '\n', b == '\r', b == '\t', b == ' ':
		default:
			return false
		}
	}

	return true
}

func looksLikeQuotedPrintable(data []byte) bool {
	for i := 0; i < len(data)-2; i++ {
		if data[i] != '=' {
			continue
		}

		if data[i+1] == '\r' || data[i+1] == '\n' {
			return true
		}

		if !isHexDigit(data[i+1]) || !isHexDigit(data[i+2]) {
			continue
		}

		if isHexPair(data[i+1], data[i+2], '3', 'D') {
			return true
		}

		if i+3 < len(data) && data[i+3] == '=' {
			return true
		}
	}

	return false
}

func isHexDigit(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'A' && value <= 'F') ||
		(value >= 'a' && value <= 'f')
}

func isHexPair(first, second, high, low byte) bool {
	return equalFoldHexNibble(first, high) && equalFoldHexNibble(second, low)
}

func equalFoldHexNibble(first, second byte) bool {
	if first == second {
		return true
	}

	if second >= 'A' && second <= 'F' {
		return first == second+('a'-'A')
	}

	return false
}

func decodeAnyBase64(data []byte) ([]byte, error) {
	value := string(stripBase64Whitespace(data))
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode raw URL base64: %w", err)
	}

	return decoded, nil
}

func stripBase64Whitespace(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case '\n', '\r', '\t', ' ':
			continue
		default:
			out = append(out, b)
		}
	}

	return out
}

// DecodeBase64URLBytes decodes Gmail body data with padded and standard fallbacks.
func DecodeBase64URLBytes(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode standard base64: %w", err)
	}

	return decoded, nil
}

func decodeBase64URL(value string) (string, error) {
	decoded, err := DecodeBase64URLBytes(value)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
