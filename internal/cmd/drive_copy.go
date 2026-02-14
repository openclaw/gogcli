package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type copyViaDriveOptions struct {
	ArgName      string
	ExpectedMime string
	KindLabel    string
}

func copyViaDrive(ctx context.Context, flags *RootFlags, opts copyViaDriveOptions, id string, name string, parent string) error {
	u := ui.FromContext(ctx)
	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}
	id = normalizeGoogleID(strings.TrimSpace(id))
	if id == "" {
		return usage(fmt.Sprintf("empty %s", argName))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return usage("empty name")
	}
	parent = normalizeGoogleID(strings.TrimSpace(parent))

	if err := dryRunExit(ctx, flags, "drive.copy", map[string]any{
		"id":     id,
		"name":   name,
		"parent": parent,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	meta, err := svc.Files.Get(id).
		SupportsAllDrives(true).
		Fields("id, name, mimeType").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if meta == nil {
		return errors.New("file not found")
	}
	if opts.ExpectedMime != "" && meta.MimeType != opts.ExpectedMime {
		label := strings.TrimSpace(opts.KindLabel)
		if label == "" {
			label = "expected type"
		}
		return fmt.Errorf("file is not a %s (mimeType=%q)", label, meta.MimeType)
	}

	req := &drive.File{Name: name}
	if parent != "" {
		req.Parents = []string{parent}
	}

	created, err := svc.Files.Copy(id, req).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if created == nil {
		return errors.New("copy failed")
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
}
