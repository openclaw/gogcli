package cmd

import (
	"bytes"
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

type DriveUploadCmd struct {
	LocalPath           string `arg:"" name:"localPath" help:"Path to local file"`
	Name                string `name:"name" help:"Override filename (create) or rename target (replace)"`
	Parent              string `name:"parent" help:"Destination folder ID (create only)"`
	ReplaceFileID       string `name:"replace" help:"Replace the content of an existing Drive file ID (preserves shared link/permissions)"`
	MimeType            string `name:"mime-type" help:"Override MIME type inference"`
	KeepRevisionForever bool   `name:"keep-revision-forever" help:"Keep the new head revision forever (binary files only)"`
	Convert             bool   `name:"convert" help:"Auto-convert to native Google format based on file extension (create only)"`
	ConvertTo           string `name:"convert-to" help:"Convert to a specific Google format: doc|sheet|slides (create only)"`
	KeepFrontmatter     bool   `name:"keep-frontmatter" help:"Keep YAML frontmatter (---) in Markdown when converting to a Google Doc (--convert or --convert-to doc; default: strip)"`
}

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
	size                int64
}

func guessMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case extPDF:
		return mimePDF
	case ".doc":
		return "application/msword"
	case extDocx:
		return mimeDocx
	case ".xls":
		return "application/vnd.ms-excel"
	case extXlsx:
		return mimeXlsx
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case extPptx:
		return mimePptx
	case extPNG:
		return mimePNG
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case extTXT:
		return mimeTextPlain
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".csv":
		return "text/csv"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

// googleConvertMimeType returns the Google-native MIME type for convertible
// Office/text formats. The boolean indicates whether the extension is supported.
func googleConvertMimeType(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case extDocx, ".doc":
		return driveMimeGoogleDoc, true
	case extXlsx, ".xls", extCSV:
		return driveMimeGoogleSheet, true
	case extPptx, ".ppt":
		return driveMimeGoogleSlides, true
	case extTXT, ".html", extMD:
		return driveMimeGoogleDoc, true
	default:
		return "", false
	}
}

func googleConvertTargetMimeType(target string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "doc":
		return driveMimeGoogleDoc, true
	case "sheet":
		return driveMimeGoogleSheet, true
	case "slides":
		return driveMimeGoogleSlides, true
	default:
		return "", false
	}
}

func driveUploadConvertMimeType(path string, auto bool, target string) (string, bool, error) {
	target = strings.TrimSpace(target)
	if target != "" {
		mimeType, ok := googleConvertTargetMimeType(target)
		if !ok {
			return "", false, fmt.Errorf("--convert-to: invalid value %q (use doc|sheet|slides)", target)
		}
		return mimeType, true, nil
	}
	if !auto {
		return "", false, nil
	}

	mimeType, ok := googleConvertMimeType(path)
	if !ok {
		return "", false, fmt.Errorf("--convert: unsupported file type %q (supported: docx, xlsx, pptx, doc, xls, ppt, csv, txt, html, md)", filepath.Ext(path))
	}
	return mimeType, true, nil
}

// stripOfficeExt removes common Office extensions from a filename so
// the resulting Google Doc/Sheet/Slides has a clean name.
func stripOfficeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case extDocx, ".doc", extXlsx, ".xls", extPptx, ".ppt", extMD:
		return strings.TrimSuffix(name, filepath.Ext(name))
	default:
		return name
	}
}

func (c *DriveUploadCmd) Run(ctx context.Context, flags *RootFlags) error {
	opts, err := prepareDriveUpload(c)
	if err != nil {
		return err
	}

	media, size, err := openDriveUploadMedia(opts, c.KeepFrontmatter)
	if err != nil {
		return err
	}
	defer media.Close()
	opts.size = size
	uploadReader := driveUploadReader(ctx, media, opts)

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	if opts.replaceFileID == "" {
		return runDriveCreateUpload(ctx, svc, uploadReader, opts)
	}
	return runDriveReplaceUpload(ctx, svc, uploadReader, opts)
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

func driveUploadShouldStripMarkdownFrontmatter(opts driveUploadOptions, keepFrontmatter bool) bool {
	return !keepFrontmatter && opts.convert && opts.mimeType == mimeTextMarkdown
}

func openDriveUploadMedia(opts driveUploadOptions, keepFrontmatter bool) (io.ReadCloser, int64, error) {
	file, err := os.Open(opts.localPath)
	if err != nil {
		return nil, 0, err
	}
	if !driveUploadShouldStripMarkdownFrontmatter(opts, keepFrontmatter) {
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return nil, 0, statErr
		}
		return file, info.Size(), nil
	}

	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, 0, readErr
	}
	if closeErr != nil {
		return nil, 0, closeErr
	}
	stripped := stripYAMLFrontmatter(data)
	return io.NopCloser(bytes.NewReader(stripped)), int64(len(stripped)), nil
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
