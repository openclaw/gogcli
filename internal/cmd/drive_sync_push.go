package cmd

import (
	"context"
	"crypto/md5" // #nosec G501 -- Google Drive exposes MD5 checksums for binary file reconciliation.
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

const (
	driveSyncCreateFolder = "create_folder"
	driveSyncCreateFile   = "create_file"
	driveSyncUpdateFile   = "update_file"
	driveSyncSkipFile     = "skip_file"
	driveSyncFields       = "id,name,mimeType,size,md5Checksum,driveId"
)

type DriveSyncCmd struct {
	Push DriveSyncPushCmd `cmd:"" name:"push" help:"Push a local directory's contents into a Drive folder (no remote deletes)"`
}

type DriveSyncPushCmd struct {
	LocalDir  string `arg:"" name:"localDirectory" help:"Local directory to push" type:"path"`
	Parent    string `name:"parent" help:"Existing destination Drive folder ID" required:""`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
}

type driveSyncLocalEntry struct {
	Path       string
	ParentPath string
	Name       string
	MimeType   string
	MD5        string
	Size       int64
	IsDir      bool
}

type driveSyncLocalTree struct {
	Root     string
	Entries  map[string]driveSyncLocalEntry
	Children map[string][]driveSyncLocalEntry
	root     *os.Root
	dirInfos map[string]os.FileInfo
}

type driveSyncAction struct {
	Action     string `json:"action"`
	Path       string `json:"path"`
	FileID     string `json:"file_id,omitempty"`
	ParentID   string `json:"parent_id,omitempty"`
	MimeType   string `json:"mime_type,omitempty"`
	MD5        string `json:"md5,omitempty"`
	Reason     string `json:"reason"`
	Size       int64  `json:"size,omitempty"`
	ParentPath string `json:"parent_path,omitempty"`
}

type driveSyncSummary struct {
	CreateFolders int `json:"create_folders"`
	CreateFiles   int `json:"create_files"`
	UpdateFiles   int `json:"update_files"`
	SkipFiles     int `json:"skip_files"`
}

type driveSyncPlan struct {
	Actions   []driveSyncAction
	folderIDs map[string]string
	entries   map[string]driveSyncLocalEntry
	localRoot *os.Root
	dirInfos  map[string]os.FileInfo
}

type driveSyncClient interface {
	Get(ctx context.Context, fileID string) (*drive.File, error)
	Children(ctx context.Context, parentID string) ([]*drive.File, error)
	CreateFolder(ctx context.Context, name, parentID string) (*drive.File, error)
	CreateFile(ctx context.Context, entry driveSyncLocalEntry, parentID string, content io.Reader) (*drive.File, error)
	UpdateFile(ctx context.Context, fileID string, entry driveSyncLocalEntry, content io.Reader) (*drive.File, error)
}

type googleDriveSyncClient struct {
	service *drive.Service
	driveID string
}

func (c *DriveSyncPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	if strings.TrimSpace(c.LocalDir) == "" {
		return usage("empty localDirectory")
	}
	localDir := c.LocalDir
	parentID := strings.TrimSpace(c.Parent)
	if parentID == "" {
		return usage("missing --parent")
	}

	tree, err := scanDriveSyncLocalTree(localDir)
	if err != nil {
		return err
	}
	defer func() { _ = tree.Close() }()

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	client := &googleDriveSyncClient{service: svc}
	plan, err := buildDriveSyncPlan(ctx, tree, parentID, c.AllDrives, client)
	if err != nil {
		return err
	}

	dryRun := flags != nil && flags.DryRun
	if !dryRun {
		if err := applyDriveSyncPlan(ctx, plan, client); err != nil {
			return err
		}
	}

	return writeDriveSyncPlan(ctx, tree.Root, parentID, dryRun, plan.Actions)
}

func scanDriveSyncLocalTree(localDir string) (driveSyncLocalTree, error) {
	expanded, err := config.ExpandPath(localDir)
	if err != nil {
		return driveSyncLocalTree{}, err
	}
	root, rootInfo, absolutePath, err := openDriveSyncRoot(expanded)
	if err != nil {
		return driveSyncLocalTree{}, err
	}

	tree := driveSyncLocalTree{
		Root:     absolutePath,
		Entries:  make(map[string]driveSyncLocalEntry),
		Children: make(map[string][]driveSyncLocalEntry),
		root:     root,
		dirInfos: map[string]os.FileInfo{"": rootInfo},
	}
	if err := scanDriveSyncDirectories(&tree); err != nil {
		_ = tree.Close()
		return driveSyncLocalTree{}, err
	}
	return tree, nil
}

func openDriveSyncRoot(localDir string) (*os.Root, os.FileInfo, string, error) {
	absolutePath, err := filepath.Abs(localDir)
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolve local directory: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)
	before, err := os.Lstat(absolutePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("inspect local directory: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, "", fmt.Errorf("local symlink not supported: %q", absolutePath)
	}
	if !before.IsDir() {
		return nil, nil, "", usagef("localDirectory is not a directory: %q", absolutePath)
	}
	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolve local directory symlinks: %w", err)
	}

	volume := filepath.VolumeName(canonicalPath)
	filesystemRoot := volume + string(filepath.Separator)
	current, err := os.OpenRoot(filesystemRoot)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open filesystem root for local directory: %w", err)
	}

	relative := strings.TrimPrefix(canonicalPath, filesystemRoot)
	components := strings.Split(relative, string(filepath.Separator))
	visited := filesystemRoot
	for _, component := range components {
		if component == "" {
			continue
		}
		visited = filepath.Join(visited, component)
		next, _, openErr := openDriveSyncChildRoot(current, component, visited)
		if openErr != nil {
			_ = current.Close()
			return nil, nil, "", openErr
		}
		_ = current.Close()
		current = next
	}
	openedFile, err := current.Open(".")
	if err != nil {
		_ = current.Close()
		return nil, nil, "", fmt.Errorf("verify local directory: %w", err)
	}
	opened, statErr := openedFile.Stat()
	closeErr := openedFile.Close()
	if statErr != nil {
		_ = current.Close()
		return nil, nil, "", fmt.Errorf("verify local directory: %w", statErr)
	}
	if closeErr != nil {
		_ = current.Close()
		return nil, nil, "", fmt.Errorf("verify local directory: %w", closeErr)
	}
	after, err := os.Lstat(absolutePath)
	if err != nil {
		_ = current.Close()
		return nil, nil, "", fmt.Errorf("reinspect local directory: %w", err)
	}
	if after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = current.Close()
		return nil, nil, "", fmt.Errorf("local directory changed while opening: %q", absolutePath)
	}
	return current, opened, absolutePath, nil
}

