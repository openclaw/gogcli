package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gapi "google.golang.org/api/googleapi"
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
	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	userSetFields := strings.TrimSpace(c.Fields) != ""
	mask := "*"
	if userSetFields {
		mask = c.Fields
	}

	f, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields(gapi.Field(mask)).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	f, err = requireRawResponse(f, "file not found")
	if err != nil {
		return err
	}

	raw, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal drive file: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("unmarshal drive file: %w", err)
	}

	if !userSetFields {
		for _, key := range driveRawSensitiveFields {
			delete(m, key)
		}
		if hints, ok := m["contentHints"].(map[string]any); ok {
			if thumb, ok := hints["thumbnail"].(map[string]any); ok {
				delete(thumb, "image")
			}
		}
	}

	return writeRawJSON(ctx, m, c.Pretty)
}
