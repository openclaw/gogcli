package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type CalendarColorsCmd struct{}

func (c *CalendarColorsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}

	colors, err := svc.Colors.Get().Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"event":    colors.Event,
			"calendar": colors.Calendar,
		})
	}

	if len(colors.Event) == 0 && len(colors.Calendar) == 0 {
		u.Err().Println("No colors available")
		return nil
	}

	out := stdoutWriter(ctx)
	if len(colors.Event) > 0 {
		fmt.Fprintln(out, "EVENT COLORS:")
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tBACKGROUND\tFOREGROUND")

		ids := make([]int, 0, len(colors.Event))
		for id := range colors.Event {
			if num, err := strconv.Atoi(id); err == nil {
				ids = append(ids, num)
			}
		}
		sort.Ints(ids)

		for _, num := range ids {
			id := strconv.Itoa(num)
			c := colors.Event[id]
			fmt.Fprintf(tw, "%s\t%s\t%s\n", id, c.Background, c.Foreground)
		}
		_ = tw.Flush()
		fmt.Fprintln(out)
	}

	if len(colors.Calendar) > 0 {
		fmt.Fprintln(out, "CALENDAR COLORS:")
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tBACKGROUND\tFOREGROUND")

		ids := make([]int, 0, len(colors.Calendar))
		for id := range colors.Calendar {
			if num, err := strconv.Atoi(id); err == nil {
				ids = append(ids, num)
			}
		}
		sort.Ints(ids)

		for _, num := range ids {
			id := strconv.Itoa(num)
			c := colors.Calendar[id]
			fmt.Fprintf(tw, "%s\t%s\t%s\n", id, c.Background, c.Foreground)
		}
		_ = tw.Flush()
	}

	return nil
}
