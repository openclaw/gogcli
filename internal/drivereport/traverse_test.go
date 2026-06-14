package drivereport

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

var errTestListFailed = errors.New("list failed")

type fakeSource struct {
	children map[string][]File
	errs     map[string]error
	calls    []string
}

func (s *fakeSource) Children(_ context.Context, parentID string) ([]File, error) {
	s.calls = append(s.calls, parentID)
	if err := s.errs[parentID]; err != nil {
		return nil, err
	}

	return s.children[parentID], nil
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: "_"},
		{in: ".", want: "_"},
		{in: "..", want: "_"},
		{in: "hello", want: "hello"},
		{in: "a/b", want: "a_b"},
		{in: "a\\b", want: "a_b"},
		{in: "  foo ", want: "foo"},
	}
	for _, tc := range cases {
		if got := sanitizeName(tc.in); got != tc.want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestJoinPath(t *testing.T) {
	t.Parallel()

	if got := joinPath("", "file"); got != "file" {
		t.Fatalf("joinPath empty = %q", got)
	}

	if got := joinPath("dir", "file"); got != "dir/file" {
		t.Fatalf("joinPath dir = %q", got)
	}
}

func TestTraversePreservesRepeatedPlacements(t *testing.T) {
	t.Parallel()

	source := &fakeSource{children: map[string][]File{
		"root": {
			{ID: "a", Name: "A", MimeType: FolderMimeType},
			{ID: "b", Name: "B", MimeType: FolderMimeType},
		},
		"a": {
			{ID: "shared", Name: "Shared", MimeType: FolderMimeType},
		},
		"b": {
			{ID: "shared", Name: "Shared", MimeType: FolderMimeType},
		},
		"shared": {
			{ID: "data", Name: "data.bin", MimeType: "application/octet-stream", Size: 10},
		},
	}}

	items, truncated, err := Traverse(context.Background(), source, Options{
		RootID:         "root",
		MaxDepth:       3,
		IncludeFiles:   true,
		IncludeFolders: true,
	})
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	if truncated {
		t.Fatal("Traverse reported truncation")
	}

	gotPaths := make([]string, 0, len(items))

	gotIDs := make([]PlacementID, 0, len(items))

	for _, item := range items {
		gotPaths = append(gotPaths, item.Path)
		gotIDs = append(gotIDs, item.PlacementID)
	}

	wantPaths := []string{
		"A",
		"B",
		"A/Shared",
		"B/Shared",
		"A/Shared/data.bin",
		"B/Shared/data.bin",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("paths = %#v, want %#v", gotPaths, wantPaths)
	}

	if want := []PlacementID{2, 3, 4, 5, 6, 7}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("placement IDs = %#v, want %#v", gotIDs, want)
	}

	if want := []string{"root", "a", "b", "shared", "shared"}; !reflect.DeepEqual(source.calls, want) {
		t.Fatalf("source calls = %#v, want %#v", source.calls, want)
	}

	if items[4].ParentPlacementID != 4 || items[5].ParentPlacementID != 5 {
		t.Fatalf("repeated child parents = %d/%d, want 4/5", items[4].ParentPlacementID, items[5].ParentPlacementID)
	}
}

func TestTraverseTreatsShortcutsAsZeroByteLeaves(t *testing.T) {
	t.Parallel()

	source := &fakeSource{children: map[string][]File{
		"root": {
			{
				ID:       "shortcut",
				Name:     "Folder link",
				MimeType: ShortcutMimeType,
				Size:     99,
				ShortcutDetails: &ShortcutDetails{
					TargetID:       "target",
					TargetMimeType: FolderMimeType,
				},
			},
			{ID: "target", Name: "Target", MimeType: FolderMimeType},
		},
		"target": {
			{ID: "child", Name: "child.txt", MimeType: "text/plain", Size: 5},
		},
	}}

	items, _, err := Traverse(context.Background(), source, Options{
		RootID:         "root",
		MaxDepth:       2,
		IncludeFiles:   true,
		IncludeFolders: true,
	})
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	if items[0].Size != 0 {
		t.Fatalf("shortcut size = %d, want 0", items[0].Size)
	}

	if want := []string{"root", "target"}; !reflect.DeepEqual(source.calls, want) {
		t.Fatalf("source calls = %#v, want %#v", source.calls, want)
	}
}

