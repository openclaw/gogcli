package drivereport

import (
	"errors"
	"reflect"
	"testing"
)

func TestSummarizeRollsFilePlacementsIntoFolderAncestry(t *testing.T) {
	t.Parallel()

	placements := []Placement{
		{File: File{ID: "f1", MimeType: FolderMimeType}, Path: "a", Depth: 1, PlacementID: 2, ParentPlacementID: RootPlacementID},
		{File: File{ID: "f2", MimeType: FolderMimeType}, Path: "a/b", Depth: 2, PlacementID: 3, ParentPlacementID: 2},
		{File: File{ID: "file1", Size: 10}, Path: "a/file.txt", Depth: 2, PlacementID: 4, ParentPlacementID: 2},
		{File: File{ID: "file2", Size: 5}, Path: "a/b/file2.txt", Depth: 3, PlacementID: 5, ParentPlacementID: 3},
	}

	summaries, err := Summarize(placements, "root", 1)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	byID := summariesByID(summaries)
	if got := byID["root"]; got.Path != "." || got.Size != 15 || got.Files != 2 || got.Depth != 0 {
		t.Fatalf("root summary = %#v", got)
	}

	if got := byID["f1"]; got.Path != "a" || got.Size != 15 || got.Files != 2 || got.Depth != 1 {
		t.Fatalf("folder summary = %#v", got)
	}

	if _, exists := byID["f2"]; exists {
		t.Fatalf("depth-limited summary includes f2: %#v", summaries)
	}
}

func TestSummarizePreservesRepeatedPathsAndPlacements(t *testing.T) {
	t.Parallel()

	placements := []Placement{
		{File: File{ID: "folder-a", MimeType: FolderMimeType}, Path: "Same", Depth: 1, PlacementID: 2, ParentPlacementID: RootPlacementID},
		{File: File{ID: "folder-b", MimeType: FolderMimeType}, Path: "Same", Depth: 1, PlacementID: 3, ParentPlacementID: RootPlacementID},
		{File: File{ID: "file", Size: 3}, Path: "Same/data.bin", Depth: 2, PlacementID: 4, ParentPlacementID: 2},
		{File: File{ID: "file", Size: 5}, Path: "Same/data.bin", Depth: 2, PlacementID: 5, ParentPlacementID: 3},
	}

	summaries, err := Summarize(placements, "root", 1)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	byID := summariesByID(summaries)
	if got := byID["folder-a"]; got.Path != "Same" || got.Size != 3 || got.Files != 1 {
		t.Fatalf("folder-a summary = %#v", got)
	}

	if got := byID["folder-b"]; got.Path != "Same" || got.Size != 5 || got.Files != 1 {
		t.Fatalf("folder-b summary = %#v", got)
	}

	if got := byID["root"]; got.Size != 8 || got.Files != 2 {
		t.Fatalf("root summary = %#v", got)
	}
}

func TestSummarizeCountsShortcutPlacementWithoutBytes(t *testing.T) {
	t.Parallel()

	placements := []Placement{
		{File: File{ID: "target", Size: 10}, Path: "target.bin", Depth: 1, PlacementID: 2, ParentPlacementID: RootPlacementID},
		{File: File{ID: "shortcut", MimeType: ShortcutMimeType, Size: 999}, Path: "target link", Depth: 1, PlacementID: 3, ParentPlacementID: RootPlacementID},
	}

	summaries, err := Summarize(placements, "root", 0)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("summaries = %#v, want root only", summaries)
	}

	root := summaries[0]
	if root.ID != "root" || root.Size != 10 || root.Files != 2 {
		t.Fatalf("root summary = %#v", root)
	}
}

func TestSummarizeOmitsEmptyFolders(t *testing.T) {
	t.Parallel()

	placements := []Placement{
		{File: File{ID: "empty", MimeType: FolderMimeType}, Path: "Empty", Depth: 1, PlacementID: 2, ParentPlacementID: RootPlacementID},
	}

	summaries, err := Summarize(placements, "root", 0)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}

	if len(summaries) != 0 {
		t.Fatalf("summaries = %#v, want none", summaries)
	}
}

func TestSummarizeRejectsInvalidPlacementGraphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		placements []Placement
		want       error
	}{
		{
			name:       "missing placement",
			placements: []Placement{{File: File{ID: "file"}, ParentPlacementID: RootPlacementID}},
			want:       errPlacementMissing,
		},
		{
			name: "duplicate placement",
			placements: []Placement{
				{File: File{ID: "file-a"}, PlacementID: 2, ParentPlacementID: RootPlacementID},
				{File: File{ID: "file-b"}, PlacementID: 2, ParentPlacementID: RootPlacementID},
			},
			want: errPlacementExists,
		},
		{
			name:       "missing parent",
			placements: []Placement{{File: File{ID: "file"}, PlacementID: 2}},
			want:       errParentMissing,
		},
		{
			name:       "unknown parent",
			placements: []Placement{{File: File{ID: "file"}, PlacementID: 2, ParentPlacementID: 99}},
			want:       errParentUnknown,
		},
		{
			name: "folder cycle",
			placements: []Placement{
				{File: File{ID: "a", MimeType: FolderMimeType}, PlacementID: 2, ParentPlacementID: 3},
				{File: File{ID: "b", MimeType: FolderMimeType}, PlacementID: 3, ParentPlacementID: 2},
			},
			want: errAncestryCycle,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Summarize(tc.placements, "root", 0)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSortSummariesUsesDeterministicTies(t *testing.T) {
	t.Parallel()

	items := []Summary{
		{ID: "z", Path: "B", Size: 10, Files: 1},
		{ID: "b", Path: "A", Size: 10, Files: 1},
		{ID: "a", Path: "A", Size: 10, Files: 1},
		{ID: "small", Path: "C", Size: 5, Files: 2},
	}

	SortSummaries(items, "size", "desc")

	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.ID)
	}

	if want := []string{"a", "b", "z", "small"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted IDs = %#v, want %#v", got, want)
	}

	SortSummaries(items, "files", "asc")
	got = got[:0]

	for _, item := range items {
		got = append(got, item.ID)
	}

	if want := []string{"a", "b", "z", "small"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files-sorted IDs = %#v, want %#v", got, want)
	}
}

func summariesByID(summaries []Summary) map[string]Summary {
	out := make(map[string]Summary, len(summaries))
	for _, summary := range summaries {
		out[summary.ID] = summary
	}

	return out
}