func openDriveSyncChildRoot(parent *os.Root, name, displayPath string) (*os.Root, os.FileInfo, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect local directory %q: %w", displayPath, err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("local symlink not supported: %q", displayPath)
	}
	if !before.IsDir() {
		return nil, nil, usagef("localDirectory is not a directory: %q", displayPath)
	}

	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, nil, fmt.Errorf("open local directory %q: %w", displayPath, err)
	}
	openedFile, err := child.Open(".")
	if err != nil {
		_ = child.Close()
		return nil, nil, fmt.Errorf("verify local directory %q: %w", displayPath, err)
	}
	opened, statErr := openedFile.Stat()
	closeErr := openedFile.Close()
	if statErr != nil {
		_ = child.Close()
		return nil, nil, fmt.Errorf("verify local directory %q: %w", displayPath, statErr)
	}
	if closeErr != nil {
		_ = child.Close()
		return nil, nil, fmt.Errorf("verify local directory %q: %w", displayPath, closeErr)
	}
	after, err := parent.Lstat(name)
	if err != nil {
		_ = child.Close()
		return nil, nil, fmt.Errorf("reinspect local directory %q: %w", displayPath, err)
	}
	if after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = child.Close()
		return nil, nil, fmt.Errorf("local directory changed while opening: %q", displayPath)
	}
	return child, opened, nil
}

func scanDriveSyncDirectories(tree *driveSyncLocalTree) error {
	pending := []string{""}
	for len(pending) > 0 {
		relative := pending[0]
		pending = pending[1:]
		directory, err := openDriveSyncDirectory(tree.root, tree.dirInfos, relative)
		if err != nil {
			return err
		}
		children, scanErr := scanDriveSyncDirectory(tree, directory, relative)
		closeErr := directory.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return fmt.Errorf("close local directory %q: %w", relative, closeErr)
		}
		pending = append(pending, children...)
	}
	return nil
}

