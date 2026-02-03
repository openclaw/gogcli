package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const driveMimeShortcut = "application/vnd.google-apps.shortcut"

type DriveShortcutsCmd struct {
	Create DriveShortcutsCreateCmd `cmd:"" name:"create" help:"Create a shortcut"`
	Delete DriveShortcutsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a shortcut"`
}

type DriveShortcutsCreateCmd struct {
	Target string `name:"target" help:"Target file ID" required:""`
	Parent string `name:"parent" help:"Parent folder ID"`
	Name   string `name:"name" help:"Shortcut name"`
}

func (c *DriveShortcutsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	target := strings.TrimSpace(c.Target)
	if target == "" {
		return usage("--target is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	file := &drive.File{
		MimeType: driveMimeShortcut,
		Name:     strings.TrimSpace(c.Name),
		ShortcutDetails: &drive.FileShortcutDetails{
			TargetId: target,
		},
	}
	if strings.TrimSpace(c.Parent) != "" {
		file.Parents = []string{strings.TrimSpace(c.Parent)}
	}

	created, err := svc.Files.Create(file).
		SupportsAllDrives(true).
		Fields("id, name, shortcutDetails").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("create shortcut: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created shortcut: %s (%s)\n", created.Name, created.Id)
	return nil
}

type DriveShortcutsDeleteCmd struct {
	ShortcutID string `arg:"" name:"shortcut-id" help:"Shortcut file ID"`
}

func (c *DriveShortcutsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	shortcutID := strings.TrimSpace(c.ShortcutID)
	if shortcutID == "" {
		return usage("shortcut-id is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete shortcut %s", shortcutID)); err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Files.Delete(shortcutID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete shortcut: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"shortcutId": shortcutID, "deleted": true})
	}

	u.Out().Printf("Deleted shortcut: %s\n", shortcutID)
	return nil
}
