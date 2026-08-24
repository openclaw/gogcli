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
		name := sanitizeAttachmentFilename(appScriptFileName(file), "")
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

// appScriptFileName renders the editor-visible filename for a project file.
func appScriptFileName(file *scriptapi.File) string {
	return file.Name + appScriptExtByType[file.Type]
}
