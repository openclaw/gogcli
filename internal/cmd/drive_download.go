package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DriveDownloadCmd struct {
	FileID string         `arg:"" name:"fileId" help:"File ID"`
	Output OutputPathFlag `embed:""`
	Format string         `name:"format" help:"Export format for Google Docs files: pdf|csv|xlsx|pptx|txt|png|docx|md (default: inferred)"`
	Tab    string         `name:"tab" help:"(experimental) Export a specific tab by title or ID (Google Docs only; see 'gog docs list-tabs')"`
}

func (c *DriveDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	if fileID == "" {
		return usage("empty fileId")
	}

	if tab := strings.TrimSpace(c.Tab); tab != "" {
		if f := c.Format; f != "" && f != formatAuto {
			if _, fmtErr := tabExportFormatParam(f); fmtErr != nil {
				return fmt.Errorf("--tab limits export formats (pdf|docx|txt|md|html); %q is not supported with --tab", f)
			}
		}
		return runDocsTabExport(ctx, flags, tabExportParams{
			DocID:    fileID,
			OutFlag:  c.Output.Path,
			Format:   c.Format,
			TabQuery: tab,
		})
	}

	u := ui.FromContext(ctx)
	if formatErr := validateDriveDownloadFormatFlag(c.Format); formatErr != nil {
		return formatErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	meta, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields("id, name, mimeType").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if meta.Name == "" {
		return errors.New("file has no name")
	}
	if fileFormatErr := validateDriveDownloadFormatForFile(meta, c.Format); fileFormatErr != nil {
		return fileFormatErr
	}

	destPath, err := resolveDriveDownloadDestPath(meta, c.Output.Path)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) && isStdoutPath(destPath) {
		return usage("can't combine --json with --out -")
	}

	downloadedPath, size, err := downloadDriveFile(ctx, svc, meta, destPath, c.Format)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"path": downloadedPath,
			"size": size,
		})
	}
	if isStdoutPath(downloadedPath) {
		return nil
	}

	u.Out().Printf("path\t%s", downloadedPath)
	u.Out().Printf("size\t%s", formatDriveSize(size))
	return nil
}

func downloadDriveFile(ctx context.Context, svc *drive.Service, meta *drive.File, destPath string, format string) (string, int64, error) {
	isGoogleDoc := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	if normalizedFormat == formatAuto {
		normalizedFormat = ""
	}

	if !isGoogleDoc && normalizedFormat != "" {
		return "", 0, fmt.Errorf("--format %q not supported for non-Google Workspace files (mimeType=%q); file can only be downloaded as-is", format, meta.MimeType)
	}
	if fileFormatErr := validateDriveDownloadFormatForFile(meta, format); fileFormatErr != nil {
		return "", 0, fileFormatErr
	}

	var (
		resp    *http.Response
		outPath string
		err     error
	)

	if isGoogleDoc {
		var exportMimeType string
		if normalizedFormat == "" {
			exportMimeType = driveExportMimeType(meta.MimeType)
		} else {
			var mimeErr error
			exportMimeType, mimeErr = driveExportMimeTypeForFormat(meta.MimeType, normalizedFormat)
			if mimeErr != nil {
				return "", 0, mimeErr
			}
		}
		if isStdoutPath(destPath) {
			outPath = stdoutPath
		} else {
			outPath = replaceExt(destPath, driveExportExtension(exportMimeType))
		}
		resp, err = driveExportDownload(ctx, svc, meta.Id, exportMimeType)
	} else {
		outPath = destPath
		resp, err = driveDownload(ctx, svc, meta.Id)
	}
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if isStdoutPath(outPath) {
		n, copyErr := io.Copy(os.Stdout, resp.Body)
		return stdoutPath, n, copyErr
	}

	f, outPath, err := createUserOutputFile(outPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return "", 0, err
	}
	return outPath, n, nil
}

func validateDriveDownloadFormatFlag(format string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return nil
	}
	switch format {
	case "pdf", "csv", "xlsx", "pptx", "txt", "png", "docx", "md", "html":
		return nil
	default:
		return usagef("invalid --format %q (use pdf|csv|xlsx|pptx|txt|png|docx|md|html)", format)
	}
}

func validateDriveDownloadFormatForFile(meta *drive.File, format string) error {
	if meta == nil {
		return errors.New("missing file metadata")
	}
	isGoogleDoc := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
	if isGoogleDoc {
		return nil
	}
	if strings.TrimSpace(format) == "" {
		return nil
	}
	return fmt.Errorf("--format %q not supported for non-Google Workspace files (mimeType=%q); file can only be downloaded as-is", format, meta.MimeType)
}

var driveDownload = func(ctx context.Context, svc *drive.Service, fileID string) (*http.Response, error) {
	return svc.Files.Get(fileID).SupportsAllDrives(true).Context(ctx).Download()
}

var driveExportDownload = func(ctx context.Context, svc *drive.Service, fileID string, mimeType string) (*http.Response, error) {
	return svc.Files.Export(fileID, mimeType).Context(ctx).Download()
}

func replaceExt(path string, ext string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	return base + ext
}

func driveExportMimeType(googleMimeType string) string {
	switch googleMimeType {
	case driveMimeGoogleDoc:
		return mimePDF
	case driveMimeGoogleSheet:
		return mimeCSV
	case driveMimeGoogleSlides:
		return mimePDF
	case driveMimeGoogleDrawing:
		return mimePNG
	default:
		return mimePDF
	}
}

func driveExportMimeTypeForFormat(googleMimeType string, format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || format == formatAuto {
		return driveExportMimeType(googleMimeType), nil
	}

	switch googleMimeType {
	case driveMimeGoogleDoc:
		switch format {
		case defaultExportFormat:
			return mimePDF, nil
		case "docx":
			return mimeDocx, nil
		case "txt":
			return mimeTextPlain, nil
		case "md":
			return mimeTextMarkdown, nil
		case "html":
			return mimeHTML, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Doc (use pdf|docx|txt|md|html)", format)
		}
	case driveMimeGoogleSheet:
		switch format {
		case defaultExportFormat:
			return mimePDF, nil
		case "csv":
			return mimeCSV, nil
		case "xlsx":
			return mimeXlsx, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Sheet (use pdf|csv|xlsx)", format)
		}
	case driveMimeGoogleSlides:
		switch format {
		case defaultExportFormat:
			return mimePDF, nil
		case "pptx":
			return mimePptx, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Slides (use pdf|pptx)", format)
		}
	case driveMimeGoogleDrawing:
		switch format {
		case "png":
			return mimePNG, nil
		case defaultExportFormat:
			return mimePDF, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Drawing (use png|pdf)", format)
		}
	default:
		if format == defaultExportFormat {
			return mimePDF, nil
		}
		return "", fmt.Errorf("invalid --format %q for file type %q (use pdf)", format, googleMimeType)
	}
}

func driveExportExtension(mimeType string) string {
	switch mimeType {
	case mimePDF:
		return extPDF
	case mimeCSV:
		return extCSV
	case mimeXlsx:
		return extXlsx
	case mimeDocx:
		return extDocx
	case mimePptx:
		return extPptx
	case mimePNG:
		return extPNG
	case mimeTextPlain:
		return extTXT
	case mimeTextMarkdown:
		return extMD
	case mimeHTML:
		return extHTML
	default:
		return extPDF
	}
}
