package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

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
					{
						"id":           "shortcut1",
						"name":         "Reports elsewhere",
						"mimeType":     driveMimeShortcut,
						"modifiedTime": "2026-01-02T00:00:00Z",
						"shortcutDetails": map[string]any{
							"targetId":       "folder-target",
							"targetMimeType": driveMimeFolder,
						},
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

	result := executeWithDriveTestService(t, []string{"--json", "--account", "a@example.com", "drive", "tree", "--parent", "root", "--depth", "2"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", result.err, result.stderr)
	}

	var parsed struct {
		Items []driveTreeItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if len(parsed.Items) != 4 {
		t.Fatalf("items len = %d, want 4: %#v", len(parsed.Items), parsed.Items)
	}
	if parsed.Items[2].Path != "Reports elsewhere" || driveShortcutDetailsTargetID(parsed.Items[2].ShortcutDetails) != "folder-target" {
		t.Fatalf("shortcut item = %#v", parsed.Items[2])
	}
	if parsed.Items[3].Path != "Reports/child.txt" {
		t.Fatalf("nested path = %q, want Reports/child.txt", parsed.Items[3].Path)
	}

	for _, tc := range []struct {
		name       string
		args       []string
		wantHeader string
	}{
		{
			name:       "tree",
			args:       []string{"--plain", "--account", "a@example.com", "drive", "tree", "--parent", "root", "--depth", "2"},
			wantHeader: "PATH\tTYPE\tSIZE\tMODIFIED\tID\n",
		},
		{
			name:       "inventory",
			args:       []string{"--plain", "--account", "a@example.com", "drive", "inventory", "--parent", "root", "--depth", "2"},
			wantHeader: "PATH\tTYPE\tSIZE\tMODIFIED\tOWNER\tID\n",
		},
	} {
		t.Run(tc.name+" plain schema", func(t *testing.T) {
			plainResult := executeWithDriveTestService(t, tc.args, svc)
			if plainResult.err != nil {
				t.Fatalf("Execute: %v\nstderr=%s", plainResult.err, plainResult.stderr)
			}
			if !strings.HasPrefix(plainResult.stdout, tc.wantHeader) {
				t.Fatalf("plain output header = %q, want prefix %q", plainResult.stdout, tc.wantHeader)
			}
			if strings.Contains(plainResult.stdout, "TARGET_ID") {
				t.Fatalf("plain output schema changed unexpectedly: %q", plainResult.stdout)
			}
		})
	}
}

func TestDriveReportingPreservesRepeatedPlacements(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}

		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "'root' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "a", "name": "A", "mimeType": driveMimeFolder},
					{"id": "b", "name": "B", "mimeType": driveMimeFolder},
				},
			})
		case strings.Contains(q, "'a' in parents"), strings.Contains(q, "'b' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "shared-folder", "name": "Shared", "mimeType": driveMimeFolder},
				},
			})
		case strings.Contains(q, "'shared-folder' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "shared-file", "name": "data.bin", "mimeType": "application/octet-stream", "size": "10"},
				},
			})
		default:
			t.Fatalf("unexpected query: %q", q)
		}
	}))
	defer closeSrv()

	treeResult := executeWithDriveTestService(t, []string{
		"--json", "--account", "a@example.com",
		"drive", "tree", "--parent", "root", "--depth", "3",
	}, svc)
	if treeResult.err != nil {
		t.Fatalf("tree: %v\nstderr=%s", treeResult.err, treeResult.stderr)
	}

	var tree struct {
		Items []driveTreeItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(treeResult.stdout), &tree); err != nil {
		t.Fatalf("tree JSON: %v\nout=%q", err, treeResult.stdout)
	}
	paths := make(map[string]string, len(tree.Items))
	for _, item := range tree.Items {
		paths[item.Path] = item.ID
	}
	for path, id := range map[string]string{
		"A/Shared":          "shared-folder",
		"B/Shared":          "shared-folder",
		"A/Shared/data.bin": "shared-file",
		"B/Shared/data.bin": "shared-file",
	} {
		if got := paths[path]; got != id {
			t.Errorf("tree path %q id = %q, want %q; items=%#v", path, got, id, tree.Items)
		}
	}

	truncatedResult := executeWithDriveTestService(t, []string{
		"--json", "--account", "a@example.com",
		"drive", "tree", "--parent", "root", "--depth", "3", "--max", "3",
	}, svc)
	if truncatedResult.err != nil {
		t.Fatalf("truncated tree: %v\nstderr=%s", truncatedResult.err, truncatedResult.stderr)
	}
	var truncated struct {
		Items     []driveTreeItem `json:"items"`
		Truncated bool            `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(truncatedResult.stdout), &truncated); err != nil {
		t.Fatalf("truncated tree JSON: %v\nout=%q", err, truncatedResult.stdout)
	}
	if len(truncated.Items) != 3 || !truncated.Truncated {
		t.Fatalf("truncated tree = items %d truncated %t, want 3/true", len(truncated.Items), truncated.Truncated)
	}

	duResult := executeWithDriveTestService(t, []string{
		"--json", "--account", "a@example.com",
		"drive", "du", "--parent", "root", "--depth", "2", "--sort", "path",
	}, svc)
	if duResult.err != nil {
		t.Fatalf("du: %v\nstderr=%s", duResult.err, duResult.stderr)
	}

	var du struct {
		Folders []driveDuSummary `json:"folders"`
	}
	if err := json.Unmarshal([]byte(duResult.stdout), &du); err != nil {
		t.Fatalf("du JSON: %v\nout=%q", err, duResult.stdout)
	}
	summaries := make(map[string]driveDuSummary, len(du.Folders))
	for _, summary := range du.Folders {
		summaries[summary.Path] = summary
	}
	for _, path := range []string{".", "A", "B", "A/Shared", "B/Shared"} {
		summary, ok := summaries[path]
		if !ok {
			t.Errorf("missing du path %q: %#v", path, du.Folders)
			continue
		}
		wantSize := int64(20)
		wantFiles := 2
		if path != "." {
			wantSize = 10
			wantFiles = 1
		}
		if summary.Size != wantSize || summary.Files != wantFiles {
			t.Errorf("du path %q = size %d files %d, want size %d files %d", path, summary.Size, summary.Files, wantSize, wantFiles)
		}
	}
}

func TestDriveDuCountsShortcutWithoutTargetContent(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		if q := r.URL.Query().Get("q"); !strings.Contains(q, "'root' in parents") {
			t.Fatalf("unexpected query: %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "target", "name": "target.bin", "mimeType": "application/octet-stream", "size": "10"},
				{
					"id":       "shortcut",
					"name":     "target link",
					"mimeType": driveMimeShortcut,
					"size":     "999",
					"shortcutDetails": map[string]any{
						"targetId":       "target",
						"targetMimeType": "application/octet-stream",
					},
				},
			},
		})
	}))
	defer closeSrv()

	result := executeWithDriveTestService(t, []string{
		"--json", "--account", "a@example.com",
		"drive", "du", "--parent", "root",
	}, svc)
	if result.err != nil {
		t.Fatalf("du: %v\nstderr=%s", result.err, result.stderr)
	}
	var parsed struct {
		Folders []driveDuSummary `json:"folders"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("du JSON: %v\nout=%q", err, result.stdout)
	}
	if len(parsed.Folders) != 1 {
		t.Fatalf("folders = %#v, want root only", parsed.Folders)
	}
	root := parsed.Folders[0]
	if root.Path != "." || root.Size != 10 || root.Files != 2 {
		t.Fatalf("root summary = %#v, want size 10 and 2 file placements", root)
	}
}

func TestDriveTreeRejectsFolderCycle(t *testing.T) {
	svc, closeSrv := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}

		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "'root' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "a", "name": "A", "mimeType": driveMimeFolder},
				},
			})
		case strings.Contains(q, "'a' in parents"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "root", "name": "Root again", "mimeType": driveMimeFolder},
				},
			})
		default:
			t.Fatalf("unexpected query: %q", q)
		}
	}))
	defer closeSrv()

	result := executeWithDriveTestService(t, []string{
		"--json", "--account", "a@example.com",
		"drive", "tree", "--parent", "root", "--depth", "3",
	}, svc)
	if result.err == nil {
		t.Fatalf("expected cycle error, got stdout=%q stderr=%q", result.stdout, result.stderr)
	}
	if !strings.Contains(result.err.Error(), `drive folder cycle detected at "A/Root again" (id root)`) {
		t.Fatalf("cycle error = %q", result.err)
	}
}
