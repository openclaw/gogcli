package drivereport

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	errPlacementMissing = errors.New("drive placement identity is missing")
	errPlacementExists  = errors.New("drive placement identity is duplicated")
	errParentMissing    = errors.New("drive parent placement identity is missing")
	errParentUnknown    = errors.New("drive parent placement identity is unknown")
	errAncestryCycle    = errors.New("drive placement ancestry contains a cycle")
)

type Summary struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Files int    `json:"files"`
	Depth int    `json:"depth"`
}

type folderMeta struct {
	id              string
	path            string
	depth           int
	parentPlacement PlacementID
}

func Summarize(placements []Placement, rootID string, depthLimit int) ([]Summary, error) {
	rootID = strings.TrimSpace(rootID)
	if rootID == "" {
		rootID = "root"
	}

	folders := map[PlacementID]folderMeta{
		RootPlacementID: {
			id:   rootID,
			path: ".",
		},
	}
	seen := map[PlacementID]struct{}{
		RootPlacementID: {},
	}

	for _, placement := range placements {
		if placement.PlacementID == 0 {
			return nil, fmt.Errorf("%w for item %q", errPlacementMissing, placement.ID)
		}

		if _, exists := seen[placement.PlacementID]; exists {
			return nil, fmt.Errorf("%w: %d", errPlacementExists, placement.PlacementID)
		}
		seen[placement.PlacementID] = struct{}{}

		if !placement.IsFolder() {
			continue
		}

		if placement.ParentPlacementID == 0 {
			return nil, fmt.Errorf("%w for folder %q", errParentMissing, placement.ID)
		}
		folders[placement.PlacementID] = folderMeta{
			id:              placement.ID,
			path:            placement.Path,
			depth:           placement.Depth,
			parentPlacement: placement.ParentPlacementID,
		}
	}

	for placementID := range folders {
		if _, err := placementAncestry(placementID, folders); err != nil {
			return nil, err
		}
	}

	sizes := make(map[PlacementID]*Summary)
	getSummary := func(placementID PlacementID) *Summary {
		if summary, ok := sizes[placementID]; ok {
			return summary
		}
		meta := folders[placementID]
		summary := &Summary{
			ID:    meta.id,
			Path:  meta.path,
			Depth: meta.depth,
		}
		sizes[placementID] = summary

		return summary
	}

	for _, placement := range placements {
		if placement.IsFolder() {
			continue
		}

		if placement.ParentPlacementID == 0 {
			return nil, fmt.Errorf("%w for item %q", errParentMissing, placement.ID)
		}

		ancestry, err := placementAncestry(placement.ParentPlacementID, folders)
		if err != nil {
			return nil, fmt.Errorf("item %q: %w", placement.ID, err)
		}

		size := placement.Size
		if placement.MimeType == ShortcutMimeType {
			size = 0
		}

		for _, placementID := range ancestry {
			summary := getSummary(placementID)
			summary.Size += size
			summary.Files++
		}
	}

	out := make([]Summary, 0, len(sizes))
	for _, summary := range sizes {
		if depthLimit > 0 && summary.Depth > depthLimit {
			continue
		}
		out = append(out, *summary)
	}

	return out, nil
}

func placementAncestry(start PlacementID, folders map[PlacementID]folderMeta) ([]PlacementID, error) {
	ancestry := make([]PlacementID, 0, 8)
	visited := make(map[PlacementID]struct{}, 8)

	for current := start; current != 0; {
		if _, exists := visited[current]; exists {
			return nil, fmt.Errorf("%w at %d", errAncestryCycle, current)
		}
		visited[current] = struct{}{}

		meta, ok := folders[current]
		if !ok {
			return nil, fmt.Errorf("%w: %d", errParentUnknown, current)
		}

		ancestry = append(ancestry, current)
		current = meta.parentPlacement
	}

	return ancestry, nil
}

func SortSummaries(items []Summary, sortBy string, order string) {
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	descending := strings.EqualFold(strings.TrimSpace(order), "desc")

	sort.Slice(items, func(i, j int) bool {
		comparison := compareSummary(items[i], items[j], sortBy)
		if comparison != 0 {
			if descending {
				return comparison > 0
			}

			return comparison < 0
		}

		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}

		return items[i].ID < items[j].ID
	})
}

func compareSummary(left, right Summary, sortBy string) int {
	switch sortBy {
	case "path":
		return strings.Compare(left.Path, right.Path)
	case "files":
		return compareInt(left.Files, right.Files)
	default:
		return compareInt64(left.Size, right.Size)
	}
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
