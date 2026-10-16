package cmd

import (
	"reflect"
	"strings"

	"google.golang.org/api/drive/v3"
)

const (
	driveFileListFields = "nextPageToken, files(id, name, mimeType, size, modifiedTime, parents, webViewLink, owners(emailAddress), driveId, hasThumbnail, thumbnailLink, shortcutDetails(targetId,targetMimeType,targetResourceKey))"
	driveFileGetFields  = "id, name, mimeType, size, modifiedTime, createdTime, parents, webViewLink, description, starred, driveId, hasThumbnail, thumbnailLink, shortcutDetails(targetId,targetMimeType,targetResourceKey)"
)

// driveFileConditionalBools are absent from Drive responses when they do not
// apply (shared-drive vs My Drive items), so forcing them would turn "not
// applicable" into false. They keep the SDK's omitempty behavior.
var driveFileConditionalBools = map[string]bool{
	"HasAugmentedPermissions": true,
	"OwnedByMe":               true,
	"Shared":                  true,
	"WritersCanShare":         true,
}

// forceRequestedDriveFileBools keeps top-level boolean fields named in mask in
// JSON output. drive.File tags them omitempty, so a requested `trashed: false`
// would otherwise disappear.
func forceRequestedDriveFileBools(f *drive.File, mask string) {
	if f == nil {
		return
	}
	requested := driveMaskTopLevelFields(mask)
	t := reflect.TypeOf(*f)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() != reflect.Bool || driveFileConditionalBools[field.Name] {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if requested["*"] || requested[name] {
			f.ForceSendFields = append(f.ForceSendFields, field.Name)
		}
	}
}

// driveMaskTopLevelFields returns the top-level field names of a Drive field
// mask, e.g. "id, owners(emailAddress), capabilities/canEdit" -> id, owners, capabilities.
func driveMaskTopLevelFields(mask string) map[string]bool {
	fields := map[string]bool{}
	depth := 0
	start := 0
	add := func(part string) {
		part = strings.TrimSpace(part)
		if i := strings.IndexAny(part, "(/"); i >= 0 {
			part = part[:i]
		}
		if part = strings.TrimSpace(part); part != "" {
			fields[part] = true
		}
	}
	for i, r := range mask {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				add(mask[start:i])
				start = i + 1
			}
		}
	}
	add(mask[start:])
	return fields
}
