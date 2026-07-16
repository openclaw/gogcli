package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
)

type fakeDriveSyncClient struct {
	files         map[string]*drive.File
	children      map[string][]*drive.File
	writes        []string
	nextID        int
	corruptUpload bool
	beforeRead    func()
}

func newFakeDriveSyncClient() *fakeDriveSyncClient {
	parent := &drive.File{Id: "parent", Name: "parent", MimeType: driveMimeFolder}
	return &fakeDriveSyncClient{
		files:    map[string]*drive.File{parent.Id: parent},
		children: make(map[string][]*drive.File),
	}
}

func (f *fakeDriveSyncClient) Get(_ context.Context, fileID string) (*drive.File, error) {
	file := f.files[fileID]
	if file == nil {
		return nil, fmt.Errorf("missing file %s", fileID)
	}
	return file, nil
}

func (f *fakeDriveSyncClient) Children(_ context.Context, parentID string) ([]*drive.File, error) {
	return append([]*drive.File(nil), f.children[parentID]...), nil
}

func (f *fakeDriveSyncClient) CreateFolder(_ context.Context, name, parentID string) (*drive.File, error) {
	f.nextID++
	file := &drive.File{Id: fmt.Sprintf("folder-%d", f.nextID), Name: name, MimeType: driveMimeFolder, Parents: []string{parentID}}
	f.add(parentID, file)
	f.writes = append(f.writes, driveSyncCreateFolder+":"+name)
	return file, nil
}

func (f *fakeDriveSyncClient) CreateFile(_ context.Context, entry driveSyncLocalEntry, parentID string, content io.Reader) (*drive.File, error) {
	if f.beforeRead != nil {
		f.beforeRead()
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return nil, err
	}
	f.nextID++
	file := &drive.File{
		Id:          fmt.Sprintf("file-%d", f.nextID),
		Name:        entry.Name,
		MimeType:    entry.MimeType,
		Parents:     []string{parentID},
		Size:        int64(len(data)),
		Md5Checksum: testDriveSyncMD5(data),
	}
	if f.corruptUpload {
		file.Md5Checksum = testDriveSyncMD5([]byte("different"))
	}
	f.add(parentID, file)
	f.writes = append(f.writes, driveSyncCreateFile+":"+entry.Path)
	return file, nil
}

func (f *fakeDriveSyncClient) UpdateFile(_ context.Context, fileID string, entry driveSyncLocalEntry, content io.Reader) (*drive.File, error) {
	if f.beforeRead != nil {
		f.beforeRead()
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return nil, err
	}
	file := f.files[fileID]
	if file == nil {
		return nil, fmt.Errorf("missing update target %s", fileID)
	}
	file.Size = int64(len(data))
	file.Md5Checksum = testDriveSyncMD5(data)
	if f.corruptUpload {
		file.Md5Checksum = testDriveSyncMD5([]byte("different"))
	}
	f.writes = append(f.writes, driveSyncUpdateFile+":"+entry.Path)
	return file, nil
}

func (f *fakeDriveSyncClient) add(parentID string, file *drive.File) {
	f.files[file.Id] = file
	f.children[parentID] = append(f.children[parentID], file)
}

func testDriveSyncMD5(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func writeDriveSyncFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "b.txt"), []byte("bravo"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func scanDriveSyncTestTree(t *testing.T, root string) driveSyncLocalTree {
	t.Helper()

	tree, err := scanDriveSyncLocalTree(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := tree.Close(); closeErr != nil {
			t.Errorf("close local tree: %v", closeErr)
		}
	})
	return tree
}

