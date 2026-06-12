package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DriveShortcutCmd struct {
	Create DriveShortcutCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a shortcut to a Drive file or folder"`
}

type DriveShortcutCreateCmd struct {
	TargetID string `arg:"" name:"targetId" help:"Target file or folder ID"`
	Parent   string `name:"parent" help:"Destination folder ID (required)"`
	Name     string `name:"name" help:"Shortcut name (default: target name)"`
}

func (c *DriveShortcutCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	targetID := strings.TrimSpace(c.TargetID)
	if targetID == "" {
		return usage("empty targetId")
	}
	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("missing --parent")
	}
	name := strings.TrimSpace(c.Name)

	if err := dryRunExit(ctx, flags, "drive.shortcut.create", map[string]any{
		"targetId": targetID,
		"parent":   parent,
		"name":     name,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if name == "" {
		target, getErr := svc.Files.Get(targetID).
			SupportsAllDrives(true).
			Fields("id,name").
			Context(ctx).
			Do()
		if getErr != nil {
			return getErr
		}
		name = target.Name
		if strings.TrimSpace(name) == "" {
			return usage("target file has no name; specify --name")
		}
	}

	created, err := svc.Files.Create(&drive.File{
		Name:     name,
		MimeType: driveMimeShortcut,
		Parents:  []string{parent},
		ShortcutDetails: &drive.FileShortcutDetails{
			TargetId: targetID,
		},
	}).
		SupportsAllDrives(true).
		Fields("id,name,mimeType,parents,webViewLink,shortcutDetails(targetId,targetMimeType,targetResourceKey)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"shortcut": created})
	}

	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("name\t%s", created.Name)
	u.Out().Linef("target_id\t%s", driveShortcutTargetID(created))
	if len(created.Parents) > 0 {
		u.Out().Linef("parent\t%s", created.Parents[0])
	}
	if created.WebViewLink != "" {
		u.Out().Linef("link\t%s", created.WebViewLink)
	}
	return nil
}
