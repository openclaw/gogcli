package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"

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
	f, err := loadInfoViaDrive(ctx, flags, opts, id)
	if err != nil {
		return err
	}
	return writeInfoViaDrive(ctx, f)
}

func loadInfoViaDrive(ctx context.Context, flags *RootFlags, opts infoViaDriveOptions, id string) (*drive.File, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}

	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}
	id = normalizeGoogleID(strings.TrimSpace(id))
	if id == "" {
		return nil, usage(fmt.Sprintf("empty %s", argName))
	}

	svc, err := driveService(ctx, account)
	if err != nil {
		return nil, err
	}

	f, err := svc.Files.Get(id).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, size, createdTime, modifiedTime, webViewLink, parents").
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, errors.New("file not found")
	}
	if opts.ExpectedMime != "" && f.MimeType != opts.ExpectedMime {
		label := strings.TrimSpace(opts.KindLabel)
		if label == "" {
			label = infoViaDriveDefaultKindLabel
		}
		return nil, fmt.Errorf("file is not a %s (mimeType=%q)", label, f.MimeType)
	}
	return f, nil
}

func writeInfoViaDrive(ctx context.Context, f *drive.File) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{strFile: f})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("id\t%s", f.Id)
	u.Out().Linef("name\t%s", f.Name)
	u.Out().Linef("mime\t%s", f.MimeType)
	if f.WebViewLink != "" {
		u.Out().Linef("link\t%s", f.WebViewLink)
	}
	if f.CreatedTime != "" {
		u.Out().Linef("created\t%s", f.CreatedTime)
	}
	if f.ModifiedTime != "" {
		u.Out().Linef("modified\t%s", f.ModifiedTime)
	}
	if len(f.Parents) > 0 {
		u.Out().Linef("parents\t%s", strings.Join(f.Parents, ","))
	}
	return nil
}
