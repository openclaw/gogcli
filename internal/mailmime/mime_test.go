package mailmime

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBuildRFC822Plain(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "\r\nMessage-ID: <") {
		t.Fatalf("missing message-id: %q", s)
	}

	if !strings.Contains(s, "Content-Type: text/plain") {
		t.Fatalf("missing content-type: %q", s)
	}

	if !strings.Contains(s, "\r\n\r\nHello\r\n") {
		t.Fatalf("missing body: %q", s)
	}
}

func TestBuildRFC822HTMLOnly(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		BodyHTML: "<p>Hello</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("missing content-type: %q", s)
	}

	if strings.Contains(s, "multipart/alternative") {
		t.Fatalf("unexpected multipart/alternative: %q", s)
	}

	if !strings.Contains(s, "<p>Hello</p>") {
		t.Fatalf("missing html body: %q", s)
	}
}

func TestBuildRFC822PlainAndHTMLAlternative(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		Body:     "Plain",
		BodyHTML: "<p>HTML</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative: %q", s)
	}

	if !strings.Contains(s, "Content-Type: text/plain") || !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("expected both text/plain and text/html parts: %q", s)
	}

	if !strings.Contains(s, "\r\n\r\nPlain\r\n") || !strings.Contains(s, "<p>HTML</p>") {
		t.Fatalf("missing bodies: %q", s)
	}
}

func TestBuildRFC822WithAttachment(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
		Attachments: []mailAttachment{
			{Filename: "x.txt", MIMEType: "text/plain", Data: []byte("abc")},
		},
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "multipart/mixed") {
		t.Fatalf("expected multipart: %q", s)
	}

	if !strings.Contains(s, "Content-Disposition: attachment; filename=\"x.txt\"") {
		t.Fatalf("missing attachment header: %q", s)
	}
}

func TestBuildRFC822AlternativeWithAttachment(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		Body:     "Plain",
		BodyHTML: "<p>HTML</p>",
		Attachments: []mailAttachment{
			{Filename: "x.txt", MIMEType: "text/plain", Data: []byte("abc")},
		},
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "multipart/mixed") {
		t.Fatalf("expected multipart/mixed: %q", s)
	}

	if !strings.Contains(s, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative: %q", s)
	}

	if !strings.Contains(s, "Content-Disposition: attachment; filename=\"x.txt\"") {
		t.Fatalf("missing attachment header: %q", s)
	}

	if !strings.Contains(s, "Content-Type: text/plain") || !strings.Contains(s, "Content-Type: text/html") {
		t.Fatalf("expected both text/plain and text/html parts: %q", s)
	}
}