func TestDriveSyncPushPlanApplyAndRepeat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := writeDriveSyncFixture(t)
	client := newFakeDriveSyncClient()

	tree := scanDriveSyncTestTree(t, root)
	plan, err := buildDriveSyncPlan(ctx, tree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}
	wantActions := []string{
		driveSyncCreateFolder + ":nested",
		driveSyncCreateFile + ":a.txt",
		driveSyncCreateFile + ":nested/b.txt",
	}
	if got := driveSyncActionNames(plan.Actions); strings.Join(got, ",") != strings.Join(wantActions, ",") {
		t.Fatalf("initial actions = %v, want %v", got, wantActions)
	}
	if applyErr := applyDriveSyncPlan(ctx, plan, client); applyErr != nil {
		t.Fatal(applyErr)
	}
	if got := len(client.writes); got != 3 {
		t.Fatalf("writes = %d, want 3", got)
	}
	nestedID := plan.Actions[0].FileID
	nestedFileID := plan.Actions[2].FileID
	if nestedID == "" || nestedFileID == "" {
		t.Fatalf("missing created IDs: %+v", plan.Actions)
	}

	repeatPlan, err := buildDriveSyncPlan(ctx, tree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}
	wantActions = []string{
		driveSyncSkipFile + ":a.txt",
		driveSyncSkipFile + ":nested/b.txt",
	}
	if got := driveSyncActionNames(repeatPlan.Actions); strings.Join(got, ",") != strings.Join(wantActions, ",") {
		t.Fatalf("repeat actions = %v, want %v", got, wantActions)
	}
	if applyErr := applyDriveSyncPlan(ctx, repeatPlan, client); applyErr != nil {
		t.Fatal(applyErr)
	}
	if got := len(client.writes); got != 3 {
		t.Fatalf("repeat writes = %d, want 3", got)
	}

	remoteOnly := &drive.File{Id: "remote-only", Name: "remote-only.txt", MimeType: mimeTextPlain, Size: 4, Md5Checksum: testDriveSyncMD5([]byte("keep"))}
	client.add("parent", remoteOnly)
	if writeErr := os.WriteFile(filepath.Join(root, "nested", "b.txt"), []byte("changed"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	changedTree := scanDriveSyncTestTree(t, root)
	changedPlan, err := buildDriveSyncPlan(ctx, changedTree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}
	wantActions = []string{
		driveSyncSkipFile + ":a.txt",
		driveSyncUpdateFile + ":nested/b.txt",
	}
	if got := driveSyncActionNames(changedPlan.Actions); strings.Join(got, ",") != strings.Join(wantActions, ",") {
		t.Fatalf("changed actions = %v, want %v", got, wantActions)
	}
	if changedPlan.Actions[1].FileID != nestedFileID {
		t.Fatalf("update target = %q, want %q", changedPlan.Actions[1].FileID, nestedFileID)
	}
	if err := applyDriveSyncPlan(ctx, changedPlan, client); err != nil {
		t.Fatal(err)
	}
	if changedPlan.Actions[1].FileID != nestedFileID {
		t.Fatalf("updated file ID changed: %q", changedPlan.Actions[1].FileID)
	}
	if client.files[remoteOnly.Id] != remoteOnly {
		t.Fatal("remote-only file was changed or removed")
	}
	if client.files[nestedID] == nil {
		t.Fatal("nested folder missing after update")
	}
}

func driveSyncActionNames(actions []driveSyncAction) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.Action+":"+action.Path)
	}
	return result
}

