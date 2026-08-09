package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type copyViaDriveOptions struct {
	Op           string
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

	op := strings.TrimSpace(opts.Op)
	if op == "" {
		op = "drive.copy"
	}
	if err := dryRunExit(ctx, flags, op, map[string]any{
		"id":     id,
		"name":   name,
		"parent": parent,
	}); err != nil {
		return err
	}

	_, svc, err := requireDriveService(ctx, flags)
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{strFile: created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("name\t%s", created.Name)
	u.Out().Linef("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Linef("link\t%s", created.WebViewLink)
	}
	return nil
}
