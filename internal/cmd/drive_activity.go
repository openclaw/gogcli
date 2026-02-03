package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/driveactivity/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDriveActivityService = googleapi.NewDriveActivity

type DriveActivityCmd struct {
	FileID string `arg:"" name:"file-id" help:"Drive file ID"`
	Max    int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page   string `name:"page" help:"Page token"`
}

func (c *DriveActivityCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("file-id is required")
	}

	svc, err := newDriveActivityService(ctx, account)
	if err != nil {
		return err
	}

	req := &driveactivity.QueryDriveActivityRequest{
		ItemName: "items/" + fileID,
		PageSize: c.Max,
		PageToken: func() string {
			return c.Page
		}(),
	}
	resp, err := svc.Activity.Query(req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("query activity: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Activities) == 0 {
		u.Err().Println("No activity found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TIME\tACTOR\tACTION")
	for _, act := range resp.Activities {
		if act == nil {
			continue
		}
		time := act.Timestamp
		if time == "" && act.TimeRange != nil {
			time = act.TimeRange.EndTime
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(time),
			sanitizeTab(activityActor(act.Actors)),
			sanitizeTab(activityAction(act.PrimaryActionDetail)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func activityActor(actors []*driveactivity.Actor) string {
	if len(actors) == 0 || actors[0] == nil {
		return ""
	}
	actor := actors[0]
	if actor.User != nil {
		if actor.User.KnownUser != nil && actor.User.KnownUser.PersonName != "" {
			return actor.User.KnownUser.PersonName
		}
		if actor.User.KnownUser != nil && actor.User.KnownUser.IsCurrentUser {
			return "me"
		}
	}
	if actor.Administrator != nil {
		return "admin"
	}
	if actor.System != nil {
		return "system"
	}
	if actor.Anonymous != nil {
		return "anonymous"
	}
	return "unknown"
}

func activityAction(detail *driveactivity.ActionDetail) string {
	if detail == nil {
		return ""
	}
	switch {
	case detail.Create != nil:
		return "create"
	case detail.Edit != nil:
		return "edit"
	case detail.Move != nil:
		return "move"
	case detail.Rename != nil:
		return "rename"
	case detail.Delete != nil:
		return "delete"
	case detail.Restore != nil:
		return "restore"
	case detail.PermissionChange != nil:
		return "permission"
	case detail.Comment != nil:
		return "comment"
	case detail.DlpChange != nil:
		return "dlp"
	case detail.SettingsChange != nil:
		return "settings"
	case detail.Reference != nil:
		return "reference"
	default:
		return "other"
	}
}