func TestBuildRFC822WithInlineResource(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Hi",
		Body:     "Plain",
		BodyHTML: `<p>HTML<img src="cid:image@example.com"></p>`,
		Attachments: []mailAttachment{
			{
				Filename:  "inline.png",
				MIMEType:  "image/png",
				Data:      []byte("png"),
				DataSet:   true,
				Inline:    true,
				ContentID: "image@example.com",
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}

	s := string(raw)
	for _, want := range []string{
		"multipart/related",
		"multipart/alternative",
		"Content-ID: <image@example.com>",
		`Content-Disposition: inline; filename="inline.png"`,
		`cid:image@example.com`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
}

// Inspect generated MIME independently of contributor-reported Gmail web behavior.
func TestBuildRFC822GivesEveryAttachmentUniqueID(t *testing.T) {
	raw, buildErr := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Re: Hi",
		Body:     "Plain",
		BodyHTML: `<p>Reply</p><blockquote><img src="cid:logo@example.com"></blockquote>`,
		Attachments: []mailAttachment{
			{Filename: "logo.png", MIMEType: "image/png", Data: []byte("png"), DataSet: true, Inline: true, ContentID: "logo@example.com"},
			{Filename: "a.pdf", MIMEType: "application/pdf", Data: []byte("a"), DataSet: true},
			{Filename: "b.pdf", MIMEType: "application/pdf", Data: []byte("b"), DataSet: true},
			{Filename: "c.docx", MIMEType: "application/octet-stream", Data: []byte("c"), DataSet: true},
		},
	}, nil)
	if buildErr != nil {
		t.Fatalf("buildRFC822: %v", buildErr)
	}

	msg, readErr := mail.ReadMessage(bytes.NewReader(raw))
	if readErr != nil {
		t.Fatalf("read message: %v", readErr)
	}

	type leaf struct {
		filename, contentID, attachmentID string
	}

	var leaves []leaf

	var walk func(header textproto.MIMEHeader, body io.Reader)
	walk = func(header textproto.MIMEHeader, body io.Reader) {
		mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse Content-Type %q: %v", header.Get("Content-Type"), err)
		}

		if strings.HasPrefix(mediaType, "multipart/") {
			reader := multipart.NewReader(body, params["boundary"])

			for {
				part, partErr := reader.NextRawPart()
				if errors.Is(partErr, io.EOF) {
					return
				}

				if partErr != nil {
					t.Fatalf("next part: %v", partErr)
				}

				walk(part.Header, part)
			}
		}

		if strings.HasPrefix(mediaType, "text/") {
			return
		}

		_, dispParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
		leaves = append(leaves, leaf{
			filename:     dispParams["filename"],
			contentID:    header.Get("Content-ID"),
			attachmentID: header.Get("X-Attachment-Id"),
		})
	}
	walk(textproto.MIMEHeader(msg.Header), msg.Body)

	if len(leaves) != 4 {
		t.Fatalf("got %d attachment parts, want 4:\n%s", len(leaves), raw)
	}

	seenAttachmentIDs := map[string]bool{}
	seenContentIDs := map[string]bool{}

	for _, l := range leaves {
		if l.attachmentID == "" {
			t.Fatalf("%s: missing X-Attachment-Id:\n%s", l.filename, raw)
		}

		if seenAttachmentIDs[l.attachmentID] {
			t.Fatalf("%s: duplicate X-Attachment-Id %q", l.filename, l.attachmentID)
		}
		seenAttachmentIDs[l.attachmentID] = true

		if seenContentIDs[l.contentID] {
			t.Fatalf("%s: duplicate Content-ID %q", l.filename, l.contentID)
		}
		seenContentIDs[l.contentID] = true

		switch l.filename {
		case "logo.png":
			if l.contentID != "<logo@example.com>" {
				t.Fatalf("quoted image Content-ID = %q, want original <logo@example.com>", l.contentID)
			}
		default:
			if l.contentID != "<"+l.attachmentID+">" {
				t.Fatalf("%s: Content-ID = %q, want <%s>", l.filename, l.contentID, l.attachmentID)
			}
		}
	}
}

func TestPrepareMailAttachments(t *testing.T) {
	dir := t.TempDir()

	payloadPath := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(payloadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	emptyPath := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("write empty attachment: %v", err)
	}

	prepared, metadata, err := prepareMailAttachments([]mailAttachment{
		{Path: payloadPath},
		{Path: emptyPath},
		{Filename: "preserved.dat", Data: []byte("old"), DataSet: true},
	})
	if err != nil {
		t.Fatalf("prepareMailAttachments: %v", err)
	}

	if len(prepared) != 3 || len(metadata) != 3 {
		t.Fatalf("unexpected attachment counts: prepared=%d metadata=%d", len(prepared), len(metadata))
	}

	if string(prepared[0].Data) != "payload" || !prepared[0].DataSet {
		t.Fatalf("unexpected prepared payload: %#v", prepared[0])
	}

	if !prepared[1].DataSet || len(prepared[1].Data) != 0 {
		t.Fatalf("unexpected prepared empty attachment: %#v", prepared[1])
	}

	want := []mailAttachmentMetadata{
		{Filename: "report.txt", Size: 7},
		{Filename: "empty.bin", Size: 0},
		{Filename: "preserved.dat", Size: 3},
	}
	for i := range want {
		if metadata[i] != want[i] {
			t.Fatalf("metadata[%d] = %#v, want %#v", i, metadata[i], want[i])
		}
	}
}

