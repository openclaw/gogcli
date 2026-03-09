package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type driveUploadOptions struct {
	localPath           string
	fileName            string
	parent              string
	replaceFileID       string
	mimeType            string
	convertMimeType     string
	isExplicitName      bool
	keepRevisionForever bool
	convert             bool
}

func (c *DriveUploadCmd) Run(ctx context.Context, flags *RootFlags) error {
	opts, err := prepareDriveUpload(c)
	if err != nil {
		return err
	}

	file, err := os.Open(opts.localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	if opts.replaceFileID == "" {
		return runDriveCreateUpload(ctx, svc, file, opts)
	}
	return runDriveReplaceUpload(ctx, svc, file, opts)
}

func prepareDriveUpload(c *DriveUploadCmd) (driveUploadOptions, error) {
	localPath := strings.TrimSpace(c.LocalPath)
	if localPath == "" {
		return driveUploadOptions{}, usage("empty localPath")
	}

	expandedPath, err := config.ExpandPath(localPath)
	if err != nil {
		return driveUploadOptions{}, err
	}

	opts := driveUploadOptions{
		localPath:           expandedPath,
		fileName:            strings.TrimSpace(c.Name),
		parent:              strings.TrimSpace(c.Parent),
		replaceFileID:       strings.TrimSpace(c.ReplaceFileID),
		mimeType:            strings.TrimSpace(c.MimeType),
		keepRevisionForever: c.KeepRevisionForever,
	}
	opts.isExplicitName = opts.fileName != ""

	if opts.replaceFileID != "" && opts.parent != "" {
		return driveUploadOptions{}, usage("--parent cannot be combined with --replace (use drive move)")
	}
	if opts.replaceFileID != "" && (c.Convert || strings.TrimSpace(c.ConvertTo) != "") {
		return driveUploadOptions{}, usage("--convert/--convert-to cannot be combined with --replace")
	}
	if opts.mimeType == "" {
		opts.mimeType = guessMimeType(opts.localPath)
	}
	if opts.replaceFileID == "" {
		opts.convertMimeType, opts.convert, err = driveUploadConvertMimeType(opts.localPath, c.Convert, c.ConvertTo)
		if err != nil {
			return driveUploadOptions{}, err
		}
		if opts.fileName == "" {
			opts.fileName = filepath.Base(opts.localPath)
		}
	}

	return opts, nil
}

func runDriveCreateUpload(ctx context.Context, svc *drive.Service, file io.Reader, opts driveUploadOptions) error {
	meta := &drive.File{Name: opts.fileName}
	if opts.parent != "" {
		meta.Parents = []string{opts.parent}
	}
	if opts.convert {
		meta.MimeType = opts.convertMimeType
		if !opts.isExplicitName {
			meta.Name = stripOfficeExt(meta.Name)
		}
	}

	call := svc.Files.Create(meta).
		SupportsAllDrives(true).
		Media(file, gapi.ContentType(opts.mimeType)).
		Fields("id, name, mimeType, size, webViewLink").
		Context(ctx)
	if opts.keepRevisionForever {
		call = call.KeepRevisionForever(true)
	}

	created, err := call.Do()
	if err != nil {
		return err
	}
	return writeDriveUploadResult(ctx, created, false, "")
}

func runDriveReplaceUpload(ctx context.Context, svc *drive.Service, file io.Reader, opts driveUploadOptions) error {
	existing, err := svc.Files.Get(opts.replaceFileID).
		SupportsAllDrives(true).
		Fields("id, mimeType").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if strings.HasPrefix(existing.MimeType, "application/vnd.google-apps.") {
		return fmt.Errorf("cannot replace content for Google Workspace files (mimeType=%s)", existing.MimeType)
	}

	meta := &drive.File{}
	if opts.fileName != "" {
		meta.Name = opts.fileName
	}

	call := svc.Files.Update(opts.replaceFileID, meta).
		SupportsAllDrives(true).
		Media(file, gapi.ContentType(opts.mimeType)).
		Fields("id, name, mimeType, size, webViewLink").
		Context(ctx)
	if opts.keepRevisionForever {
		call = call.KeepRevisionForever(true)
	}

	updated, err := call.Do()
	if err != nil {
		return err
	}
	return writeDriveUploadResult(ctx, updated, true, opts.replaceFileID)
}

func writeDriveUploadResult(ctx context.Context, file *drive.File, replaced bool, replacedFileID string) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{strFile: file}
		if replaced {
			payload["replaced"] = true
			payload["preservedFileId"] = file.Id == replacedFileID
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", file.Id)
	u.Out().Printf("name\t%s", file.Name)
	if replaced {
		u.Out().Printf("replaced\t%t", true)
	}
	if file.WebViewLink != "" {
		u.Out().Printf("link\t%s", file.WebViewLink)
	}
	return nil
}
