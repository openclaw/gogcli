package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/driveactivity/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const driveActivityActionCreate = "create"

type DriveActivityCmd struct {
	Query DriveActivityQueryCmd `cmd:"" name:"query" aliases:"list,ls" help:"Query Drive Activity API v2"`
}

type DriveActivityQueryCmd struct {
	File        string `name:"file" aliases:"file-id" help:"Drive file ID to query"`
	Folder      string `name:"folder" aliases:"folder-id" help:"Drive folder ID; includes descendants"`
	Actions     string `name:"actions" help:"Comma-separated action filters (edit,create,delete,move,rename,restore,comment,share,label,dlp,reference,settings)"`
	From        string `name:"from" help:"Lower activity time bound (RFC3339)"`
	To          string `name:"to" help:"Upper activity time bound (RFC3339)"`
	Filter      string `name:"filter" help:"Raw Drive Activity filter expression appended with AND"`
	Max         int64  `name:"max" aliases:"limit" help:"Page size" default:"10"`
	Page        string `name:"page" aliases:"cursor" help:"Page token"`
	All         bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	Consolidate bool   `name:"consolidate" help:"Use Drive Activity legacy consolidation strategy"`
	FailEmpty   bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no activities"`
}

func (c *DriveActivityQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if strings.TrimSpace(c.File) != "" && strings.TrimSpace(c.Folder) != "" {
		return usage("use either --file or --folder, not both")
	}
	req, err := c.queryRequest()
	if err != nil {
		return err
	}
	_, svc, err := requireDriveActivityService(ctx, flags)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*driveactivity.DriveActivity, string, error) {
		pageReq := *req
		pageReq.PageToken = pageToken
		resp, callErr := svc.Activity.Query(&pageReq).Context(ctx).Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Activities, resp.NextPageToken, nil
	}
	activities, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"activities":    activities,
			"nextPageToken": nextPageToken,
		}, len(activities), c.FailEmpty)
	}
	if len(activities) == 0 {
		u.Err().Println("No drive activity")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TIME\tACTION\tACTOR\tTARGET")
	for _, activity := range activities {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			activityTime(activity),
			driveActivityActionName(activity.PrimaryActionDetail),
			sanitizeTab(driveActivityActors(activity.Actors)),
			sanitizeTab(driveActivityTargets(activity.Targets)),
		)
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

func (c *DriveActivityQueryCmd) queryRequest() (*driveactivity.QueryDriveActivityRequest, error) {
	if c.Max <= 0 {
		return nil, usage("max must be > 0")
	}
	req := &driveactivity.QueryDriveActivityRequest{PageSize: c.Max}
	if fileID := strings.TrimSpace(c.File); fileID != "" {
		req.ItemName = "items/" + strings.TrimPrefix(fileID, "items/")
	}
	if folderID := strings.TrimSpace(c.Folder); folderID != "" {
		req.AncestorName = "items/" + strings.TrimPrefix(folderID, "items/")
	}
	if c.Consolidate {
		req.ConsolidationStrategy = &driveactivity.ConsolidationStrategy{Legacy: &driveactivity.Legacy{}}
	}
	filter, err := driveActivityFilter(c.Actions, c.From, c.To, c.Filter)
	if err != nil {
		return nil, err
	}
	req.Filter = filter
	return req, nil
}

func driveActivityFilter(actions, from, to, raw string) (string, error) {
	var parts []string
	if strings.TrimSpace(from) != "" {
		parts = append(parts, fmt.Sprintf("time >= %q", strings.TrimSpace(from)))
	}
	if strings.TrimSpace(to) != "" {
		parts = append(parts, fmt.Sprintf("time <= %q", strings.TrimSpace(to)))
	}
	if strings.TrimSpace(actions) != "" {
		mapped, err := driveActivityActionCases(actions)
		if err != nil {
			return "", err
		}
		if len(mapped) == 1 {
			parts = append(parts, "detail.action_detail_case:"+mapped[0])
		} else {
			parts = append(parts, "detail.action_detail_case:("+strings.Join(mapped, " ")+")")
		}
	}
	if strings.TrimSpace(raw) != "" {
		parts = append(parts, strings.TrimSpace(raw))
	}
	return strings.Join(parts, " AND "), nil
}

func driveActivityActionCases(actions string) ([]string, error) {
	actionMap := map[string]string{
		"applied_label_change": "APPLIED_LABEL_CHANGE",
		"label":                "APPLIED_LABEL_CHANGE",
		"comment":              "COMMENT",
		"delete":               "DELETE",
		"dlp":                  "DLP_CHANGE",
		"dlp_change":           "DLP_CHANGE",
		"edit":                 "EDIT",
		"move":                 "MOVE",
		"permission_change":    "PERMISSION_CHANGE",
		"share":                "PERMISSION_CHANGE",
		"reference":            "REFERENCE",
		"rename":               "RENAME",
		"restore":              "RESTORE",
		"settings":             "SETTINGS_CHANGE",
		"settings_change":      "SETTINGS_CHANGE",
	}
	actionMap[driveActivityActionCreate] = "CREATE"
	var out []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(actions, ",") {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		mapped, ok := actionMap[key]
		if !ok {
			return nil, usagef("unknown Drive Activity action %q", raw)
		}
		if !seen[mapped] {
			out = append(out, mapped)
			seen[mapped] = true
		}
	}
	if len(out) == 0 {
		return nil, usage("empty --actions")
	}
	return out, nil
}

func driveActivityActionName(detail *driveactivity.ActionDetail) string {
	switch {
	case detail == nil:
		return ""
	case detail.AppliedLabelChange != nil:
		return "applied_label_change"
	case detail.Comment != nil:
		return "comment"
	case detail.Create != nil:
		return driveActivityActionCreate
	case detail.Delete != nil:
		return "delete"
	case detail.DlpChange != nil:
		return "dlp_change"
	case detail.Edit != nil:
		return "edit"
	case detail.Move != nil:
		return "move"
	case detail.PermissionChange != nil:
		return "permission_change"
	case detail.Reference != nil:
		return "reference"
	case detail.Rename != nil:
		return "rename"
	case detail.Restore != nil:
		return "restore"
	case detail.SettingsChange != nil:
		return "settings_change"
	default:
		return ""
	}
}

func activityTime(activity *driveactivity.DriveActivity) string {
	if activity == nil {
		return ""
	}
	if activity.Timestamp != "" {
		return activity.Timestamp
	}
	if activity.TimeRange != nil {
		return activity.TimeRange.StartTime
	}
	return ""
}

func driveActivityActors(actors []*driveactivity.Actor) string {
	var parts []string
	for _, actor := range actors {
		switch {
		case actor == nil:
		case actor.User != nil && actor.User.KnownUser != nil:
			if actor.User.KnownUser.IsCurrentUser {
				parts = append(parts, "me")
			} else {
				parts = append(parts, actor.User.KnownUser.PersonName)
			}
		case actor.User != nil && actor.User.DeletedUser != nil:
			parts = append(parts, "deleted_user")
		case actor.User != nil:
			parts = append(parts, "unknown_user")
		case actor.Administrator != nil:
			parts = append(parts, "administrator")
		case actor.Anonymous != nil:
			parts = append(parts, "anonymous")
		case actor.System != nil:
			parts = append(parts, "system")
		case actor.Impersonation != nil:
			parts = append(parts, "impersonation")
		}
	}
	return strings.Join(parts, ",")
}

func driveActivityTargets(targets []*driveactivity.Target) string {
	var parts []string
	for _, target := range targets {
		switch {
		case target == nil:
		case target.DriveItem != nil:
			title := target.DriveItem.Title
			if title == "" {
				title = target.DriveItem.Name
			}
			parts = append(parts, title)
		case target.Drive != nil:
			parts = append(parts, target.Drive.Title)
		case target.FileComment != nil && target.FileComment.Parent != nil:
			parts = append(parts, target.FileComment.Parent.Title)
		}
	}
	return strings.Join(parts, ",")
}
