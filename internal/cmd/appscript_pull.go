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

	// File names come from the API; keep them from escaping dir.
	targets := make([]*scriptapi.File, 0, len(content.Files))
	written := make([]string, 0, len(content.Files))

	for _, file := range content.Files {
		if file == nil {
			continue
		}

		if name := sanitizeAttachmentFilename(appScriptFileName(file), ""); name != "" {
			targets = append(targets, file)
			written = append(written, name)
		}
	}

	// Name every file that would be replaced before replacing any of them, so a
	// pull into a working directory cannot quietly discard local edits.
	if !c.Overwrite {
		if clashes := existingAppScriptTargets(dir, written); len(clashes) > 0 {
			return usagef("%s already contains %d of these file(s): %s (pass --overwrite to replace them)",
				dir, len(clashes), strings.Join(clashes, ", "))
		}
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	for i, file := range targets {
		if err := writeAppScriptFile(filepath.Join(dir, written[i]), file.Source, c.Overwrite); err != nil {
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

// existingAppScriptTargets reports which of names already exist in dir.
func existingAppScriptTargets(dir string, names []string) []string {
	var clashes []string

	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			clashes = append(clashes, name)
		}
	}

	return clashes
}

// writeAppScriptFile writes one pulled file through the shared output helper,
// whose default is O_EXCL -- so the overwrite decision is enforced at open time
// rather than trusted from the pre-flight check above.
func writeAppScriptFile(path, source string, overwrite bool) error {
	f, _, err := openUserOutputFile(path, outputFileOptions{
		Overwrite: overwrite,
		FileMode:  0o600,
		DirMode:   0o700,
	})
	if err != nil {
		return err
	}

	if _, writeErr := f.WriteString(source); writeErr != nil {
		_ = f.Close()

		return writeErr
	}

	return f.Close()
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
