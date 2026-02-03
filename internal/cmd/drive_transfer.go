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

type DriveTransferCmd struct {
	From              string `name:"from" help:"Current owner email" required:""`
	To                string `name:"to" help:"New owner email" required:""`
	RetainPermissions bool   `name:"retain-permissions" help:"Keep existing owner permission"`
	Max               int64  `name:"max" aliases:"limit" default:"100" help:"Max files per page"`
}

func (c *DriveTransferCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	from := strings.TrimSpace(c.From)
	to := strings.TrimSpace(c.To)
	if from == "" || to == "" {
		return usage("--from and --to are required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("transfer Drive ownership from %s to %s", from, to)); err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	moved := 0
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(fmt.Sprintf("'%s' in owners and trashed=false", from)).
			Fields("nextPageToken, files(id, name, owners(emailAddress), permissions(id,emailAddress))").
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true)
		if c.Max > 0 {
			call = call.PageSize(c.Max)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("list files: %w", err)
		}

		for _, f := range resp.Files {
			if f == nil {
				continue
			}
			perm := &drive.Permission{
				Type:         "user",
				Role:         "owner",
				EmailAddress: to,
			}
			if _, err := svc.Permissions.Create(f.Id, perm).
				TransferOwnership(true).
				SendNotificationEmail(false).
				SupportsAllDrives(true).
				UseDomainAdminAccess(true).
				Context(ctx).
				Do(); err != nil {
				return fmt.Errorf("transfer %s: %w", f.Id, err)
			}

			if !c.RetainPermissions {
				if err := removeDrivePermission(ctx, svc, f.Id, from); err != nil {
					return err
				}
			}
			moved++
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"transferred": moved, "from": from, "to": to})
	}

	u.Out().Printf("Transferred %d files from %s to %s\n", moved, from, to)
	return nil
}

func removeDrivePermission(ctx context.Context, svc *drive.Service, fileID string, email string) error {
	perms, err := svc.Permissions.List(fileID).
		Fields("permissions(id,emailAddress)").
		SupportsAllDrives(true).
		UseDomainAdminAccess(true).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("list permissions for %s: %w", fileID, err)
	}

	for _, perm := range perms.Permissions {
		if perm == nil {
			continue
		}
		if strings.EqualFold(perm.EmailAddress, email) {
			if err := svc.Permissions.Delete(fileID, perm.Id).
				SupportsAllDrives(true).
				UseDomainAdminAccess(true).
				Context(ctx).
				Do(); err != nil {
				return fmt.Errorf("remove permission %s: %w", perm.Id, err)
			}
			break
		}
	}

	return nil
}