func TestDriveSyncPushPreflightConflictsDoNotWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := writeDriveSyncFixture(t)
	tree := scanDriveSyncTestTree(t, root)

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		client := newFakeDriveSyncClient()
		client.add("parent", &drive.File{Id: "a1", Name: "a.txt", MimeType: mimeTextPlain})
		client.add("parent", &drive.File{Id: "a2", Name: "a.txt", MimeType: mimeTextPlain})
		_, err := buildDriveSyncPlan(ctx, tree, "parent", true, client)
		if err == nil || !strings.Contains(err.Error(), "ambiguous Drive sibling") {
			t.Fatalf("error = %v", err)
		}
		if len(client.writes) != 0 {
			t.Fatalf("unexpected writes: %v", client.writes)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()

		client := newFakeDriveSyncClient()
		client.add("parent", &drive.File{Id: "a-folder", Name: "a.txt", MimeType: driveMimeFolder})
		_, err := buildDriveSyncPlan(ctx, tree, "parent", true, client)
		if err == nil || !strings.Contains(err.Error(), "drive type conflict") {
			t.Fatalf("error = %v", err)
		}
		if len(client.writes) != 0 {
			t.Fatalf("unexpected writes: %v", client.writes)
		}
	})

	t.Run("Google-native file", func(t *testing.T) {
		t.Parallel()

		client := newFakeDriveSyncClient()
		client.add("parent", &drive.File{Id: "a-doc", Name: "a.txt", MimeType: driveMimeGoogleDoc})
		_, err := buildDriveSyncPlan(ctx, tree, "parent", true, client)
		if err == nil || !strings.Contains(err.Error(), "drive type conflict") {
			t.Fatalf("error = %v", err)
		}
		if len(client.writes) != 0 {
			t.Fatalf("unexpected writes: %v", client.writes)
		}
	})
}

func TestDriveSyncPushRejectsProviderChecksumMismatch(t *testing.T) {
	t.Parallel()

	root := writeDriveSyncFixture(t)
	tree := scanDriveSyncTestTree(t, root)
	client := newFakeDriveSyncClient()
	client.corruptUpload = true
	plan, err := buildDriveSyncPlan(context.Background(), tree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}
	err = applyDriveSyncPlan(context.Background(), plan, client)
	if err == nil || !strings.Contains(err.Error(), "provider md5=") {
		t.Fatalf("error = %v", err)
	}
}

