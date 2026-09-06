package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// Apps Script stores a file's extension in its type, not its name: a file named
// "Code" with type SERVER_JS is Code.gs in the editor. These two maps are the
// translation in both directions.
var appScriptTypeByExt = map[string]string{
	".gs":   "SERVER_JS",
	".js":   "SERVER_JS",
	".html": "HTML",
	".json": "JSON",
}

// appScriptManifest is the only JSON file a project may hold, and it is
// mandatory: UpdateContent rejects a payload without appsscript.json.
const appScriptManifest = "appsscript"

type AppScriptPushCmd struct {
	ScriptID string `arg:"" name:"scriptId" help:"Script ID"`
	Dir      string `arg:"" name:"dir" help:"Local directory holding .gs/.js/.html files and appsscript.json" type:"path"`
	Prune    bool   `name:"prune" aliases:"delete" help:"Delete remote files that dir does not provide (default: keep them)"`
}

func (c *AppScriptPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("empty scriptId")
	}

	dir, err := expandAppScriptDir(c.Dir)
	if err != nil {
		return err
	}

	files, err := readAppScriptDir(dir)
	if err != nil {
		return err
	}

	localNames := appScriptFileNames(files)

	// Without --prune nothing can be deleted, so the request is the whole
	// preview and the dry run stays auth-free. --prune has to read the project
	// before it can say what it would remove, so its dry run runs after the
	// fetch below -- same shape as `drive sync push`.
	if !c.Prune {
		if dryRunErr := dryRunExit(ctx, flags, "appscript.push", map[string]any{
			"script_id": scriptID,
			"dir":       dir,
			"files":     localNames,
			"prune":     false,
		}); dryRunErr != nil {
			return dryRunErr
		}
	}

	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	// UpdateContent replaces the project wholesale: whatever the payload leaves
	// out is deleted. Read the current content first so the default can carry
	// remote-only files through, and so --prune can name what it removes.
	current, err := appScriptCurrentContent(ctx, svc, scriptID, c.Prune)
	if err != nil {
		return err
	}

	remoteOnly := appScriptRemoteOnlyFiles(files, current.Files)
	remoteOnlyNames := appScriptFileNames(remoteOnly)

	payload := files

	var removed, kept []string

	if c.Prune {
		removed = remoteOnlyNames

		if dryRunErr := dryRunExit(ctx, flags, "appscript.push", map[string]any{
			"script_id": scriptID,
			"dir":       dir,
			"files":     localNames,
			"prune":     true,
			"removed":   nonNilStrings(removed),
		}); dryRunErr != nil {
			return dryRunErr
		}
	} else {
		kept = remoteOnlyNames
		payload = slices.Concat(files, remoteOnly)
	}

	if len(removed) > 0 {
		// Name every file the prune drops before making the call that drops it.
		printAppScriptPruneSet(ctx, dir, removed)

		action := fmt.Sprintf("delete %d remote file(s): %s", len(removed), strings.Join(removed, ", "))
		if confirmErr := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), action); confirmErr != nil {
			return confirmErr
		}
	}

	content, err := svc.Projects.UpdateContent(scriptID, &scriptapi.Content{
		Files: payload,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"pushed":     true,
			"pruned":     c.Prune,
			"removed":    nonNilStrings(removed),
			"kept":       nonNilStrings(kept),
			"content":    content,
			"editor_url": appScriptEditURL(scriptID),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("pushed\ttrue")
	u.Out().Linef("script_id\t%s", scriptID)
	printAppScriptFiles(u, content.Files)

	for _, name := range removed {
		u.Out().Linef("removed\t%s", sanitizeTab(name))
	}

	if len(kept) > 0 {
		u.Err().Linef("# Kept %d remote-only file(s) that %s does not provide: %s", len(kept), dir, strings.Join(kept, ", "))
		u.Err().Println("# Use --prune to delete them instead.")
	}

	u.Out().Linef("editor_url\t%s", appScriptEditURL(scriptID))

	return nil
}

// appScriptCurrentContent reads the project's current files. A prune only needs
// each remote file's identity, so it asks for a partial response instead of
// downloading every source; the merge path needs the sources it sends back.
func appScriptCurrentContent(
	ctx context.Context,
	svc *scriptapi.Service,
	scriptID string,
	identityOnly bool,
) (*scriptapi.Content, error) {
	call := svc.Projects.GetContent(scriptID)
	if identityOnly {
		call = call.Fields("files(name,type)")
	}

	return call.Context(ctx).Do()
}

// printAppScriptPruneSet reports the exact deletion set. Removing Apps Script
// content is not recoverable from the CLI, so an operator has to be able to see
// what is going before it goes.
func printAppScriptPruneSet(ctx context.Context, dir string, removed []string) {
	u := ui.FromContext(ctx)
	u.Err().Linef("# %d remote file(s) will be deleted (not present in %s):", len(removed), dir)

	for _, name := range removed {
		u.Err().Linef("#   %s", name)
	}
}

// appScriptRemoteOnlyFiles returns the remote files the local directory does not
// provide. Apps Script keys a file by name *and* type, so Code.gs and Code.html
// are two different files.
func appScriptRemoteOnlyFiles(local, remote []*scriptapi.File) []*scriptapi.File {
	localKeys := make(map[string]struct{}, len(local))
	for _, file := range local {
		localKeys[appScriptFileKey(file)] = struct{}{}
	}

	var remoteOnly []*scriptapi.File

	for _, file := range remote {
		if file == nil {
			continue
		}

		if _, ok := localKeys[appScriptFileKey(file)]; ok {
			continue
		}

		remoteOnly = append(remoteOnly, file)
	}

	return remoteOnly
}

func appScriptFileKey(file *scriptapi.File) string {
	return file.Name + "\x00" + file.Type
}

func appScriptFileNames(files []*scriptapi.File) []string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, appScriptFileName(file))
	}

	return names
}

// readAppScriptDir turns a local directory into the file list the API wants.
// Reads go through an os.Root so a link cannot pull in a file from outside dir,
// and os.ReadDir yields entries sorted by name, so the pushed order is stable
// without an extra sort.
func readAppScriptDir(dir string) ([]*scriptapi.File, error) {
	root, _, _, err := openDriveSyncRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var (
		files       []*scriptapi.File
		hasManifest bool
	)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)

		fileType, ok := appScriptTypeByExt[strings.ToLower(ext)]
		if !ok {
			continue
		}

		// DirEntry.Type comes from the ReadDir snapshot and is false for a
		// symlink's target, so re-check against the live filesystem: following a
		// link named Code.gs would upload a file from outside dir.
		info, statErr := root.Lstat(name)
		if statErr != nil {
			return nil, statErr
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return nil, usagef("%s is a symlink; push only uploads regular files inside %s", name, dir)
		}

		if !info.Mode().IsRegular() {
			return nil, usagef("%s is not a regular file; push only uploads regular files inside %s", name, dir)
		}

		// Decide before reading, so a stray package-lock.json is never loaded.
		base := strings.TrimSuffix(name, ext)
		if fileType == "JSON" {
			if base != appScriptManifest {
				continue
			}

			hasManifest = true
		}

		source, readErr := root.ReadFile(name)
		if readErr != nil {
			return nil, readErr
		}

		files = append(files, &scriptapi.File{
			Name:   base,
			Type:   fileType,
			Source: string(source),
		})
	}

	if len(files) == 0 {
		return nil, usagef("no .gs/.js/.html/appsscript.json files found in %s", dir)
	}

	if !hasManifest {
		return nil, usagef("%s must contain appsscript.json (the API rejects a push without a manifest)", dir)
	}

	return files, nil
}
