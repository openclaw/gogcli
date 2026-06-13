package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// MeetHistoryCmd lists past conferences (calls) for a meeting.
type MeetHistoryCmd struct {
	MeetingCode string `arg:"" name:"meeting-code" help:"Meeting code (e.g. abc-defg-hij)"`
	Max         int    `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page        string `name:"page" aliases:"cursor" help:"Page token"`
	All         bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty   bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *MeetHistoryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	if strings.TrimSpace(c.MeetingCode) == "" {
		return usage("empty meeting code")
	}

	if c.Max <= 0 {
		return usage("--max must be > 0")
	}

	_, svc, err := requireMeetService(ctx, flags)
	if err != nil {
		return wrapMeetError(err)
	}

	// Resolve the meeting code to the canonical space name for filtering.
	space, err := resolveMeetSpace(ctx, svc, c.MeetingCode)
	if err != nil {
		return wrapMeetError(err)
	}

	filter := meetSpaceNameFilter(space.Name)

	fetch := func(pageToken string) ([]*meet.ConferenceRecord, string, error) {
		call := svc.ConferenceRecords.List().
			PageSize(int64(c.Max)).
			Filter(filter).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapMeetError(err)
		}

		return resp.ConferenceRecords, resp.NextPageToken, nil
	}

	var records []*meet.ConferenceRecord
	nextPageToken := ""

	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}

		records = all
	} else {
		var err error

		records, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if records == nil {
		records = make([]*meet.ConferenceRecord, 0)
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"meeting_code":  c.MeetingCode,
			"conferences":   records,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}

		if len(records) == 0 {
			return failEmptyExit(c.FailEmpty)
		}

		return nil
	}

	if len(records) == 0 {
		u.Err().Linef("No past calls found for %s", c.MeetingCode)

		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "START\tEND\tCONFERENCE")

	for _, r := range records {
		if r == nil {
			continue
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(r.StartTime),
			sanitizeTab(r.EndTime),
			sanitizeTab(strings.TrimPrefix(r.Name, "conferenceRecords/")),
		)
	}

	printNextPageHint(u, nextPageToken)

	return nil
}
