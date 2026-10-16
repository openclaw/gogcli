package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"

	api "github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
)

const (
	gmailExportMaxBytes      int64 = 50 << 20
	gmailExportResponseBytes int64 = 72 << 20
)

type GmailExportCmd struct {
	Raw        GmailExportRawCmd        `cmd:"" help:"Export exact raw message bytes to a file"`
	Attachment GmailExportAttachmentCmd `cmd:"" help:"Export exact attachment bytes to a file"`
}

type GmailExportRawCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID"`
	Out       string `name:"out" required:"" help:"Destination file (written atomically)"`
	MaxBytes  int64  `name:"max-bytes" default:"52428800" help:"Decoded byte limit (1..52428800)"`
}

type GmailExportAttachmentCmd struct {
	MessageID    string `arg:"" name:"messageId" help:"Message ID"`
	AttachmentID string `arg:"" name:"attachmentId" help:"Attachment ID"`
	Out          string `name:"out" required:"" help:"Destination file (written atomically)"`
	MaxBytes     int64  `name:"max-bytes" default:"52428800" help:"Decoded byte limit (1..52428800)"`
}

type gmailExportKey struct {
	Kind         string
	MessageID    string
	AttachmentID string
}

type gmailExportMetadata struct {
	MessageID    string `json:"message_id"`
	AttachmentID string `json:"attachment_id,omitempty"`
	ThreadID     string `json:"thread_id,omitempty"`
	Size         int64  `json:"size"`
}

func (c *GmailExportRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runGmailExport(ctx, flags, gmailExportKey{Kind: "raw", MessageID: c.MessageID}, c.Out, c.MaxBytes)
}

func (c *GmailExportAttachmentCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runGmailExport(ctx, flags, gmailExportKey{Kind: "attachment", MessageID: c.MessageID, AttachmentID: c.AttachmentID}, c.Out, c.MaxBytes)
}

func runGmailExport(ctx context.Context, flags *RootFlags, key gmailExportKey, dest string, maxBytes int64) error {
	if err := validateGmailExport(key, maxBytes); err != nil {
		return err
	}
	if strings.TrimSpace(dest) == "" || dest == "-" {
		return usage("--out must name a destination file")
	}
	if err := dryRunExit(ctx, flags, "gmail.export."+key.Kind, map[string]any{
		"message_id": key.MessageID, "attachment_id": key.AttachmentID, "out": dest, "max_bytes": maxBytes,
	}); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	ctx = api.WithResponseByteLimit(ctx, gmailExportResponseBytes)
	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}
	data, info, err := fetchGmailExport(ctx, svc, key, maxBytes)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeFileAtomicWithPrivacy(dest, data, func(path string) error { return makeMCPStoragePrivate(path, false) }); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	// Only metadata reaches stdout; bytes are never MIME-normalized or wrapped.
	return outfmt.WriteJSON(ctx, stdoutWriter(ctx), info)
}

func validateGmailExport(key gmailExportKey, maxBytes int64) error {
	if maxBytes <= 0 || maxBytes > gmailExportMaxBytes {
		return usage("--max-bytes must be between 1 and 52428800")
	}
	if !gmailExportIDValid(key.MessageID) || (key.Kind == "attachment" && !gmailExportIDValid(key.AttachmentID)) {
		return usage("export IDs must be 1..4096 URL-safe token characters")
	}
	if key.Kind != "raw" && key.Kind != "attachment" {
		return usage("unknown export kind")
	}
	return nil
}

func gmailExportIDValid(value string) bool {
	if len(value) == 0 || len(value) > 4096 {
		return false
	}
	for _, ch := range []byte(value) {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func fetchGmailExport(ctx context.Context, svc *gmail.Service, key gmailExportKey, maxBytes int64) ([]byte, gmailExportMetadata, error) {
	info := gmailExportMetadata{MessageID: key.MessageID, AttachmentID: key.AttachmentID}
	if err := validateGmailExport(key, maxBytes); err != nil {
		return nil, info, err
	}
	var encoded string
	var expectedSize int64 = -1
	if key.Kind == "raw" {
		msg, err := svc.Users.Messages.Get("me", key.MessageID).Format("raw").Fields(googleapi.Field("id,threadId,raw")).Context(ctx).Do()
		if err != nil {
			return nil, info, fmt.Errorf("fetch raw export: %w", err)
		}
		if msg.Id != "" && msg.Id != key.MessageID {
			return nil, info, fmt.Errorf("provider message identity mismatch")
		}
		encoded = msg.Raw
		info.ThreadID = msg.ThreadId
	} else {
		part, err := svc.Users.Messages.Attachments.Get("me", key.MessageID, key.AttachmentID).Fields(googleapi.Field("data,size")).Context(ctx).Do()
		if err != nil {
			return nil, info, fmt.Errorf("fetch attachment export: %w", err)
		}
		encoded, expectedSize = part.Data, part.Size
	}
	data, err := decodeGmailExport(encoded, maxBytes)
	if err != nil {
		return nil, info, err
	}
	if expectedSize >= 0 && expectedSize != int64(len(data)) {
		return nil, info, fmt.Errorf("provider attachment size mismatch")
	}
	info.Size = int64(len(data))
	return data, info, nil
}

func decodeGmailExport(encoded string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > gmailExportMaxBytes {
		return nil, usage("invalid export byte limit")
	}
	unpadded := strings.TrimRight(encoded, "=")
	padding := len(encoded) - len(unpadded)
	if padding > 2 || (padding > 0 && len(encoded)%4 != 0) {
		return nil, fmt.Errorf("invalid provider base64url encoding")
	}
	// Validate before allocating. encoding/base64 itself accepts CR/LF, which
	// this exact provider contract deliberately rejects.
	for _, ch := range []byte(unpadded) {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return nil, fmt.Errorf("invalid provider base64url encoding")
		}
	}
	if int64(base64.RawURLEncoding.DecodedLen(len(unpadded))) > maxBytes {
		return nil, fmt.Errorf("decoded export exceeds byte limit")
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(unpadded)
	if err != nil {
		return nil, fmt.Errorf("invalid provider base64url encoding: %w", err)
	}
	return data, nil
}
