package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func syncTestTab(id, title, text string) *docs.Tab {
	return &docs.Tab{TabProperties: &docs.TabProperties{TabId: id, Title: title}, DocumentTab: &docs.DocumentTab{Body: &docs.Body{Content: []*docs.StructuralElement{{StartIndex: 1, EndIndex: int64(utf16Len(text) + 1), Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: text}}}}}}}}}
}

func TestDocsSyncPreservesUnmanagedTabsAndSkipsUnchanged(t *testing.T) {
	managed := syncTestTab("managed", "Managed", "rendered")
	doc := &docs.Document{Tabs: []*docs.Tab{syncTestTab("human", "Human", "notes"), managed}}
	state := docsSyncState{Entries: map[string]docsSyncEntry{"managed": {SourceHash: hashDocsSyncBytes([]byte("# source")), RemoteHash: hashDocsSyncTab(managed)}}}
	inputs := []docsSyncInput{{File: "source.md", Tab: "Managed"}}
	actions, err := planDocsSync(doc, inputs, []string{"# source"}, state, false, false)
	if err != nil || len(actions) != 1 || actions[0].Action != "skip" || actions[0].TabID != "managed" {
		t.Fatalf("%+v %v", actions, err)
	}
	managed.DocumentTab.Body.Content[0].Paragraph.Elements[0].TextRun.Content = "human edit"
	if _, err := planDocsSync(doc, inputs, []string{"changed"}, state, false, false); err == nil {
		t.Fatal("remote edit should block")
	}
	if _, err := planDocsSync(doc, inputs, []string{"changed"}, state, false, true); err != nil {
		t.Fatal(err)
	}
	if doc.Tabs[0].DocumentTab.Body.Content[0].Paragraph.Elements[0].TextRun.Content != "notes" {
		t.Fatal("unmanaged tab mutated")
	}
}

func TestDocsSyncRejectsDuplicateTargetsAndAmbiguousTitles(t *testing.T) {
	doc := &docs.Document{Tabs: []*docs.Tab{syncTestTab("one", "Same", ""), syncTestTab("two", "Same", "")}}
	for _, inputs := range [][]docsSyncInput{{{Tab: "Same"}}, {{Tab: "one"}, {Tab: "one"}}} {
		if _, err := planDocsSync(doc, inputs, []string{"a", "b"}, docsSyncState{}, false, false); err == nil {
			t.Fatal("ambiguous manifest accepted")
		}
	}
}

func TestDocsSyncPreviewDoesNotWriteRemoteOrState(t *testing.T) {
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatal("preview mutated document")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&docs.Document{Tabs: []*docs.Tab{syncTestTab("target", "Target", "old")}})
	}))
	defer cleanup()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(filepath.Join(dir, "source.md"), []byte("# Hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"tabs":[{"file":"source.md","tab":"target"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ctx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	if err := runKong(t, &DocsSyncCmd{}, []string{"test-doc", "--manifest", manifest, "--apply"}, ctx, &RootFlags{Account: "test@example.com", Client: "default", DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"applied": false`) {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(manifest + ".state.json"); !os.IsNotExist(err) {
		t.Fatalf("preview wrote state: %v", err)
	}
}

func TestDocsSyncApplyThenRerunSkipsWrites(t *testing.T) {
	managed := syncTestTab("target", "Target", "old\n")
	human := syncTestTab("human", "Human", "human notes\n")
	posts := 0
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(&docs.Document{DocumentId: "test-doc", Tabs: []*docs.Tab{human, managed}})
			return
		}
		posts++
		var req docs.BatchUpdateDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		for _, request := range req.Requests {
			if request.DeleteContentRange != nil && request.DeleteContentRange.Range.TabId != "target" {
				t.Error("deleted another tab")
			}
			if request.InsertText != nil {
				if request.InsertText.Location.TabId != "target" {
					t.Error("wrote another tab")
				}
				managed = syncTestTab("target", "Target", request.InsertText.Text+"\n")
			}
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer cleanup()
	driveSvc, driveCleanup := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "comments") {
			t.Errorf("unexpected Drive request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"comments":[]}`)
	}))
	defer driveCleanup()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(filepath.Join(dir, "source.md"), []byte("# Hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"tabs":[{"file":"source.md","tab":"target"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	firstPosts := 0
	for n := 0; n < 2; n++ {
		var out bytes.Buffer
		ctx := withDriveTestService(withDocsTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc), driveSvc)
		if err := runKong(t, &DocsSyncCmd{}, []string{"test-doc", "--manifest", manifest, "--apply"}, ctx, &RootFlags{Account: "test@example.com", Client: "default"}); err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("stdout must be one JSON object: %v %s", err, &out)
		}
		if n == 0 {
			firstPosts = posts
			if firstPosts == 0 {
				t.Fatal("no content written")
			}
		} else if posts != firstPosts {
			t.Fatal("unchanged sync wrote content again")
		}
	}
	if human.DocumentTab.Body.Content[0].Paragraph.Elements[0].TextRun.Content != "human notes\n" {
		t.Fatal("human tab changed")
	}
}

func TestDocsSyncImageURLRotation(t *testing.T) {
	for _, positioned := range []bool{false, true} {
		tab := syncTestTab("managed", "Managed", "text")
		image := &docs.ImageProperties{ContentUri: "https://example.test/old", SourceUri: "https://example.test/source"}
		embedded := &docs.EmbeddedObject{ImageProperties: image}
		if positioned {
			tab.DocumentTab.PositionedObjects = map[string]docs.PositionedObject{"image": {PositionedObjectProperties: &docs.PositionedObjectProperties{EmbeddedObject: embedded}}}
		} else {
			tab.DocumentTab.InlineObjects = map[string]docs.InlineObject{"image": {InlineObjectProperties: &docs.InlineObjectProperties{EmbeddedObject: embedded}}}
		}
		hash := hashDocsSyncTab(tab)
		image.ContentUri = "https://example.test/rotated"
		if hashDocsSyncTab(tab) != hash {
			t.Fatal("temporary URL changes fingerprint")
		}
		if image.ContentUri != "https://example.test/rotated" {
			t.Fatal("hash mutated input")
		}
		image.SourceUri = "https://example.test/changed-source"
		if hashDocsSyncTab(tab) == hash {
			t.Fatal("source change ignored")
		}
	}
}

func TestDocsSyncCompleteExistingStateRetainsTextBaseline(t *testing.T) {
	tab := syncTestTab("managed", "Managed", "existing text")
	b, err := json.Marshal(tab.DocumentTab)
	if err != nil {
		t.Fatal(err)
	}
	previous := docsSyncState{DocumentID: "doc", Account: "a@example.com", Client: "default", Entries: map[string]docsSyncEntry{"managed": {SourceHash: hashDocsSyncBytes([]byte("# source")), RemoteHash: hashDocsSyncBytes(b)}}}
	path := filepath.Join(t.TempDir(), "state.json")
	if saveErr := saveDocsSyncState(path, previous); saveErr != nil {
		t.Fatal(saveErr)
	}
	state, err := readDocsSyncState(path, "doc", "a@example.com", "default")
	if err != nil {
		t.Fatal(err)
	}
	actions, err := planDocsSync(&docs.Document{Tabs: []*docs.Tab{tab}}, []docsSyncInput{{File: "source.md", Tab: "managed"}}, []string{"# source"}, state, false, false)
	if err != nil || len(actions) != 1 || actions[0].Action != "skip" {
		t.Fatalf("%v %v", actions, err)
	}
}
