package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSanitizeDriveName(t *testing.T) {
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
		if got := sanitizeDriveName(tc.in); got != tc.want {
			t.Fatalf("sanitizeDriveName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestJoinDrivePath(t *testing.T) {
	if got := joinDrivePath("", "file"); got != "file" {
		t.Fatalf("joinDrivePath empty = %q", got)
	}
	if got := joinDrivePath("dir", "file"); got != "dir/file" {
		t.Fatalf("joinDrivePath dir = %q", got)
	}
}

func TestSummarizeDriveDu(t *testing.T) {
	items := []driveTreeItem{
		{ID: "f1", Path: "a", ParentID: "root", MimeType: driveMimeFolder, Depth: 1},
		{ID: "f2", Path: "a/b", ParentID: "f1", MimeType: driveMimeFolder, Depth: 2},
		{ID: "file1", Path: "a/file.txt", ParentID: "f1", MimeType: "text/plain", Size: 10},
		{ID: "file2", Path: "a/b/file2.txt", ParentID: "f2", MimeType: "text/plain", Size: 5},
	}

	summaries := summarizeDriveDu(items, "root", 1)
	if len(summaries) == 0 {
		t.Fatalf("expected summaries")
	}

	var rootSize int64
	var aSize int64
	for _, s := range summaries {
		if s.Path == "." {
			rootSize = s.Size
		}
		if s.Path == "a" {
			aSize = s.Size
		}
	}
	if rootSize != 15 {
		t.Fatalf("root size = %d, want 15", rootSize)
	}
	if aSize != 15 {
		t.Fatalf("a size = %d, want 15", aSize)
	}
}

func TestExecuteDriveTreeJSON(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		requireQuery(t, r, "supportsAllDrives", "true")
		requireQuery(t, r, "includeItemsFromAllDrives", "true")

		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "'root' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":           "folder1",
						"name":         "Reports",
						"mimeType":     driveMimeFolder,
						"modifiedTime": "2026-01-01T00:00:00Z",
					},
					{
						"id":           "file1",
						"name":         "root.txt",
						"mimeType":     "text/plain",
						"size":         "12",
						"modifiedTime": "2026-01-02T00:00:00Z",
					},
				},
			})
		case strings.Contains(q, "'folder1' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":           "file2",
						"name":         "child.txt",
						"mimeType":     "text/plain",
						"size":         "5",
						"modifiedTime": "2026-01-03T00:00:00Z",
					},
				},
			})
		default:
			t.Fatalf("unexpected query: %q", q)
		}
	}))
	defer closeSrv()
	stubDriveServiceForTest(t, svc)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@example.com", "drive", "tree", "--parent", "root", "--depth", "2"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Items []driveTreeItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Items) != 3 {
		t.Fatalf("items len = %d, want 3: %#v", len(parsed.Items), parsed.Items)
	}
	if parsed.Items[2].Path != "Reports/child.txt" {
		t.Fatalf("nested path = %q, want Reports/child.txt", parsed.Items[2].Path)
	}
}
