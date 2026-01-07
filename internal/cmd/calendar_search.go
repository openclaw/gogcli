package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarSearchCmd struct {
	Query      string `arg:"" name:"query" help:"Search query"`
	From       string `name:"from" help:"Start time (RFC3339; default: 30 days ago)"`
	To         string `name:"to" help:"End time (RFC3339; default: 90 days from now)"`
	CalendarID string `name:"calendar" help:"Calendar ID" default:"primary"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
}

func (c *CalendarSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(c.Query)
	if query == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	now := time.Now().UTC()
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)
	ninetyDaysLater := now.Add(90 * 24 * time.Hour)

	from := strings.TrimSpace(c.From)
	to := strings.TrimSpace(c.To)
	if from == "" {
		from = thirtyDaysAgo.Format(time.RFC3339)
	}
	if to == "" {
		to = ninetyDaysLater.Format(time.RFC3339)
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Events.List(c.CalendarID).
		Q(query).
		TimeMin(from).
		TimeMax(to).
		MaxResults(c.Max).
		SingleEvents(true).
		OrderBy("startTime")

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"events": resp.Items,
			"query":  query,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No events found")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range resp.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	_ = tw.Flush()
	return nil
}
