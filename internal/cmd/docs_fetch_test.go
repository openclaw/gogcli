package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsFetchIDsRejectPathsAndDeduplicate(t *testing.T) {
	ids, err := readDocsFetchIDs(strings.NewReader("test-doc\nhttps://docs.google.com/document/d/test-doc/edit\nother-doc\n"))
	if err != nil || len(ids) != 2 {
		t.Fatalf("%v %v", ids, err)
	}
	for _, input := range []string{"../escape", "", "a/b"} {
		if _, err := readDocsFetchIDs(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestDocsFetchSnapshotAllTabsAndCacheIdentity(t *testing.T) {
	body := func(text string) *docs.DocumentTab {
		return &docs.DocumentTab{Body: &docs.Body{Content: []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: text}}}}}}}}
	}
	doc := &docs.Document{Title: "test", RevisionId: "revision-test", Tabs: []*docs.Tab{{TabProperties: &docs.TabProperties{TabId: "parent", Title: "Parent"}, DocumentTab: body("one"), ChildTabs: []*docs.Tab{{TabProperties: &docs.TabProperties{TabId: "child", Title: "Child"}, DocumentTab: body("two")}}}}}
	snapshot := makeDocsFetchSnapshot(doc, "test-doc", "a@example.com", "default")
	if len(snapshot.Tabs) != 2 || snapshot.Tabs[1].Text != "two" {
		t.Fatalf("%+v", snapshot)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if saveErr := saveDocsFetchSnapshot(root, "test-doc.json", snapshot, false); saveErr != nil {
		t.Fatal(saveErr)
	}
	if !validDocsFetchCache(root, "test-doc.json", "test-doc", "a@example.com", "default") || validDocsFetchCache(root, "test-doc.json", "test-doc", "b@example.com", "default") {
		t.Fatal("cache identity mismatch")
	}
	info, err := root.Stat("test-doc.json")
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("private snapshot: %v %v", info, err)
	}
}

func TestDocsFetchResumesSuccessAndRetriesFailure(t *testing.T) {
	fail := true
	requests := map[string]int{}
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/documents/")
		requests[id]++
		if id == "bad-doc" && fail {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("includeTabsContent") != "true" {
			t.Error("missing all-tabs")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"title":"test","body":{"content":[]}}`)
	}))
	defer cleanup()
	dir := t.TempDir()
	ids := filepath.Join(t.TempDir(), "ids.txt")
	if err := os.WriteFile(ids, []byte("good-doc\nbad-doc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		var out bytes.Buffer
		ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
		// Pin the client explicitly so the test needs no credential configuration.
		err := runKong(t, &DocsFetchCmd{}, []string{"--ids-file", ids, "--output-dir", dir}, ctx, &RootFlags{Account: "test@example.com", Client: "default"})
		if n == 0 && err == nil {
			t.Fatal("partial failure must fail")
		}
		if n == 1 && err != nil {
			t.Fatal(err)
		}
		fail = false
	}
	if requests["good-doc"] != 1 || requests["bad-doc"] != 2 {
		t.Fatalf("requests: %v", requests)
	}
}

func TestDocsFetchPreservesUnownedDestination(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		t.Run(fmt.Sprint(refresh), func(t *testing.T) {
			svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("conflict should not fetch")
				_, _ = io.WriteString(w, `{"title":"new"}`)
			}))
			defer cleanup()
			dir := t.TempDir()
			ids := filepath.Join(t.TempDir(), "ids.txt")
			if err := os.WriteFile(ids, []byte("test-doc\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "test-doc.json")
			original := []byte(`{"unrelated":"keep"}`)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
			err := (&DocsFetchCmd{IDsFile: ids, OutputDir: dir, Refresh: refresh}).Run(ctx, &RootFlags{Account: "test@example.com", Client: "default"})
			got, readErr := os.ReadFile(path)
			if err == nil || readErr != nil || !bytes.Equal(got, original) {
				t.Fatalf("err=%v read=%v contents=%s", err, readErr, got)
			}
		})
	}
}

func TestDocsFetchSnapshotNoClobberAndExplicitRefresh(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	snapshot := docsFetchedSnapshot{DocumentID: "doc", Account: "a", Client: "default", FetchedAt: "then", Tabs: []docsFetchedTab{{Text: "old"}}}
	if err := saveDocsFetchSnapshot(root, "doc.json", snapshot, false); err != nil {
		t.Fatal(err)
	}
	snapshot.Tabs[0].Text = "new"
	if err := saveDocsFetchSnapshot(root, "doc.json", snapshot, false); err == nil {
		t.Fatal("overwrote existing snapshot")
	}
	if err := saveDocsFetchSnapshot(root, "doc.json", snapshot, true); err != nil {
		t.Fatal(err)
	}
	if !validDocsFetchCache(root, "doc.json", "doc", "a", "default") {
		t.Fatal("refresh unreadable")
	}
}
