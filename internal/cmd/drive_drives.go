package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DriveDrivesCmd lists all shared drives the user has access to.
type DriveDrivesCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results (max allowed: 100)" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Query     string `name:"query" short:"q" help:"Search query for filtering shared drives"`
}

func (c *DriveDrivesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*drive.Drive, string, error) {
		call := svc.Drives.List().
			PageSize(c.Max).
			Fields("nextPageToken, drives(id, name, createdTime)").
			Context(ctx)
		if page := strings.TrimSpace(pageToken); page != "" {
			call = call.PageToken(page)
		}
		if q := strings.TrimSpace(c.Query); q != "" {
			call = call.Q(q)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Drives, resp.NextPageToken, nil
	}

	drives, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"drives":        drives,
			"nextPageToken": nextPageToken,
		}, len(drives), c.FailEmpty)
	}

	if len(drives) == 0 {
		u.Err().Println("No shared drives")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tCREATED")
	for _, d := range drives {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\n",
			d.Id,
			d.Name,
			formatDateTime(d.CreatedTime),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}