func TestBuildRFC822UTF8Subject(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Grüße",
		Body:    "Hi",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(strings.ToLower(s), "subject: =?utf-8?") {
		t.Fatalf("expected encoded-word Subject: %q", s)
	}
}

func TestBuildRFC822UTF8FromDisplayName(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "Sérgio Bastos • Importrust <alias@domain.com>",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(strings.ToLower(s), "from: =?utf-8?") {
		t.Fatalf("expected encoded-word From header: %q", s)
	}

	if !strings.Contains(s, "<alias@domain.com>") {
		t.Fatalf("expected alias email in From header: %q", s)
	}

	if strings.Contains(s, "From: Sérgio Bastos • Importrust <alias@domain.com>") {
		t.Fatalf("expected From header to be RFC 2047 encoded: %q", s)
	}
}

func TestBuildRFC822PlainFromAddressStaysUnwrapped(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "From: a@b.com\r\n") {
		t.Fatalf("expected plain From address, got: %q", s)
	}

	if strings.Contains(s, "From: <a@b.com>\r\n") {
		t.Fatalf("unexpected wrapped From address: %q", s)
	}
}

func TestBuildRFC822ReplyToHeader(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		ReplyTo: "reply@example.com",
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "Reply-To: reply@example.com") {
		t.Fatalf("missing Reply-To header: %q", s)
	}
}

func TestBuildRFC822AdditionalHeadersMessageIDIsNotDuplicated(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Hi",
		Body:    "Hello",
		AdditionalHeaders: map[string]string{
			"Message-ID": "<custom@id>",
		},
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if strings.Count(s, "\r\nMessage-ID: ") != 1 {
		t.Fatalf("expected exactly one Message-ID header: %q", s)
	}

	if !strings.Contains(s, "\r\nMessage-ID: <custom@id>\r\n") {
		t.Fatalf("missing custom message-id: %q", s)
	}
}

func TestBuildRFC822ReplyToRejectsNewlines(t *testing.T) {
	_, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		ReplyTo: "a@b.com\r\nBcc: evil@evil.com",
		Subject: "Hi",
		Body:    "Hello",
	}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestEncodeHeaderIfNeeded(t *testing.T) {
	if got := encodeHeaderIfNeeded("Hello"); got != "Hello" {
		t.Fatalf("unexpected: %q", got)
	}

	got := encodeHeaderIfNeeded("Grüße")
	if got == "Grüße" || !strings.Contains(strings.ToLower(got), "=?utf-8?") {
		t.Fatalf("expected encoded-word, got: %q", got)
	}
}

