package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type exportViaDriveOptions struct {
	Op            string
	ArgName       string
	ExpectedMime  string
	KindLabel     string
	DefaultFormat string
	FormatHelp    string
}

const defaultExportFormat = "pdf"

func exportViaDrive(ctx context.Context, flags *RootFlags, opts exportViaDriveOptions, id string, outPathFlag string, format string) error {
	u := ui.FromContext(ctx)

	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}
	id = normalizeGoogleID(strings.TrimSpace(id))
	if id == "" {
		return usage(fmt.Sprintf("empty %s", argName))
	}

	// Avoid touching auth/keyring and avoid writing files in dry-run mode.
	outPathFlag = strings.TrimSpace(outPathFlag)
	if outPathFlag != "" {
		expanded, err := config.ExpandPath(outPathFlag)
		if err != nil {
			return err
		}
		outPathFlag = expanded
	}

	format = strings.TrimSpace(format)
	if format == "" {
		format = strings.TrimSpace(opts.DefaultFormat)
	}
	if format == "" {
		format = defaultExportFormat
	}

	op := strings.TrimSpace(opts.Op)
	if op == "" {
		op = "drive.export"
	}
	var defaultDownloadsDir string
	if outPathFlag == "" {
		if dir, err := config.DriveDownloadsDir(); err == nil {
			defaultDownloadsDir = dir
		}
	}
	if err := dryRunExit(ctx, flags, op, map[string]any{
		"id":                    id,
		"out":                   outPathFlag,
		"default_downloads_dir": defaultDownloadsDir,
		"format":                format,
		"expected_mime":         strings.TrimSpace(opts.ExpectedMime),
		"kind":                  strings.TrimSpace(opts.KindLabel),
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

	destPath, err := resolveDriveDownloadDestPath(meta, outPathFlag)
	if err != nil {
		return err
	}

	downloadedPath, size, err := downloadDriveFile(ctx, svc, meta, destPath, format)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"path": downloadedPath, "size": size})
	}
	u.Out().Printf("path\t%s", downloadedPath)
	u.Out().Printf("size\t%s", formatDriveSize(size))
	return nil
}
