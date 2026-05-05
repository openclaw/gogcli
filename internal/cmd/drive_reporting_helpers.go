package cmd

import (
	"sort"
	"strings"
)

type driveDuSummary struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Files int    `json:"files"`
	Depth int    `json:"depth"`
}

func summarizeDriveDu(items []driveTreeItem, rootID string, depthLimit int) []driveDuSummary {
	type folderMeta struct {
		path  string
		depth int
	}

	parentByID := map[string]string{}
	folderMetaByID := map[string]folderMeta{
		rootID: {path: ".", depth: 0},
	}
	for _, it := range items {
		if it.IsFolder() {
			parentByID[it.ID] = it.ParentID
			folderMetaByID[it.ID] = folderMeta{path: it.Path, depth: it.Depth}
		}
	}

	sizes := map[string]*driveDuSummary{}
	getSummary := func(id string) *driveDuSummary {
		if s, ok := sizes[id]; ok {
			return s
		}
		meta := folderMetaByID[id]
		s := &driveDuSummary{
			ID:    id,
			Path:  meta.path,
			Depth: meta.depth,
		}
		sizes[id] = s
		return s
	}

	for _, it := range items {
		if it.IsFolder() {
			continue
		}
		parentID := it.ParentID
		for parentID != "" {
			s := getSummary(parentID)
			s.Size += it.Size
			s.Files++
			parentID = parentByID[parentID]
		}
	}

	out := make([]driveDuSummary, 0, len(sizes))
	for _, s := range sizes {
		if depthLimit > 0 && s.Depth > depthLimit {
			continue
		}
		out = append(out, *s)
	}
	return out
}

func sortDriveDu(items []driveDuSummary, sortBy string, order string) {
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	order = strings.ToLower(strings.TrimSpace(order))
	desc := order == "desc"

	less := func(i, j int) bool { return false }
	switch sortBy {
	case "path":
		less = func(i, j int) bool { return items[i].Path < items[j].Path }
	case "files":
		less = func(i, j int) bool { return items[i].Files < items[j].Files }
	default:
		less = func(i, j int) bool { return items[i].Size < items[j].Size }
	}

	sort.Slice(items, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}

func sortDriveInventory(items []driveTreeItem, sortBy string, order string) {
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	order = strings.ToLower(strings.TrimSpace(order))
	desc := order == "desc"

	less := func(i, j int) bool { return false }
	switch sortBy {
	case "size":
		less = func(i, j int) bool { return items[i].Size < items[j].Size }
	case "modified":
		less = func(i, j int) bool { return items[i].ModifiedTime < items[j].ModifiedTime }
	default:
		less = func(i, j int) bool { return items[i].Path < items[j].Path }
	}

	sort.Slice(items, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}