func TestDriveSyncPushUploadsVerifiedSnapshotWhenSourceChanges(t *testing.T) {
	t.Parallel()

	root := writeDriveSyncFixture(t)
	tree := scanDriveSyncTestTree(t, root)
	client := newFakeDriveSyncClient()
	existing := &drive.File{
		Id:          "existing-a",
		Name:        "a.txt",
		MimeType:    mimeTextPlain,
		Size:        3,
		Md5Checksum: testDriveSyncMD5([]byte("old")),
	}
	client.add("parent", existing)
	mutated := false
	client.beforeRead = func() {
		if mutated {
			return
		}
		mutated = true
		if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("omega"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := buildDriveSyncPlan(context.Background(), tree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyDriveSyncPlan(context.Background(), plan, client); err != nil {
		t.Fatal(err)
	}
	if !mutated {
		t.Fatal("source mutation hook did not run")
	}
	if existing.Id != "existing-a" || existing.Md5Checksum != testDriveSyncMD5([]byte("alpha")) {
		t.Fatalf("updated file did not use verified snapshot: %+v", existing)
	}
}

func TestDriveSyncPushBoundsDirectoryHandles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const directoryCount = 512
	for index := range directoryCount {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("dir-%04d", index)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tree := scanDriveSyncTestTree(t, root)
	if len(tree.Entries) != directoryCount || len(tree.dirInfos) != directoryCount+1 {
		t.Fatalf("entries=%d dirInfos=%d", len(tree.Entries), len(tree.dirInfos))
	}
}

func TestDriveSyncPushFolderOnlyDoesNotRequireTemp(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	tree := scanDriveSyncTestTree(t, root)
	client := newFakeDriveSyncClient()
	plan, err := buildDriveSyncPlan(context.Background(), tree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("TMPDIR", unavailable)
	t.Setenv("TMP", unavailable)
	t.Setenv("TEMP", unavailable)
	if err := applyDriveSyncPlan(context.Background(), plan, client); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(client.writes, ","); got != "create_folder:empty" {
		t.Fatalf("writes = %q", got)
	}
}

func TestDriveSyncPushRejectsSymlinkBeforeService(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == literalWindows {
		t.Skip("symlink setup requires privileges on Windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	_, err := scanDriveSyncLocalTree(root)
	if err == nil || !strings.Contains(err.Error(), "local symlink not supported") {
		t.Fatalf("error = %v", err)
	}
}

func TestDriveSyncPushRejectsRootSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == literalWindows {
		t.Skip("symlink setup requires privileges on Windows")
	}

	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(base, "linked")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatal(err)
	}
	_, err := scanDriveSyncLocalTree(linkedRoot)
	if err == nil || !strings.Contains(err.Error(), "local symlink not supported") {
		t.Fatalf("error = %v", err)
	}
}

func TestDriveSyncPushPinsSourceDirectoriesAcrossPathReplacement(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == literalWindows {
		t.Skip("symlink setup requires privileges on Windows")
	}

	ctx := context.Background()
	root := writeDriveSyncFixture(t)
	tree := scanDriveSyncTestTree(t, root)
	client := newFakeDriveSyncClient()
	plan, err := buildDriveSyncPlan(ctx, tree, "parent", true, client)
	if err != nil {
		t.Fatal(err)
	}

	movedRoot := root + "-moved"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "a.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}

	if err := applyDriveSyncPlan(ctx, plan, client); err != nil {
		t.Fatal(err)
	}
	var uploaded *drive.File
	for _, file := range client.children["parent"] {
		if file.Name == "a.txt" {
			uploaded = file
			break
		}
	}
	if uploaded == nil || uploaded.Md5Checksum != testDriveSyncMD5([]byte("alpha")) {
		t.Fatalf("uploaded wrong source after path replacement: %+v", uploaded)
	}
}

func TestDriveSyncPushRejectsInvalidUTF8Name(t *testing.T) {
	t.Parallel()
	invalidName := string([]byte{'b', 0xff, 'd'})
	err := validateDriveSyncLocalName(invalidName, invalidName)
	if err == nil || !strings.Contains(err.Error(), "local name is not valid UTF-8") {
		t.Fatalf("error = %v", err)
	}
}

func TestDriveSyncPushNoAllDrivesRejectsSharedParent(t *testing.T) {
	t.Parallel()

	root := writeDriveSyncFixture(t)
	tree := scanDriveSyncTestTree(t, root)
	client := newFakeDriveSyncClient()
	client.files["parent"].DriveId = "shared-drive"

	_, err := buildDriveSyncPlan(context.Background(), tree, "parent", false, client)
	if err == nil || !strings.Contains(err.Error(), "remove --no-all-drives") {
		t.Fatalf("error = %v", err)
	}
	if len(client.writes) != 0 {
		t.Fatalf("unexpected writes: %v", client.writes)
	}
}

func TestDriveSyncPushOutputDeterministicAndSingleLine(t *testing.T) {
	t.Parallel()

	unsafePath := "bad\n\x1b\u202ename.txt"
	actions := []driveSyncAction{
		{Action: driveSyncCreateFile, Path: unsafePath, ParentID: "parent", MimeType: mimeTextPlain, MD5: "abc", Size: 3, Reason: "missing"},
	}
	var jsonOut bytes.Buffer
	jsonCtx := outfmt.WithMode(newCmdRuntimeOutputContext(t, &jsonOut, io.Discard), outfmt.Mode{JSON: true})
	if err := writeDriveSyncPlan(jsonCtx, "/tmp/local", "parent", true, actions); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		DryRun  bool              `json:"dry_run"`
		Actions []driveSyncAction `json:"actions"`
		Summary driveSyncSummary  `json:"summary"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.DryRun || len(payload.Actions) != 1 || payload.Actions[0].Path != unsafePath || payload.Summary.CreateFiles != 1 {
		t.Fatalf("unexpected JSON payload: %s", jsonOut.String())
	}

	var plainOut bytes.Buffer
	plainCtx := outfmt.WithMode(newCmdRuntimeOutputContext(t, &plainOut, io.Discard), outfmt.Mode{Plain: true})
	if err := writeDriveSyncPlan(plainCtx, "/tmp/local", "parent", true, actions); err != nil {
		t.Fatal(err)
	}
	want := "ACTION\tPATH\tID\tREASON\n" +
		"create_file\tbad\\x0a\\x1b\\u202ename.txt\t\tmissing\n" +
		"summary\t\t\tcreate_folders=0 create_files=1 update_files=0 skip_files=0\n"
	if got := plainOut.String(); got != want {
		t.Fatalf("plain output = %q, want %q", got, want)
	}
	if strings.Contains(plainOut.String(), "\x1b") || strings.Contains(plainOut.String(), "\u202e") {
		t.Fatalf("plain output contains terminal controls: %q", plainOut.String())
	}
}

func TestGoogleDriveSyncClientUsesSharedDriveFlags(t *testing.T) {
	t.Parallel()

	var requests []string
	svc, closeServer := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireSupportsAllDrives(t, r)
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files/parent"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "parent", "name": "parent", "mimeType": driveMimeFolder, "driveId": "shared-drive"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files"):
			requireQuery(t, r, "includeItemsFromAllDrives", "true")
			requireQuery(t, r, "corpora", "drive")
			requireQuery(t, r, "driveId", "shared-drive")
			if r.URL.Query().Get("pageToken") == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{}, "nextPageToken": "next"})
			} else {
				requireQuery(t, r, "pageToken", "next")
				_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{}})
			}
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "file-new", "name": "a.txt"})
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "folder-new", "name": "nested", "mimeType": driveMimeFolder})
		case r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "file-existing", "name": "a.txt"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer closeServer()

	client := &googleDriveSyncClient{service: svc}
	ctx := context.Background()
	if _, err := client.Get(ctx, "parent"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Children(ctx, "parent"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateFolder(ctx, "nested", "parent"); err != nil {
		t.Fatal(err)
	}
	entry := driveSyncLocalEntry{Name: "a.txt", Path: "a.txt", MimeType: mimeTextPlain}
	if _, err := client.CreateFile(ctx, entry, "parent", strings.NewReader("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateFile(ctx, "file-existing", entry, strings.NewReader("b")); err != nil {
		t.Fatal(err)
	}
	sort.Strings(requests)
	if len(requests) != 6 {
		t.Fatalf("requests = %v", requests)
	}
}

func TestListDriveSyncChildrenRejectsIncompleteSearch(t *testing.T) {
	t.Parallel()

	svc, closeServer := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{}, "incompleteSearch": true})
	}))
	defer closeServer()

	_, err := listDriveSyncChildren(context.Background(), svc, "parent", "")
	if err == nil || !strings.Contains(err.Error(), "incomplete child listing") {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteDriveSyncPushDryRunPlansWithoutWrites(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), " backup")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCount := 0
	svc, closeServer := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			writeCount++
			http.Error(w, "unexpected write", http.StatusInternalServerError)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/files/parent") {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "parent", "name": "parent", "mimeType": driveMimeFolder})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/files") {
			requireQuery(t, r, "corpora", "user")
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer closeServer()

	result := executeWithDriveTestService(t, []string{
		"--account", "test@example.com",
		"--json",
		"--dry-run",
		"drive", "sync", "push", root,
		"--parent", "parent",
	}, svc)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr: %s", result.err, result.stderr)
	}
	if writeCount != 0 {
		t.Fatalf("dry-run writes = %d", writeCount)
	}
	var payload struct {
		DryRun  bool              `json:"dry_run"`
		Actions []driveSyncAction `json:"actions"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("parse output: %v\n%s", err, result.stdout)
	}
	if !payload.DryRun || len(payload.Actions) != 1 || payload.Actions[0].Action != driveSyncCreateFile {
		t.Fatalf("unexpected dry-run plan: %s", result.stdout)
	}
}

func TestDriveSyncPushHelp(t *testing.T) {
	t.Parallel()

	result := executeWithTestRuntime(t, []string{"drive", "sync", "push", "--help"}, nil)
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, want := range []string{"<localDirectory>", "--parent=STRING", "--[no-]all-drives", "no remote deletes"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("help missing %q:\n%s", want, result.stdout)
		}
	}
}