func TestTraverseFiltersOutputWithoutSuppressingFolders(t *testing.T) {
	t.Parallel()

	source := &fakeSource{children: map[string][]File{
		"root": {
			{ID: "folder", Name: "Folder", MimeType: FolderMimeType},
		},
		"folder": {
			{ID: "file", Name: "file.txt", MimeType: "text/plain"},
		},
	}}

	items, _, err := Traverse(context.Background(), source, Options{
		RootID:       "root",
		IncludeFiles: true,
	})
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("items = %#v, want one file", items)
	}

	if items[0].Path != "Folder/file.txt" || items[0].PlacementID != 3 || items[0].ParentPlacementID != 2 {
		t.Fatalf("file placement = %#v", items[0])
	}
}

func TestTraverseHonorsDepthAndPlacementLimit(t *testing.T) {
	t.Parallel()

	source := &fakeSource{children: map[string][]File{
		"root": {
			{ID: "folder", Name: "Folder", MimeType: FolderMimeType},
			{ID: "root-file", Name: "root.txt", MimeType: "text/plain"},
		},
		"folder": {
			{ID: "child", Name: "child.txt", MimeType: "text/plain"},
		},
	}}

	items, truncated, err := Traverse(context.Background(), source, Options{
		RootID:         "root",
		MaxDepth:       1,
		MaxItems:       2,
		IncludeFiles:   true,
		IncludeFolders: true,
	})
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	if !truncated {
		t.Fatal("Traverse did not report truncation at the placement limit")
	}

	if got := []string{items[0].Path, items[1].Path}; !reflect.DeepEqual(got, []string{"Folder", "root.txt"}) {
		t.Fatalf("paths = %#v", got)
	}

	if want := []string{"root"}; !reflect.DeepEqual(source.calls, want) {
		t.Fatalf("source calls = %#v, want %#v", source.calls, want)
	}
}

func TestTraverseRejectsBranchCycle(t *testing.T) {
	t.Parallel()

	source := &fakeSource{children: map[string][]File{
		"root": {
			{ID: "a", Name: "A", MimeType: FolderMimeType},
		},
		"a": {
			{ID: "root", Name: "Root again", MimeType: FolderMimeType},
		},
	}}

	_, _, err := Traverse(context.Background(), source, Options{
		RootID:         "root",
		IncludeFiles:   true,
		IncludeFolders: true,
	})
	if err == nil || err.Error() != `drive folder cycle detected at "A/Root again" (id root)` {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestTraverseReturnsSourceAndContextErrors(t *testing.T) {
	t.Parallel()

	source := &fakeSource{
		children: map[string][]File{},
		errs:     map[string]error{"root": errTestListFailed},
	}

	_, _, err := Traverse(context.Background(), source, Options{IncludeFiles: true})
	if !errors.Is(err, errTestListFailed) {
		t.Fatalf("source error = %v, want %v", err, errTestListFailed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source = &fakeSource{children: map[string][]File{}}

	_, _, err = Traverse(ctx, source, Options{IncludeFiles: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context error = %v, want canceled", err)
	}

	if len(source.calls) != 0 {
		t.Fatalf("canceled traversal called source: %#v", source.calls)
	}
}

func TestTraverseRequiresSource(t *testing.T) {
	t.Parallel()

	_, _, err := Traverse(context.Background(), nil, Options{})
	if !errors.Is(err, errSourceRequired) {
		t.Fatalf("error = %v, want source required", err)
	}
}
