package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type infoViaDriveOptions struct {
	ArgName      string
	ExpectedMime string
	KindLabel    string
}

const infoViaDriveDefaultKindLabel = "expected type"

func infoViaDrive(ctx context.Context, flags *RootFlags, opts infoViaDriveOptions, id string) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}
	id = normalizeGoogleID(strings.TrimSpace(id))
	if id == "" {
		return usage(fmt.Sprintf("empty %s", argName))
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	f, err := svc.Files.Get(id).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, size, createdTime, modifiedTime, webViewLink, parents").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if f == nil {
		return errors.New("file not found")
	}
	if opts.ExpectedMime != "" && f.MimeType != opts.ExpectedMime {
		label := strings.TrimSpace(opts.KindLabel)
		if label == "" {
			label = infoViaDriveDefaultKindLabel
		}
		return fmt.Errorf("file is not a %s (mimeType=%q)", label, f.MimeType)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: f})
	}

	u.Out().Printf("id\t%s", f.Id)
	u.Out().Printf("name\t%s", f.Name)
	u.Out().Printf("mime\t%s", f.MimeType)
	if f.WebViewLink != "" {
		u.Out().Printf("link\t%s", f.WebViewLink)
	}
	if f.CreatedTime != "" {
		u.Out().Printf("created\t%s", f.CreatedTime)
	}
	if f.ModifiedTime != "" {
		u.Out().Printf("modified\t%s", f.ModifiedTime)
	}
	if len(f.Parents) > 0 {
		u.Out().Printf("parents\t%s", strings.Join(f.Parents, ","))
	}
	return nil
}
