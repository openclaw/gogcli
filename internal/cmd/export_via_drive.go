package cmd

import (
	"context"
	"errors"
	"fmt"
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

func exportViaDrive(ctx context.Context, flags *RootFlags, opts exportViaDriveOptions, id string, outPathFlag string, format string, overwrite bool) error {
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
		layout, layoutErr := commandLayout(ctx, config.PathKindConfig)
		if layoutErr != nil {
			return layoutErr
		}
		defaultDownloadsDir = layout.DriveDownloadsDir()
	}
	if err := dryRunExit(ctx, flags, op, map[string]any{
		"id":                    id,
		"out":                   outPathFlag,
		"default_downloads_dir": defaultDownloadsDir,
		"format":                format,
		"overwrite":             overwrite,
		"expected_mime":         strings.TrimSpace(opts.ExpectedMime),
		"kind":                  strings.TrimSpace(opts.KindLabel),
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

	destPath, err := resolveDriveDownloadDestPath(meta, outPathFlag, defaultDownloadsDir)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) && isStdoutPath(destPath) {
		return usage("can't combine --json with --out -")
	}

	downloadedPath, size, err := downloadDriveFile(ctx, svc, meta, destPath, format, overwrite)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"path": downloadedPath, "size": size})
	}
	if isStdoutPath(downloadedPath) {
		return nil
	}
	u.Out().Linef("path\t%s", downloadedPath)
	u.Out().Linef("size\t%s", formatDriveSize(size))
	return nil
}
