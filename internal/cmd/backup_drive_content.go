package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
)

const (
	driveMimeGoogleFolder   = "application/vnd.google-apps.folder"
	driveMimeGoogleShortcut = "application/vnd.google-apps.shortcut"
)

type driveBackupContent struct {
	FileID       string `json:"fileId"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	ExportName   string `json:"exportName"`
	ExportMime   string `json:"exportMimeType,omitempty"`
	Extension    string `json:"extension,omitempty"`
	Source       string `json:"source"`
	Size         int64  `json:"size,omitempty"`
	MD5Checksum  string `json:"md5Checksum,omitempty"`
	ModifiedTime string `json:"modifiedTime,omitempty"`
	Skipped      bool   `json:"skipped,omitempty"`
	Error        string `json:"error,omitempty"`
	DataBase64   string `json:"dataBase64,omitempty"`
}

type driveBackupContentPlan struct {
	ExportName string
	MimeType   string
	Extension  string
	Source     string
}

func fetchBackupDriveContents(ctx context.Context, svc *drive.Service, files []driveBackupFile, opts driveBackupOptions) ([]driveBackupContent, map[string]int) {
	var out []driveBackupContent
	counts := map[string]int{}
	for _, row := range files {
		if row.File == nil || strings.TrimSpace(row.File.Id) == "" {
			continue
		}
		plans := driveBackupContentPlans(row.File, opts.IncludeBinary)
		if len(plans) == 0 {
			out = append(out, driveContentErrorRow(row.File, driveBackupContentPlan{Source: "skip"}, "unsupported Drive file type for content backup"))
			counts["drive.contents.skipped"]++
			continue
		}
		for _, plan := range plans {
			if opts.MaxContentBytes > 0 && row.File.Size > opts.MaxContentBytes && plan.Source == "download" {
				out = append(out, driveContentSkippedRow(row.File, plan, fmt.Sprintf("file size %d exceeds --drive-content-max-bytes %d", row.File.Size, opts.MaxContentBytes)))
				counts["drive.contents.skipped"]++
				continue
			}
			content, err := downloadDriveBackupContent(ctx, svc, row.File, plan, opts.ContentTimeout)
			if err != nil {
				out = append(out, driveContentErrorRow(row.File, plan, err.Error()))
				counts["drive.contents.errors"]++
				continue
			}
			if opts.MaxContentBytes > 0 && int64(len(content)) > opts.MaxContentBytes {
				out = append(out, driveContentSkippedRow(row.File, plan, fmt.Sprintf("export size %d exceeds --drive-content-max-bytes %d", len(content), opts.MaxContentBytes)))
				counts["drive.contents.skipped"]++
				continue
			}
			out = append(out, driveBackupContent{
				FileID:       row.File.Id,
				Name:         row.File.Name,
				MimeType:     row.File.MimeType,
				ExportName:   plan.ExportName,
				ExportMime:   plan.MimeType,
				Extension:    plan.Extension,
				Source:       plan.Source,
				Size:         int64(len(content)),
				MD5Checksum:  row.File.Md5Checksum,
				ModifiedTime: row.File.ModifiedTime,
				DataBase64:   base64.StdEncoding.EncodeToString(content),
			})
			counts["drive.contents"]++
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FileID == out[j].FileID {
			return out[i].ExportName < out[j].ExportName
		}
		return out[i].FileID < out[j].FileID
	})
	if _, ok := counts["drive.contents"]; !ok {
		counts["drive.contents"] = 0
	}
	return out, counts
}

func driveBackupContentPlans(file *drive.File, includeBinary bool) []driveBackupContentPlan {
	if file == nil {
		return nil
	}
	switch file.MimeType {
	case driveMimeGoogleFolder, driveMimeGoogleShortcut:
		return nil
	case driveMimeGoogleDoc:
		return []driveBackupContentPlan{
			driveExportPlan(file, mimeDocx),
			driveExportPlan(file, mimeTextMarkdown),
		}
	case driveMimeGoogleSheet:
		return []driveBackupContentPlan{driveExportPlan(file, mimeXlsx)}
	case driveMimeGoogleSlides:
		return []driveBackupContentPlan{
			driveExportPlan(file, mimePptx),
			driveExportPlan(file, mimePDF),
		}
	case driveMimeGoogleDrawing:
		return []driveBackupContentPlan{
			driveExportPlan(file, mimePNG),
			driveExportPlan(file, mimePDF),
		}
	default:
		if strings.HasPrefix(file.MimeType, "application/vnd.google-apps.") {
			return []driveBackupContentPlan{driveExportPlan(file, driveExportMimeType(file.MimeType))}
		}
		if !includeBinary {
			return nil
		}
		return []driveBackupContentPlan{driveDownloadPlan(file)}
	}
}

func driveExportPlan(file *drive.File, mimeType string) driveBackupContentPlan {
	ext := driveExportExtension(mimeType)
	return driveBackupContentPlan{
		ExportName: safeDriveExportName(file, ext),
		MimeType:   mimeType,
		Extension:  ext,
		Source:     "export",
	}
}

func driveDownloadPlan(file *drive.File) driveBackupContentPlan {
	ext := strings.ToLower(filepath.Ext(file.Name))
	if ext == "" && strings.TrimSpace(file.FileExtension) != "" {
		ext = "." + strings.TrimLeft(strings.ToLower(file.FileExtension), ".")
	}
	return driveBackupContentPlan{
		ExportName: safeDriveExportName(file, ext),
		MimeType:   file.MimeType,
		Extension:  ext,
		Source:     "download",
	}
}

func safeDriveExportName(file *drive.File, ext string) string {
	name := strings.TrimSpace(file.Name)
	if name == "" {
		name = file.Id
	}
	if ext != "" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
		if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
			name += ext
		}
	}
	return sanitizeFilePart(name)
}

func downloadDriveBackupContent(ctx context.Context, svc *drive.Service, file *drive.File, plan driveBackupContentPlan, timeout time.Duration) ([]byte, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if plan.Source == "export" {
		httpResp, err := driveExportDownload(ctx, svc, file.Id, plan.MimeType)
		if err != nil {
			return nil, err
		}
		defer httpResp.Body.Close()
		if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
			body, _ := io.ReadAll(httpResp.Body)
			return nil, fmt.Errorf("export failed: %s: %s", httpResp.Status, strings.TrimSpace(string(body)))
		}
		return io.ReadAll(httpResp.Body)
	}
	httpResp, err := driveDownload(ctx, svc, file.Id)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("download failed: %s: %s", httpResp.Status, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(httpResp.Body)
}

func driveContentErrorRow(file *drive.File, plan driveBackupContentPlan, message string) driveBackupContent {
	row := driveContentSkippedRow(file, plan, message)
	row.Skipped = false
	return row
}

func driveContentSkippedRow(file *drive.File, plan driveBackupContentPlan, message string) driveBackupContent {
	return driveBackupContent{
		FileID:       file.Id,
		Name:         file.Name,
		MimeType:     file.MimeType,
		ExportName:   plan.ExportName,
		ExportMime:   plan.MimeType,
		Extension:    plan.Extension,
		Source:       plan.Source,
		MD5Checksum:  file.Md5Checksum,
		ModifiedTime: file.ModifiedTime,
		Skipped:      true,
		Error:        message,
	}
}
