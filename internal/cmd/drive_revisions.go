package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DriveRevisionsCmd struct {
	List   DriveRevisionsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List file revisions"`
	Get    DriveRevisionsGetCmd    `cmd:"" name:"get" help:"Get a file revision"`
	Delete DriveRevisionsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a file revision"`
}

type DriveRevisionsListCmd struct {
	FileID string `arg:"" name:"file-id" help:"File ID"`
}

func (c *DriveRevisionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("file-id is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Revisions.List(fileID).
		Fields("revisions(id, modifiedTime, keepForever, mimeType, lastModifyingUser(emailAddress, displayName))").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("list revisions: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Revisions) == 0 {
		u.Err().Println("No revisions found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tMODIFIED\tKEEP\tMIME\tUSER")
	for _, rev := range resp.Revisions {
		if rev == nil {
			continue
		}
		user := ""
		if rev.LastModifyingUser != nil {
			if rev.LastModifyingUser.DisplayName != "" {
				user = rev.LastModifyingUser.DisplayName
			} else {
				user = rev.LastModifyingUser.EmailAddress
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\t%s\n",
			sanitizeTab(rev.Id),
			sanitizeTab(rev.ModifiedTime),
			rev.KeepForever,
			sanitizeTab(rev.MimeType),
			sanitizeTab(user),
		)
	}
	return nil
}

type DriveRevisionsGetCmd struct {
	FileID     string `arg:"" name:"file-id" help:"File ID"`
	RevisionID string `arg:"" name:"revision-id" help:"Revision ID"`
}

func (c *DriveRevisionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := strings.TrimSpace(c.FileID)
	revID := strings.TrimSpace(c.RevisionID)
	if fileID == "" || revID == "" {
		return usage("file-id and revision-id are required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	rev, err := svc.Revisions.Get(fileID, revID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get revision: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, rev)
	}

	fmt.Fprintf(os.Stdout, "ID: %s\n", rev.Id)
	fmt.Fprintf(os.Stdout, "Modified: %s\n", rev.ModifiedTime)
	fmt.Fprintf(os.Stdout, "Mime: %s\n", rev.MimeType)
	fmt.Fprintf(os.Stdout, "Keep: %t\n", rev.KeepForever)
	if rev.LastModifyingUser != nil {
		fmt.Fprintf(os.Stdout, "User: %s\n", rev.LastModifyingUser.EmailAddress)
	}
	return nil
}

type DriveRevisionsDeleteCmd struct {
	FileID     string `arg:"" name:"file-id" help:"File ID"`
	RevisionID string `arg:"" name:"revision-id" help:"Revision ID"`
}

func (c *DriveRevisionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := strings.TrimSpace(c.FileID)
	revID := strings.TrimSpace(c.RevisionID)
	if fileID == "" || revID == "" {
		return usage("file-id and revision-id are required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete revision %s for file %s", revID, fileID)); err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Revisions.Delete(fileID, revID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete revision: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"fileId": fileID, "revisionId": revID, "deleted": true})
	}

	u.Out().Printf("Deleted revision %s for %s\n", revID, fileID)
	return nil
}
