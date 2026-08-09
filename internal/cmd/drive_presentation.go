package cmd

import (
	"strconv"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func driveFileListColumns(plain bool) []outfmt.Column[*drive.File] {
	columns := []outfmt.Column[*drive.File]{
		{Header: "ID", Value: func(file *drive.File) string { return file.Id }},
		{Header: "NAME", Value: func(file *drive.File) string { return file.Name }},
		{Header: "TYPE", Value: func(file *drive.File) string { return driveType(file.MimeType) }},
		{Header: "SIZE", Value: func(file *drive.File) string { return formatDriveSize(file.Size) }},
		{Header: "MODIFIED", Value: func(file *drive.File) string { return formatDateTime(file.ModifiedTime) }},
		{Header: "OWNER", Value: func(file *drive.File) string { return driveOwnerEmail(file.Owners) }},
	}
	if !plain {
		columns = append(columns, outfmt.Column[*drive.File]{
			Header: "TARGET_ID",
			Value:  driveShortcutTargetID,
		})
	}
	return columns
}

func driveTreeColumns(plain bool) []outfmt.Column[driveTreeItem] {
	columns := []outfmt.Column[driveTreeItem]{
		{Header: "PATH", Value: func(item driveTreeItem) string { return sanitizeTab(item.Path) }},
		{Header: "TYPE", Value: func(item driveTreeItem) string { return driveType(item.MimeType) }},
		{Header: "SIZE", Value: func(item driveTreeItem) string { return formatDriveSize(item.Size) }},
		{Header: "MODIFIED", Value: func(item driveTreeItem) string { return formatDateTime(item.ModifiedTime) }},
		{Header: "ID", Value: func(item driveTreeItem) string { return item.ID }},
	}
	if !plain {
		columns = append(columns, outfmt.Column[driveTreeItem]{
			Header: "TARGET_ID",
			Value: func(item driveTreeItem) string {
				return driveShortcutDetailsTargetID(item.ShortcutDetails)
			},
		})
	}
	return columns
}

func driveInventoryColumns(plain bool) []outfmt.Column[driveTreeItem] {
	columns := []outfmt.Column[driveTreeItem]{
		{Header: "PATH", Value: func(item driveTreeItem) string { return sanitizeTab(item.Path) }},
		{Header: "TYPE", Value: func(item driveTreeItem) string { return driveType(item.MimeType) }},
		{Header: "SIZE", Value: func(item driveTreeItem) string { return formatDriveSize(item.Size) }},
		{Header: "MODIFIED", Value: func(item driveTreeItem) string { return formatDateTime(item.ModifiedTime) }},
		{Header: "OWNER", Value: driveTreeOwner},
		{Header: "ID", Value: func(item driveTreeItem) string { return item.ID }},
	}
	if !plain {
		columns = append(columns, outfmt.Column[driveTreeItem]{
			Header: "TARGET_ID",
			Value: func(item driveTreeItem) string {
				return driveShortcutDetailsTargetID(item.ShortcutDetails)
			},
		})
	}
	return columns
}

func driveDuColumns() []outfmt.Column[driveDuSummary] {
	return []outfmt.Column[driveDuSummary]{
		{Header: "PATH", Value: func(summary driveDuSummary) string { return sanitizeTab(summary.Path) }},
		{Header: "SIZE", Value: func(summary driveDuSummary) string { return formatDriveSize(summary.Size) }},
		{Header: "FILES", Value: func(summary driveDuSummary) string { return strconv.Itoa(summary.Files) }},
	}
}

func driveTreeOwner(item driveTreeItem) string {
	if len(item.Owners) == 0 {
		return "-"
	}
	return item.Owners[0]
}
