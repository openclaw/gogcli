package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailFiltersCmd struct {
	List   GmailFiltersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List all email filters"`
	Get    GmailFiltersGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a specific filter"`
	Create GmailFiltersCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a new email filter"`
	Delete GmailFiltersDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a filter"`
	Export GmailFiltersExportCmd `cmd:"" name:"export" help:"Export filters as JSON"`
}

type GmailFiltersListCmd struct{}

func (c *GmailFiltersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return err
	}
	return writeGmailFiltersList(ctx, resp.Filter)
}

type GmailFiltersGetCmd struct {
	FilterID string `arg:"" name:"filterId" help:"Filter ID"`
}

func (c *GmailFiltersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	filterID := strings.TrimSpace(c.FilterID)
	if filterID == "" {
		return usage("empty filterId")
	}
	filter, err := svc.Users.Settings.Filters.Get("me", filterID).Do()
	if err != nil {
		return err
	}
	return writeGmailFilter(ctx, filter)
}

type GmailFiltersCreateCmd struct {
	From          string `name:"from" help:"Match messages from this sender"`
	To            string `name:"to" help:"Match messages to this recipient"`
	Subject       string `name:"subject" help:"Match messages with this subject"`
	Query         string `name:"query" help:"Advanced Gmail search query for matching"`
	HasAttachment bool   `name:"has-attachment" help:"Match messages with attachments"`
	AddLabel      string `name:"add-label" help:"Label(s) to add to matching messages (comma-separated, name or ID)"`
	RemoveLabel   string `name:"remove-label" help:"Label(s) to remove from matching messages (comma-separated, name or ID)"`
	Archive       bool   `name:"archive" help:"Archive matching messages (skip inbox)"`
	MarkRead      bool   `name:"mark-read" help:"Mark matching messages as read"`
	Star          bool   `name:"star" help:"Star matching messages"`
	Forward       string `name:"forward" help:"Forward to this email address"`
	Trash         bool   `name:"trash" help:"Move matching messages to trash"`
	NeverSpam     bool   `name:"never-spam" help:"Never mark as spam"`
	Important     bool   `name:"important" help:"Mark as important"`
}

func (c *GmailFiltersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	forwardTarget, err := c.validate()
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.filters.create", c.dryRunPayload(forwardTarget)); dryRunErr != nil {
		return dryRunErr
	}
	if forwardTarget != "" {
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("create gmail filter forwarding to %s", forwardTarget)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	filter, err := c.buildFilter(svc, forwardTarget)
	if err != nil {
		return err
	}

	created, err := svc.Users.Settings.Filters.Create("me", filter).Do()
	if err != nil {
		return err
	}
	return writeCreatedGmailFilter(ctx, created)
}

type GmailFiltersDeleteCmd struct {
	FilterID string `arg:"" name:"filterId" help:"Filter ID"`
}

func (c *GmailFiltersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	filterID := strings.TrimSpace(c.FilterID)
	if filterID == "" {
		return usage("empty filterId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete gmail filter %s", filterID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.Filters.Delete("me", filterID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"success":  true,
			"filterId": filterID,
		})
	}

	ui.FromContext(ctx).Out().Printf("Filter %s deleted successfully", filterID)
	return nil
}

type GmailFiltersExportCmd struct {
	Out string `name:"out" short:"o" help:"Write JSON export to this file (defaults to stdout)"`
}

func (c *GmailFiltersExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return err
	}

	payload := map[string]any{"filters": resp.Filter}
	outPath := strings.TrimSpace(c.Out)
	if outPath == "" {
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	f, outPath, err := createUserOutputFile(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if err := outfmt.WriteJSON(ctx, f, payload); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"exported": true,
			"path":     outPath,
			"count":    len(resp.Filter),
		})
	}

	ui.FromContext(ctx).Out().Printf("Exported %d filters to %s", len(resp.Filter), outPath)
	return nil
}