func scanDriveSyncDirectory(tree *driveSyncLocalTree, directory *os.Root, relativeParent string) ([]string, error) {
	directoryFile, err := directory.Open(".")
	if err != nil {
		return nil, fmt.Errorf("read local directory %q: %w", relativeParent, err)
	}
	entries, readErr := directoryFile.ReadDir(-1)
	closeErr := directoryFile.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read local directory %q: %w", relativeParent, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("read local directory %q: %w", relativeParent, closeErr)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	childDirectories := make([]string, 0)
	for _, dirEntry := range entries {
		name := dirEntry.Name()
		relative := path.Join(relativeParent, name)
		if nameErr := validateDriveSyncLocalName(name, relative); nameErr != nil {
			return nil, nameErr
		}
		info, infoErr := directory.Lstat(name)
		if infoErr != nil {
			return nil, fmt.Errorf("inspect local path %q: %w", relative, infoErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("local symlink not supported: %q", relative)
		}

		entry := driveSyncLocalEntry{
			Path:       relative,
			ParentPath: relativeParent,
			Name:       name,
			IsDir:      info.IsDir(),
		}
		switch {
		case info.IsDir():
			child, childInfo, childErr := openDriveSyncChildRoot(directory, name, relative)
			if childErr != nil {
				return nil, childErr
			}
			closeErr := child.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("close local directory %q: %w", relative, closeErr)
			}
			tree.dirInfos[relative] = childInfo
			childDirectories = append(childDirectories, relative)
		case info.Mode().IsRegular():
			entry.Size = info.Size()
			entry.MimeType = guessMimeType(name)
			entry.MD5, infoErr = driveSyncChecksum(directory, name, relative)
			if infoErr != nil {
				return nil, infoErr
			}
		default:
			return nil, fmt.Errorf("unsupported local file type: %q", relative)
		}

		tree.Entries[relative] = entry
		tree.Children[relativeParent] = append(tree.Children[relativeParent], entry)
	}
	return childDirectories, nil
}

func (tree *driveSyncLocalTree) Close() error {
	if tree.root == nil {
		return nil
	}
	err := tree.root.Close()
	tree.root = nil
	tree.dirInfos = nil
	return err
}

func openDriveSyncDirectory(treeRoot *os.Root, dirInfos map[string]os.FileInfo, relative string) (*os.Root, error) {
	current, currentInfo, err := openDriveSyncChildRoot(treeRoot, ".", ".")
	if err != nil {
		return nil, err
	}
	if expected := dirInfos[""]; expected == nil || !os.SameFile(expected, currentInfo) {
		_ = current.Close()
		return nil, fmt.Errorf("local root directory changed after preflight")
	}
	if relative == "" {
		return current, nil
	}

	prefix := ""
	for _, component := range strings.Split(relative, "/") {
		prefix = path.Join(prefix, component)
		next, nextInfo, openErr := openDriveSyncChildRoot(current, component, prefix)
		_ = current.Close()
		if openErr != nil {
			return nil, openErr
		}
		expected := dirInfos[prefix]
		if expected == nil || !os.SameFile(expected, nextInfo) {
			_ = next.Close()
			return nil, fmt.Errorf("local directory changed after preflight: %q", prefix)
		}
		current = next
	}
	return current, nil
}

func validateDriveSyncLocalName(name, relative string) error {
	if !utf8.ValidString(name) {
		return fmt.Errorf("local name is not valid UTF-8: %q", relative)
	}
	return nil
}

func driveSyncChecksum(directory *os.Root, name, displayPath string) (string, error) {
	file, _, err := openDriveSyncRegularFile(directory, name, displayPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	checksum, err := driveSyncOpenFileChecksum(file)
	if err != nil {
		return "", fmt.Errorf("checksum %q: %w", displayPath, err)
	}
	return checksum, nil
}

func openDriveSyncRegularFile(directory *os.Root, name, displayPath string) (*os.File, os.FileInfo, error) {
	before, err := directory.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("local symlink not supported: %q", displayPath)
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("local path is not a regular file: %q", displayPath)
	}

	file, err := directory.Open(name)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	after, err := directory.Lstat(name)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("local file changed while opening: %q", displayPath)
	}
	return file, opened, nil
}

