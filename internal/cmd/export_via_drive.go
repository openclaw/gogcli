package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

type exportViaDriveOptions struct {
	ArgName       string
	ExpectedMime  string
	KindLabel     string
	DefaultFormat string
	FormatHelp    string
}

const defaultExportFormat = "pdf"

func exportViaDrive(ctx context.Context, flags *RootFlags, opts exportViaDriveOptions, id string, outPathFlag string, format string) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return usage(fmt.Sprintf("empty %s", argName))
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

	destPath, err := resolveDriveDownloadDestPath(meta, outPathFlag)
	if err != nil {
		return err
	}

	format = strings.TrimSpace(format)
	if format == "" {
		format = strings.TrimSpace(opts.DefaultFormat)
	}
	if format == "" {
		format = defaultExportFormat
	}

	downloadedPath, size, err := downloadDriveFile(ctx, svc, meta, destPath, format)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"path": downloadedPath, "size": size})
	}
	u.Out().Printf("path\t%s", downloadedPath)
	u.Out().Printf("size\t%s", formatDriveSize(size))
	return nil
}
