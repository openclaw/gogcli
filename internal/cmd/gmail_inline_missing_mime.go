package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
)

func validateMissingInlineSource(ctx context.Context, svc *gmail.Service, messageID string, missing map[string]string) error {
	message, err := svc.Users.Messages.Get("me", messageID).Format(gmailFormatRaw).Fields("id,raw").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("validate quoted MIME source: %w", err)
	}
	if message == nil || message.Raw == "" || (message.Id != "" && message.Id != messageID) {
		return fmt.Errorf("quoted MIME source is unavailable")
	}
	if int64(len(message.Raw)) > ((maxGmailRawMessageBytes+2)/3)*4 {
		return fmt.Errorf("quoted MIME source exceeds the 35 MiB validation limit")
	}
	raw, err := gmailcontent.DecodeBase64URLBytes(message.Raw)
	if err != nil {
		return fmt.Errorf("quoted MIME source has invalid base64 encoding")
	}
	return validateMissingInlineMIME(raw, missing)
}

func strictInlineContentID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		value = value[1 : len(value)-1]
	}
	if value == "" {
		return "", fmt.Errorf("empty MIME Content-ID")
	}
	for _, char := range value {
		if char < '!' || char > '~' || char == '<' || char == '>' {
			return "", fmt.Errorf("invalid MIME Content-ID")
		}
	}
	return value, nil
}

func validateMissingInlineMIME(raw []byte, missing map[string]string) error {
	if int64(len(raw)) > maxGmailRawMessageBytes {
		return fmt.Errorf("quoted MIME source exceeds the 35 MiB validation limit")
	}
	const maxHeaders = 64 * 1024
	source := bytes.NewReader(raw)
	reader := bufio.NewReader(io.LimitReader(source, maxHeaders))
	headers, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		return fmt.Errorf("quoted message has malformed or oversized MIME headers")
	}
	seen := make(map[string]bool)
	count := 0
	var walk func(textproto.MIMEHeader, io.Reader, int) error
	walk = func(headers textproto.MIMEHeader, body io.Reader, depth int) error {
		count++
		if depth > maxInlineMIMEDepth || count > maxInlineMIMEParts {
			return fmt.Errorf("MIME structure exceeds inline-image validation limits")
		}
		for _, name := range []string{"Content-Type", "Content-Transfer-Encoding", "Content-ID", "Content-Disposition"} {
			if len(headers.Values(name)) > 1 {
				return fmt.Errorf("quoted message has ambiguous MIME headers")
			}
		}
		headerBytes := 0
		for name, values := range headers {
			for _, value := range values {
				headerBytes += len(name) + len(value) + 4
			}
		}
		if headerBytes > maxHeaders {
			return fmt.Errorf("quoted message has oversized MIME headers")
		}
		if values := headers.Values("Content-Disposition"); len(values) != 0 {
			if _, _, dispositionErr := mime.ParseMediaType(values[0]); dispositionErr != nil {
				return fmt.Errorf("quoted message has an invalid MIME Content-Disposition")
			}
		}
		if values := headers.Values("Content-ID"); len(values) != 0 {
			id, idErr := strictInlineContentID(values[0])
			if idErr != nil {
				return idErr
			}
			key := canonicalContentID(id)
			if seen[key] {
				return fmt.Errorf("quoted message has duplicate MIME Content-IDs")
			}
			seen[key] = true
		}
		mediaType := "text/plain"
		var params map[string]string
		if values := headers.Values("Content-Type"); len(values) != 0 {
			var parseErr error
			mediaType, params, parseErr = mime.ParseMediaType(values[0])
			if parseErr != nil {
				return fmt.Errorf("quoted message has an invalid MIME Content-Type")
			}
		}
		encoding := strings.ToLower(strings.TrimSpace(headers.Get("Content-Transfer-Encoding")))
		if len(headers.Values("Content-Transfer-Encoding")) != 0 && encoding == "" {
			return fmt.Errorf("quoted message has an empty MIME transfer encoding")
		}
		if strings.HasPrefix(mediaType, "multipart/") {
			if mediaType != "multipart/mixed" && mediaType != "multipart/related" && mediaType != "multipart/alternative" {
				return fmt.Errorf("quoted message uses an unsupported multipart type for image replacement")
			}
			if params["boundary"] == "" || (encoding != "" && encoding != "7bit" && encoding != "8bit" && encoding != "binary") {
				return fmt.Errorf("quoted message has invalid multipart MIME metadata")
			}
			parts := multipart.NewReader(body, params["boundary"])
			for {
				part, partErr := parts.NextRawPart()
				// A wrapped EOF can mean the closing boundary is missing.
				if partErr == io.EOF {
					return nil
				}
				if partErr != nil {
					return fmt.Errorf("quoted message has malformed multipart MIME")
				}
				if walkErr := walk(part.Header, part, depth+1); walkErr != nil {
					_ = part.Close()
					return walkErr
				}
				if _, readErr := io.Copy(io.Discard, part); readErr != nil {
					return fmt.Errorf("quoted message has an incomplete MIME part")
				}
				if closeErr := part.Close(); closeErr != nil {
					return fmt.Errorf("quoted message has an incomplete MIME part")
				}
			}
		}
		if strings.HasPrefix(mediaType, "message/") {
			return fmt.Errorf("quoted message contains an embedded message unsupported for image replacement")
		}
		switch encoding {
		case "", "7bit", "8bit", "binary":
		case "base64":
			body = base64.NewDecoder(base64.StdEncoding, body)
		case "quoted-printable":
			return validateInlineQuotedPrintable(body)
		default:
			return fmt.Errorf("quoted message has an unsupported MIME transfer encoding")
		}
		if _, readErr := io.Copy(io.Discard, body); readErr != nil {
			return fmt.Errorf("quoted message has an incomplete or invalid MIME body")
		}
		return nil
	}
	if err := walk(headers, io.MultiReader(reader, source), 0); err != nil {
		return err
	}
	for id := range missing {
		if seen[id] {
			return fmt.Errorf("a supposedly missing inline image exists in the raw MIME source; refusing replacement")
		}
	}
	return nil
}

func validateInlineQuotedPrintable(body io.Reader) error {
	reader := bufio.NewReader(body)
	isHex := func(char byte) bool {
		return char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F'
	}
	for {
		char, err := reader.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("quoted message has an incomplete MIME body")
		}
		if char != '=' {
			continue
		}
		first, firstErr := reader.ReadByte()
		if firstErr != nil {
			return fmt.Errorf("quoted message has invalid quoted-printable data")
		}
		if first == '\n' {
			continue
		}
		second, secondErr := reader.ReadByte()
		if secondErr != nil || !((first == '\r' && second == '\n') || (isHex(first) && isHex(second))) {
			return fmt.Errorf("quoted message has invalid quoted-printable data")
		}
	}
}
