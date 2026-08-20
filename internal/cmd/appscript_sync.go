package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/config"
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

var appScriptExtByType = map[string]string{
	"SERVER_JS": ".gs",
	"HTML":      ".html",
	"JSON":      ".json",
}

// appScriptManifest is the only JSON file a project may hold, and it is
// mandatory: UpdateContent rejects a payload without appsscript.json.
const appScriptManifest = "appsscript"

type AppScriptPushCmd struct {
	ScriptID string `arg:"" name:"scriptId" help:"Script ID"`
	Dir      string `arg:"" name:"dir" help:"Local directory holding .gs/.js/.html files and appsscript.json" type:"path"`
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

	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Name+appScriptExtByType[file.Type])
	}

	// UpdateContent replaces the project wholesale, so any remote file absent
	// from dir is dropped. The dry-run payload lists exactly what would survive.
	if dryRunErr := dryRunExit(ctx, flags, "appscript.push", map[string]any{
		"script_id": scriptID,
		"dir":       dir,
		"files":     names,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	content, err := svc.Projects.UpdateContent(scriptID, &scriptapi.Content{
		Files: files,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"pushed":     true,
			"content":    content,
			"editor_url": appScriptEditURL(scriptID),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("pushed\ttrue")
	u.Out().Linef("script_id\t%s", scriptID)
	printAppScriptFiles(u, content.Files)
	u.Out().Linef("editor_url\t%s", appScriptEditURL(scriptID))

	return nil
}

type AppScriptPullCmd struct {
	ScriptID string `arg:"" name:"scriptId" help:"Script ID"`
	Dir      string `arg:"" name:"dir" help:"Local directory to write files into (created if missing)" type:"path"`
}

func (c *AppScriptPullCmd) Run(ctx context.Context, flags *RootFlags) error {
	scriptID := strings.TrimSpace(normalizeGoogleID(c.ScriptID))
	if scriptID == "" {
		return usage("empty scriptId")
	}

	dir, err := expandAppScriptDir(c.Dir)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "appscript.pull", map[string]any{
		"script_id": scriptID,
		"dir":       dir,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireAppScriptService(ctx, flags)
	if err != nil {
		return err
	}

	content, err := svc.Projects.GetContent(scriptID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	written := make([]string, 0, len(content.Files))

	for _, file := range content.Files {
		if file == nil {
			continue
		}

		// File names come from the API; keep them from escaping dir.
		name := sanitizeAttachmentFilename(file.Name+appScriptExtByType[file.Type], "")
		if name == "" {
			continue
		}

		if err := writeFileAtomic(filepath.Join(dir, name), []byte(file.Source)); err != nil {
			return err
		}

		written = append(written, name)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"pulled": true,
			"dir":    dir,
			"files":  written,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("pulled\ttrue")
	u.Out().Linef("dir\t%s", dir)

	for _, name := range written {
		u.Out().Linef("file\t%s", name)
	}

	return nil
}

func expandAppScriptDir(dir string) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", usage("empty dir")
	}

	return config.ExpandPath(trimmed)
}

// readAppScriptDir turns a local directory into the file list the API wants.
// os.ReadDir yields entries sorted by name, so the pushed order is already
// stable and needs no extra sort.
func readAppScriptDir(dir string) ([]*scriptapi.File, error) {
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

		// Decide before reading, so a stray package-lock.json is never loaded.
		base := strings.TrimSuffix(name, ext)
		if fileType == "JSON" {
			if base != appScriptManifest {
				continue
			}

			hasManifest = true
		}

		source, readErr := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- path is confined to the caller-selected push directory.
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
