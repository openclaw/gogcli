package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// driveRawSensitiveFields is the set of top-level File fields redacted from
// `gog drive raw` output when the user did not name them via --fields. See
// docs/raw-audit.md for the rationale per field.
var driveRawSensitiveFields = []string{
	"thumbnailLink",
	"webContentLink",
	"exportLinks",
	"resourceKey",
	"appProperties",
	"properties",
}

// DriveRawCmd dumps the full Files.Get response as JSON. Uses fields=* by
// default to expose the entire File resource. When --fields is absent the
// command redacts a small set of capability/token-shaped fields (see
// driveRawSensitiveFields); when --fields is explicitly set the response is
// returned verbatim, honoring exactly what the user asked for. This means
// passing `--fields "id,name,thumbnailLink"` returns thumbnailLink as
// requested.
//
// REST reference: https://developers.google.com/drive/api/reference/rest/v3/files/get
// Go type: https://pkg.go.dev/google.golang.org/api/drive/v3#File
type DriveRawCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Fields string `name:"fields" help:"Drive API field mask (default: * with sensitive fields redacted client-side). Set explicitly to disable redaction."`
	Pretty bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *DriveRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := driveHTTPClient(ctx, account)
	if err != nil {
		return err
	}

	userSetFields := strings.TrimSpace(c.Fields) != ""
	mask := "*"
	if userSetFields {
		mask = c.Fields
	}

	m, err := readRawObject(ctx, client, "https://www.googleapis.com/drive/v3/files/"+url.PathEscape(fileID), url.Values{
		"supportsAllDrives": {"true"}, "fields": {mask},
	}, "file")
	if err != nil {
		return err
	}

	if !userSetFields {
		for _, key := range driveRawSensitiveFields {
			delete(m, key)
		}
		if err := redactDriveRawThumbnail(m); err != nil {
			return err
		}
	}

	return writeRawJSON(ctx, m, c.Pretty)
}

func redactDriveRawThumbnail(fields map[string]json.RawMessage) error {
	if len(fields["contentHints"]) == 0 {
		return nil
	}
	var hints map[string]json.RawMessage
	if err := json.Unmarshal(fields["contentHints"], &hints); err != nil {
		return fmt.Errorf("decode drive content hints: %w", err)
	}
	if len(hints["thumbnail"]) == 0 {
		return nil
	}
	var thumbnail map[string]json.RawMessage
	if err := json.Unmarshal(hints["thumbnail"], &thumbnail); err != nil {
		return fmt.Errorf("decode drive thumbnail: %w", err)
	}
	if _, ok := thumbnail["image"]; !ok {
		return nil
	}
	delete(thumbnail, "image")
	var err error
	hints["thumbnail"], err = json.Marshal(thumbnail)
	if err != nil {
		return err
	}
	fields["contentHints"], err = json.Marshal(hints)
	return err
}