func driveSyncOpenFileChecksum(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hasher := md5.New() // #nosec G401 -- Google Drive's binary checksum contract is MD5.
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func buildDriveSyncPlan(ctx context.Context, tree driveSyncLocalTree, parentID string, allDrives bool, client driveSyncClient) (*driveSyncPlan, error) {
	parent, err := client.Get(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("inspect Drive parent %q: %w", parentID, err)
	}
	if parent.MimeType != driveMimeFolder {
		return nil, fmt.Errorf("drive parent %q is not a folder (mimeType=%s)", parentID, parent.MimeType)
	}
	if !allDrives && parent.DriveId != "" {
		return nil, fmt.Errorf("drive parent %q belongs to shared drive %q; remove --no-all-drives", parentID, parent.DriveId)
	}

	planner := driveSyncPlanner{
		client:    client,
		tree:      tree,
		folderIDs: map[string]string{"": parentID},
	}
	if err := planner.planDirectory(ctx, "", parentID, true); err != nil {
		return nil, err
	}
	sort.Slice(planner.directories, func(i, j int) bool {
		leftDepth := strings.Count(planner.directories[i].Path, "/")
		rightDepth := strings.Count(planner.directories[j].Path, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return planner.directories[i].Path < planner.directories[j].Path
	})
	sort.Slice(planner.files, func(i, j int) bool {
		return planner.files[i].Path < planner.files[j].Path
	})

	actions := append([]driveSyncAction(nil), planner.directories...)
	actions = append(actions, planner.files...)
	for index := range actions {
		actions[index].ParentID = planner.folderIDs[actions[index].ParentPath]
	}
	return &driveSyncPlan{
		Actions:   actions,
		folderIDs: planner.folderIDs,
		entries:   tree.Entries,
		localRoot: tree.root,
		dirInfos:  tree.dirInfos,
	}, nil
}

type driveSyncPlanner struct {
	client      driveSyncClient
	tree        driveSyncLocalTree
	folderIDs   map[string]string
	directories []driveSyncAction
	files       []driveSyncAction
}

func (p *driveSyncPlanner) planDirectory(ctx context.Context, localParent, remoteParentID string, remoteExists bool) error {
	remoteByName := make(map[string][]*drive.File)
	if remoteExists {
		children, err := p.client.Children(ctx, remoteParentID)
		if err != nil {
			return fmt.Errorf("list Drive folder for %q: %w", localParent, err)
		}
		for _, child := range children {
			if child != nil {
				remoteByName[child.Name] = append(remoteByName[child.Name], child)
			}
		}
	}

	for _, local := range p.tree.Children[localParent] {
		matches := remoteByName[local.Name]
		if len(matches) > 1 {
			return fmt.Errorf("ambiguous Drive sibling for local path %q: found %d matches", local.Path, len(matches))
		}
		var remote *drive.File
		if len(matches) == 1 {
			remote = matches[0]
		}
		if local.IsDir {
			if err := p.planLocalDirectory(ctx, local, remote); err != nil {
				return err
			}
			continue
		}
		if err := p.planLocalFile(local, remote); err != nil {
			return err
		}
	}
	return nil
}

func (p *driveSyncPlanner) planLocalDirectory(ctx context.Context, local driveSyncLocalEntry, remote *drive.File) error {
	if remote == nil {
		p.directories = append(p.directories, driveSyncAction{
			Action:     driveSyncCreateFolder,
			Path:       local.Path,
			MimeType:   driveMimeFolder,
			Reason:     "missing",
			ParentPath: local.ParentPath,
		})
		return p.planDirectory(ctx, local.Path, "", false)
	}
	if remote.MimeType != driveMimeFolder {
		return driveSyncTypeConflict(local, remote)
	}
	p.folderIDs[local.Path] = remote.Id
	return p.planDirectory(ctx, local.Path, remote.Id, true)
}

func (p *driveSyncPlanner) planLocalFile(local driveSyncLocalEntry, remote *drive.File) error {
	action := driveSyncAction{
		Path:       local.Path,
		MimeType:   local.MimeType,
		MD5:        local.MD5,
		Size:       local.Size,
		ParentPath: local.ParentPath,
	}
	if remote == nil {
		action.Action = driveSyncCreateFile
		action.Reason = "missing"
		p.files = append(p.files, action)
		return nil
	}
	if remote.MimeType == driveMimeFolder || strings.HasPrefix(remote.MimeType, "application/vnd.google-apps.") {
		return driveSyncTypeConflict(local, remote)
	}
	action.FileID = remote.Id
	if remote.Md5Checksum != "" && remote.Size == local.Size && strings.EqualFold(remote.Md5Checksum, local.MD5) {
		action.Action = driveSyncSkipFile
		action.Reason = "size and md5 match"
		p.files = append(p.files, action)
		return nil
	}
	action.Action = driveSyncUpdateFile
	switch {
	case remote.Md5Checksum == "":
		action.Reason = "remote md5 unavailable"
	case remote.Size != local.Size:
		action.Reason = "size differs"
	default:
		action.Reason = "md5 differs"
	}
	p.files = append(p.files, action)
	return nil
}

func driveSyncTypeConflict(local driveSyncLocalEntry, remote *drive.File) error {
	want := "binary file"
	if local.IsDir {
		want = strFolder
	}
	return fmt.Errorf("drive type conflict for local path %q: need %s, found mimeType=%s", local.Path, want, remote.MimeType)
}

func applyDriveSyncPlan(ctx context.Context, plan *driveSyncPlan, client driveSyncClient) error {
	var stagingDir string
	defer func() {
		if stagingDir != "" {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	for index := range plan.Actions {
		action := &plan.Actions[index]
		switch action.Action {
		case driveSyncCreateFolder:
			parentID, ok := plan.folderIDs[action.ParentPath]
			if !ok || parentID == "" {
				return fmt.Errorf("missing resolved Drive parent for %q", action.Path)
			}
			created, err := client.CreateFolder(ctx, path.Base(action.Path), parentID)
			if err != nil {
				return fmt.Errorf("create Drive folder %q: %w", action.Path, err)
			}
			if created == nil || created.Id == "" {
				return fmt.Errorf("create Drive folder %q: response missing file ID", action.Path)
			}
			action.FileID = created.Id
			action.ParentID = parentID
			plan.folderIDs[action.Path] = created.Id
		case driveSyncCreateFile, driveSyncUpdateFile:
			if stagingDir == "" {
				createdDir, mkdirErr := os.MkdirTemp("", "gog-drive-sync-")
				if mkdirErr != nil {
					return fmt.Errorf("create Drive sync staging directory: %w", mkdirErr)
				}
				stagingDir = createdDir
			}
			if err := applyDriveSyncFile(ctx, action, plan, client, stagingDir); err != nil {
				return err
			}
		case driveSyncSkipFile:
		default:
			return fmt.Errorf("unsupported Drive sync action %q", action.Action)
		}
	}
	return nil
}

func applyDriveSyncFile(ctx context.Context, action *driveSyncAction, plan *driveSyncPlan, client driveSyncClient, stagingDir string) error {
	entry, ok := plan.entries[action.Path]
	if !ok {
		return fmt.Errorf("missing local entry for %q", action.Path)
	}
	snapshot, cleanup, err := stageDriveSyncFile(plan, entry, action.Path, stagingDir)
	if err != nil {
		return err
	}
	defer cleanup()

	if action.Action == driveSyncCreateFile {
		parentID, found := plan.folderIDs[action.ParentPath]
		if !found || parentID == "" {
			return fmt.Errorf("missing resolved Drive parent for %q", action.Path)
		}
		created, createErr := client.CreateFile(ctx, entry, parentID, snapshot)
		if createErr != nil {
			return fmt.Errorf("create Drive file %q: %w", action.Path, createErr)
		}
		if created == nil || created.Id == "" {
			return fmt.Errorf("create Drive file %q: response missing file ID", action.Path)
		}
		if verifyErr := verifyDriveSyncUpload(entry, created); verifyErr != nil {
			return fmt.Errorf("verify created Drive file %q (id=%s): %w", action.Path, created.Id, verifyErr)
		}
		action.FileID = created.Id
		action.ParentID = parentID
		return nil
	}

	updated, err := client.UpdateFile(ctx, action.FileID, entry, snapshot)
	if err != nil {
		return fmt.Errorf("update Drive file %q: %w", action.Path, err)
	}
	if updated == nil || updated.Id != action.FileID {
		return fmt.Errorf("update Drive file %q: response did not preserve file ID", action.Path)
	}
	if verifyErr := verifyDriveSyncUpload(entry, updated); verifyErr != nil {
		return fmt.Errorf("verify updated Drive file %q (id=%s): %w", action.Path, updated.Id, verifyErr)
	}
	return nil
}

func stageDriveSyncFile(plan *driveSyncPlan, entry driveSyncLocalEntry, displayPath, stagingDir string) (*os.File, func(), error) {
	localParent, err := openDriveSyncDirectory(plan.localRoot, plan.dirInfos, entry.ParentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reopen local parent for %q: %w", displayPath, err)
	}
	file, info, err := openDriveSyncRegularFile(localParent, entry.Name, displayPath)
	closeParentErr := localParent.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("open local file %q: %w", displayPath, err)
	}
	if closeParentErr != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("close local parent for %q: %w", displayPath, closeParentErr)
	}
	if info.Size() != entry.Size {
		_ = file.Close()
		return nil, nil, fmt.Errorf("local file changed after preflight: %q", displayPath)
	}

	snapshot, err := os.CreateTemp(stagingDir, "content-")
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("create local snapshot for %q: %w", displayPath, err)
	}
	cleanup := func() {
		_ = snapshot.Close()
		_ = os.Remove(snapshot.Name())
	}
	hasher := md5.New() // #nosec G401 -- Google Drive's binary checksum contract is MD5.
	written, copyErr := io.Copy(io.MultiWriter(snapshot, hasher), file)
	closeSourceErr := file.Close()
	if copyErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("snapshot local file %q: %w", displayPath, copyErr)
	}
	if closeSourceErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close local file %q: %w", displayPath, closeSourceErr)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	if written != entry.Size || !strings.EqualFold(checksum, entry.MD5) {
		cleanup()
		return nil, nil, fmt.Errorf("local file changed while snapshotting: %q", displayPath)
	}
	if syncErr := snapshot.Sync(); syncErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("sync local snapshot for %q: %w", displayPath, syncErr)
	}
	if _, seekErr := snapshot.Seek(0, io.SeekStart); seekErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("rewind local snapshot for %q: %w", displayPath, seekErr)
	}
	return snapshot, cleanup, nil
}