func TestContentDispositionFilename(t *testing.T) {
	if got := contentDispositionFilename("a.txt"); got != "filename=\"a.txt\"" {
		t.Fatalf("unexpected: %q", got)
	}

	got := contentDispositionFilename("Grüße.txt")
	if !strings.HasPrefix(got, "filename*=UTF-8''") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestNormalizeCRLF(t *testing.T) {
	if got := normalizeCRLF(""); got != "" {
		t.Fatalf("unexpected: %q", got)
	}

	got := normalizeCRLF("a\nb\r\nc\rd")
	if got != "a\r\nb\r\nc\r\nd" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestHasHeader(t *testing.T) {
	if hasHeader(nil, "Message-ID") {
		t.Fatalf("expected false")
	}

	if hasHeader(map[string]string{}, "Message-ID") {
		t.Fatalf("expected false")
	}

	if !hasHeader(map[string]string{"message-id": "x"}, "Message-ID") {
		t.Fatalf("expected true")
	}

	if !hasHeader(map[string]string{"Message-Id": "x"}, "message-id") {
		t.Fatalf("expected true")
	}
}

func TestRandomMessageID(t *testing.T) {
	id, err := randomMessageID("A <a@b.com>", testConfig().Random)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !regexp.MustCompile(`^<[A-Za-z0-9_-]+@b\.com>$`).MatchString(id) {
		t.Fatalf("unexpected: %q", id)
	}

	id, err = randomMessageID("not-an-email", testConfig().Random)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if !regexp.MustCompile(`^<[A-Za-z0-9_-]+@gogcli\.local>$`).MatchString(id) {
		t.Fatalf("unexpected: %q", id)
	}
}

func TestFormatAddressHeaderUnparseable(t *testing.T) {
	input := "not an email at all"

	got := formatAddressHeader(input)
	if got != input {
		t.Fatalf("expected unparseable input returned unchanged, got: %q", got)
	}
}

func TestFormatAddressHeadersMixed(t *testing.T) {
	input := []string{"Alice <a@b.com>", "c@d.com", "Sérgio Bastos <s@b.com>"}
	got := formatAddressHeaders(input)

	// Should contain all three addresses comma-separated.
	parts := strings.SplitN(got, ", ", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 comma-separated parts, got %d: %q", len(parts), got)
	}

	// First part: display name "Alice" with address a@b.com.
	if !strings.Contains(parts[0], "Alice") || !strings.Contains(parts[0], "a@b.com") {
		t.Fatalf("unexpected first part: %q", parts[0])
	}

	// Second part: plain address, no angle brackets.
	if parts[1] != "c@d.com" {
		t.Fatalf("expected plain address c@d.com, got: %q", parts[1])
	}

	// Third part: non-ASCII name must be RFC 2047 encoded.
	if !strings.Contains(strings.ToLower(parts[2]), "=?utf-8?") {
		t.Fatalf("expected RFC 2047 encoded name in third part, got: %q", parts[2])
	}

	if !strings.Contains(parts[2], "s@b.com") {
		t.Fatalf("expected address s@b.com in third part, got: %q", parts[2])
	}
}

func TestFormatAddressHeadersFiltersEmpty(t *testing.T) {
	got := formatAddressHeaders([]string{"a@b.com", "", "b@c.com"})

	expected := "a@b.com, b@c.com"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestBuildRFC822PlainBodyNotHardWrapped(t *testing.T) {
	// A single long paragraph (~200 chars) must survive round-trip through
	// quoted-printable encoding without hard line breaks in the decoded output.
	longLine := "Hope you are doing well. I wanted to connect you both as I believe there could be a mutually interesting conversation around potential synergies between your respective companies and their product offerings."

	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Test",
		Body:    longLine,
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)

	// Must use quoted-printable, not 7bit, to avoid transport-level wrapping.
	if !strings.Contains(s, "Content-Transfer-Encoding: quoted-printable") {
		t.Fatalf("expected quoted-printable encoding, got: %q", s)
	}

	// Decode the QP body and verify the original line is intact.
	// Split at the header/body separator.
	parts := strings.SplitN(s, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("could not find header/body separator in: %q", s)
	}
	bodyEncoded := parts[1]

	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(bodyEncoded)))
	if err != nil {
		t.Fatalf("QP decode error: %v", err)
	}

	decodedStr := strings.TrimRight(string(decoded), "\r\n")
	if decodedStr != longLine {
		t.Fatalf("decoded body mismatch:\n  got:  %q\n  want: %q", decodedStr, longLine)
	}
}

