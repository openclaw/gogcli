package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

// DriveDrivesCmd lists all shared drives the user has access to.
type DriveDrivesCmd struct {
	Max   int64  `name:"max" aliases:"limit" help:"Max results (max allowed: 100)" default:"100"`
	Page  string `name:"page" help:"Page token"`
	Query string `name:"query" short:"q" help:"Search query for filtering shared drives"`
}

func (c *DriveDrivesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Drives.List().
		PageSize(c.Max).
		Fields("nextPageToken, drives(id, name, createdTime)").
		Context(ctx)

	if page := strings.TrimSpace(c.Page); page != "" {
		call = call.PageToken(page)
	}
	if q := strings.TrimSpace(c.Query); q != "" {
		call = call.Q(q)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"drives":        resp.Drives,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Drives) == 0 {
		u.Err().Println("No shared drives")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tCREATED")
	for _, d := range resp.Drives {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\n",
			d.Id,
			d.Name,
			formatDateTime(d.CreatedTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
