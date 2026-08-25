package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// Apps Script stores a file's extension in its type, not its name: a file named
// "Code" with type SERVER_JS is Code.gs in the editor.
var appScriptExtByType = map[string]string{
	"SERVER_JS": ".gs",
	"HTML":      ".html",
	"JSON":      ".json",
}

type AppScriptPullCmd struct {
	ScriptID  string `arg:"" name:"scriptId" help:"Script ID"`
	Dir       string `arg:"" name:"dir" help:"Local directory to write files into (created if missing)" type:"path"`
	Overwrite bool   `name:"overwrite" help:"Overwrite files that already exist in dir"`
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
		"overwrite": c.Overwrite,
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

	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return mkdirErr
	}
	root, _, _, err := openDriveSyncRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	targets := make([]*scriptapi.File, 0, len(content.Files))
	written := make([]string, 0, len(content.Files))
	seenNames := make(map[string]struct{}, len(content.Files))
	var clashes []string

	for _, file := range content.Files {
		if file == nil {
			continue
		}

		// File names come from the API; contain traversal and reject collisions.
		name := sanitizeAttachmentFilename(appScriptFileName(file), "")
		if name == "" {
			continue
		}
		nameKey := strings.ToLower(name)
		if _, exists := seenNames[nameKey]; exists {
			return usagef("multiple Apps Script files resolve to local path %q", name)
		}
		seenNames[nameKey] = struct{}{}

		info, statErr := root.Lstat(name)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("refusing to overwrite non-regular output file %q", filepath.Join(dir, name))
			}
			if !c.Overwrite {
				clashes = append(clashes, name)
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}

		targets = append(targets, file)
		written = append(written, name)
	}

	// Validate every target before writing so conflicts never leave partial pulls.
	if len(clashes) > 0 {
		return usagef("%s already contains %d of these file(s): %s (pass --overwrite to replace them)",
			dir, len(clashes), strings.Join(clashes, ", "))
	}

	for i, file := range targets {
		if err := writeAppScriptFile(root, written[i], file.Source, c.Overwrite); err != nil {
			return err
		}
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

func writeAppScriptFile(root *os.Root, name, source string, overwrite bool) error {
	outputName := name
	if overwrite {
		outputName = ".gog-appscript-" + rand.Text()
		defer func() { _ = root.Remove(outputName) }()
	}

	// Keep replacement rooted even if its original directory is swapped.
	f, err := root.OpenFile(outputName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	if _, writeErr := f.WriteString(source); writeErr != nil {
		_ = f.Close()

		return writeErr
	}

	if err := f.Close(); err != nil {
		return err
	}
	if overwrite {
		return root.Rename(outputName, name)
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

// appScriptFileName renders the editor-visible filename for a project file.
func appScriptFileName(file *scriptapi.File) string {
	return file.Name + appScriptExtByType[file.Type]
}