func verifyDriveSyncUpload(entry driveSyncLocalEntry, uploaded *drive.File) error {
	if uploaded.Size != entry.Size {
		return fmt.Errorf("provider size=%d, expected %d", uploaded.Size, entry.Size)
	}
	if uploaded.Md5Checksum == "" {
		return fmt.Errorf("provider response missing md5Checksum")
	}
	if !strings.EqualFold(uploaded.Md5Checksum, entry.MD5) {
		return fmt.Errorf("provider md5=%s, expected %s", uploaded.Md5Checksum, entry.MD5)
	}
	return nil
}

func (c *googleDriveSyncClient) Get(ctx context.Context, fileID string) (*drive.File, error) {
	file, err := c.service.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields(gapi.Field(driveSyncFields)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	c.driveID = file.DriveId
	return file, nil
}

func (c *googleDriveSyncClient) Children(ctx context.Context, parentID string) ([]*drive.File, error) {
	return listDriveSyncChildren(ctx, c.service, parentID, c.driveID)
}

func listDriveSyncChildren(ctx context.Context, svc *drive.Service, parentID, driveID string) ([]*drive.File, error) {
	query := buildDriveListQuery(parentID, "")
	files := make([]*drive.File, 0, 64)
	var pageToken string
	for {
		call := svc.Files.List().
			Q(query).
			PageSize(driveDefaultPageSize).
			PageToken(pageToken).
			OrderBy("folder,name")
		if driveID != "" {
			call = driveFilesListCallWithDriveSupport(call, true, driveID)
		} else {
			call = call.SupportsAllDrives(true).
				IncludeItemsFromAllDrives(false).
				Corpora("user")
		}
		response, err := call.Fields(
			gapi.Field("nextPageToken"),
			gapi.Field("incompleteSearch"),
			gapi.Field("files("+driveSyncFields+")"),
		).Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		if response.IncompleteSearch {
			return nil, fmt.Errorf("drive returned an incomplete child listing for parent %q", parentID)
		}
		files = append(files, response.Files...)
		if response.NextPageToken == "" {
			return files, nil
		}
		pageToken = response.NextPageToken
	}
}

func (c *googleDriveSyncClient) CreateFolder(ctx context.Context, name, parentID string) (*drive.File, error) {
	return c.service.Files.Create(&drive.File{
		Name:     name,
		MimeType: driveMimeFolder,
		Parents:  []string{parentID},
	}).
		SupportsAllDrives(true).
		Fields(gapi.Field(driveSyncFields)).
		Context(ctx).
		Do()
}

func (c *googleDriveSyncClient) CreateFile(ctx context.Context, entry driveSyncLocalEntry, parentID string, content io.Reader) (*drive.File, error) {
	return c.service.Files.Create(&drive.File{
		Name:    entry.Name,
		Parents: []string{parentID},
	}).
		SupportsAllDrives(true).
		Media(content, gapi.ContentType(entry.MimeType)).
		Fields(gapi.Field(driveSyncFields)).
		Context(ctx).
		Do()
}

func (c *googleDriveSyncClient) UpdateFile(ctx context.Context, fileID string, entry driveSyncLocalEntry, content io.Reader) (*drive.File, error) {
	return c.service.Files.Update(fileID, &drive.File{}).
		SupportsAllDrives(true).
		Media(content, gapi.ContentType(entry.MimeType)).
		Fields(gapi.Field(driveSyncFields)).
		Context(ctx).
		Do()
}

func writeDriveSyncPlan(ctx context.Context, localDir, parentID string, dryRun bool, actions []driveSyncAction) error {
	summary := summarizeDriveSyncActions(actions)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"actions":   actions,
			"dry_run":   dryRun,
			"local_dir": localDir,
			"parent_id": parentID,
			"summary":   summary,
		})
	}

	rows := append([]driveSyncAction(nil), actions...)
	rows = append(rows, driveSyncAction{
		Action: "summary",
		Reason: fmt.Sprintf(
			"create_folders=%d create_files=%d update_files=%d skip_files=%d",
			summary.CreateFolders,
			summary.CreateFiles,
			summary.UpdateFiles,
			summary.SkipFiles,
		),
	})
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), rows, []outfmt.Column[driveSyncAction]{
		{Header: "ACTION", Value: func(row driveSyncAction) string { return driveSyncOutputField(row.Action) }},
		{Header: "PATH", Value: func(row driveSyncAction) string { return driveSyncOutputField(row.Path) }},
		{Header: "ID", Value: func(row driveSyncAction) string { return driveSyncOutputField(row.FileID) }},
		{Header: "REASON", Value: func(row driveSyncAction) string { return driveSyncOutputField(row.Reason) }},
	})
}

func summarizeDriveSyncActions(actions []driveSyncAction) driveSyncSummary {
	var summary driveSyncSummary
	for _, action := range actions {
		switch action.Action {
		case driveSyncCreateFolder:
			summary.CreateFolders++
		case driveSyncCreateFile:
			summary.CreateFiles++
		case driveSyncUpdateFile:
			summary.UpdateFiles++
		case driveSyncSkipFile:
			summary.SkipFiles++
		}
	}
	return summary
}

func driveSyncOutputField(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	for _, current := range value {
		if unicode.IsControl(current) || unicode.Is(unicode.Cf, current) {
			switch {
			case current <= 0xff:
				fmt.Fprintf(&output, "\\x%02x", current)
			case current <= 0xffff:
				fmt.Fprintf(&output, "\\u%04x", current)
			default:
				fmt.Fprintf(&output, "\\U%08x", current)
			}
			continue
		}
		output.WriteRune(current)
	}
	return output.String()
}
