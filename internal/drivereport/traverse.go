package drivereport

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	FolderMimeType   = "application/vnd.google-apps.folder"
	ShortcutMimeType = "application/vnd.google-apps.shortcut"
	RootPlacementID  = PlacementID(1)
)

var errSourceRequired = errors.New("drive report source is required")

type folderCycleError struct {
	path string
	id   string
}

func (e folderCycleError) Error() string {
	return fmt.Sprintf("drive folder cycle detected at %q (id %s)", e.path, e.id)
}

type PlacementID uint64

type ShortcutDetails struct {
	TargetID          string
	TargetMimeType    string
	TargetResourceKey string
}

type File struct {
	ID              string
	Name            string
	MimeType        string
	Size            int64
	ModifiedTime    string
	WebViewLink     string
	Owners          []string
	MD5             string
	ShortcutDetails *ShortcutDetails
}

func (f File) IsFolder() bool {
	return f.MimeType == FolderMimeType
}

type Placement struct {
	File
	Path              string
	ParentID          string
	Depth             int
	PlacementID       PlacementID
	ParentPlacementID PlacementID
}

type Source interface {
	Children(context.Context, string) ([]File, error)
}

type Options struct {
	RootID         string
	MaxDepth       int
	MaxItems       int
	IncludeFiles   bool
	IncludeFolders bool
}

type folderQueueItem struct {
	ID          string
	Path        string
	Depth       int
	PlacementID PlacementID
	Ancestry    *folderAncestry
}

type folderAncestry struct {
	ID     string
	Parent *folderAncestry
}

func (a *folderAncestry) contains(id string) bool {
	for current := a; current != nil; current = current.Parent {
		if current.ID == id {
			return true
		}
	}

	return false
}

func Traverse(ctx context.Context, source Source, opts Options) ([]Placement, bool, error) {
	if source == nil {
		return nil, false, errSourceRequired
	}

	rootID := strings.TrimSpace(opts.RootID)
	if rootID == "" {
		rootID = "root"
	}

	queue := []folderQueueItem{{
		ID:          rootID,
		PlacementID: RootPlacementID,
		Ancestry:    &folderAncestry{ID: rootID},
	}}
	out := make([]Placement, 0, 128)
	nextPlacementID := RootPlacementID

	for queueIndex := 0; queueIndex < len(queue); queueIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, false, fmt.Errorf("traverse Drive placements: %w", err)
		}

		folder := queue[queueIndex]

		children, err := source.Children(ctx, folder.ID)
		if err != nil {
			return nil, false, fmt.Errorf("list Drive folder %s: %w", folder.ID, err)
		}

		for _, child := range children {
			nextPlacementID++
			depth := folder.Depth + 1

			if child.MimeType == ShortcutMimeType {
				child.Size = 0
			}

			item := Placement{
				File:              child,
				Path:              joinPath(folder.Path, child.Name),
				ParentID:          folder.ID,
				Depth:             depth,
				PlacementID:       nextPlacementID,
				ParentPlacementID: folder.PlacementID,
			}

			if item.IsFolder() {
				if folder.Ancestry.contains(item.ID) {
					return nil, false, folderCycleError{path: item.Path, id: item.ID}
				}

				if opts.IncludeFolders {
					out = append(out, item)
				}

				if opts.MaxDepth <= 0 || depth < opts.MaxDepth {
					queue = append(queue, folderQueueItem{
						ID:          child.ID,
						Path:        item.Path,
						Depth:       depth,
						PlacementID: item.PlacementID,
						Ancestry: &folderAncestry{
							ID:     child.ID,
							Parent: folder.Ancestry,
						},
					})
				}
			} else if opts.IncludeFiles {
				out = append(out, item)
			}

			if opts.MaxItems > 0 && len(out) >= opts.MaxItems {
				return out, true, nil
			}
		}
	}

	return out, false, nil
}

func joinPath(parent string, name string) string {
	name = sanitizeName(name)
	if parent == "" {
		return name
	}

	return path.Join(parent, name)
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")

	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "_"
	}

	return name
}