func TestBuildRFC822PlainBodyMultiParagraph(t *testing.T) {
	body := "First long paragraph that should flow naturally without any hard wrapping at seventy-two characters or any other artificial limit.\r\n\r\nSecond paragraph also long enough to verify it stays as one logical line when decoded from quoted-printable.\r\n\r\nThird short paragraph."

	raw, err := buildRFC822(mailOptions{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "Test",
		Body:    body,
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)

	parts := strings.SplitN(s, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("could not find header/body separator")
	}

	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(parts[1])))
	if err != nil {
		t.Fatalf("QP decode error: %v", err)
	}

	decodedStr := strings.TrimRight(string(decoded), "\r\n")
	if decodedStr != body {
		t.Fatalf("decoded body mismatch:\n  got:  %q\n  want: %q", decodedStr, body)
	}
}

func TestBuildRFC822HTMLBodyStays7bit(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Test",
		BodyHTML: "<p>Hello world</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "Content-Transfer-Encoding: 7bit") {
		t.Fatalf("expected 7bit encoding for HTML body, got: %q", s)
	}
}

func TestBuildRFC822NonASCIIHTMLBodyUses8bit(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Test",
		BodyHTML: "<p>Hej åäö</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "Content-Transfer-Encoding: 8bit") {
		t.Fatalf("expected 8bit encoding for non-ASCII HTML body, got: %q", s)
	}

	if strings.Contains(s, "Content-Transfer-Encoding: 7bit") {
		t.Fatalf("non-ASCII HTML body must not be declared 7bit: %q", s)
	}
}

func TestBuildRFC822NonASCIIHTMLPartUses8bit(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:     "a@b.com",
		To:       []string{"c@d.com"},
		Subject:  "Test",
		Body:     "Plain body",
		BodyHTML: "<p>Hej åäö</p>",
	}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	s := string(raw)
	if !strings.Contains(s, "Content-Type: text/html; charset=\"utf-8\"\r\nContent-Transfer-Encoding: 8bit") {
		t.Fatalf("expected 8bit encoding for non-ASCII HTML part, got: %q", s)
	}
}

func TestFormatAddressHeadersParsesCommaSeparatedList(t *testing.T) {
	got := formatAddressHeaders([]string{"Alice <a@b.com>, Bob <b@c.com>"})

	parts := strings.SplitN(got, ", ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 2 comma-separated parts, got %q", got)
	}

	if !strings.Contains(parts[0], "a@b.com") || !strings.Contains(parts[1], "b@c.com") {
		t.Fatalf("expected both addresses in output, got %q", got)
	}
}

func TestAttachmentIDsStableAcrossMIMEBuilds(t *testing.T) {
	opts := mailOptions{
		From: "a@example.invalid", To: []string{"b@example.invalid"}, Subject: "Stable attachment IDs", Body: "message",
		Attachments: []mailAttachment{
			{Filename: "duplicate.txt", MIMEType: "text/plain", Data: []byte("same"), DataSet: true},
			{Filename: "duplicate.txt", MIMEType: "text/plain", Data: []byte("same"), DataSet: true},
			{Filename: "logo.png", MIMEType: "image/png", Data: []byte("image"), DataSet: true, Inline: true, ContentID: "original@example.invalid"},
		},
	}

	ids := regexp.MustCompile(`(?m)^X-Attachment-Id: ([^\r\n]+)`)

	var first []string

	for build := 0; build < 2; build++ {
		raw, err := buildRFC822(opts, nil)
		if err != nil {
			t.Fatal(err)
		}

		matches := ids.FindAllSubmatch(raw, -1)
		if len(matches) != 3 {
			t.Fatalf("got %d IDs", len(matches))
		}

		seen := map[string]bool{}

		for i, m := range matches {
			id := string(m[1])
			if seen[id] {
				t.Fatalf("duplicate ID %q", id)
			}

			seen[id] = true

			if build == 0 {
				first = append(first, id)
			} else if id != first[i] {
				t.Fatalf("ID changed between builds: %q != %q", id, first[i])
			}
		}

		if !bytes.Contains(raw, []byte("Content-ID: <original@example.invalid>")) {
			t.Fatal("inline Content-ID changed")
		}
	}
}
