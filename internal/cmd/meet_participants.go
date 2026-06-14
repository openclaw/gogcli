package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// MeetParticipantsCmd lists participants from the most recent call in a meeting,
// or from a specific conference if --conference is provided.
type MeetParticipantsCmd struct {
	MeetingCode string `arg:"" name:"meeting-code" help:"Meeting code (e.g. abc-defg-hij)"`
	Conference  string `name:"conference" help:"Specific conference ID (default: most recent call)"`
	Max         int    `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page        string `name:"page" aliases:"cursor" help:"Page token"`
	All         bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty   bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *MeetParticipantsCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	// Determine which conference to list participants for.
	conferenceName, err := resolveConference(ctx, svc, c.MeetingCode, c.Conference)
	if err != nil {
		return err
	}

	participants := make([]*meet.Participant, 0)
	nextPageToken := ""

	if conferenceName != "" {
		fetch := func(pageToken string) ([]*meet.Participant, string, error) {
			call := svc.ConferenceRecords.Participants.List(conferenceName).
				PageSize(int64(c.Max)).
				Context(ctx)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}

			resp, err := call.Do()
			if err != nil {
				return nil, "", wrapMeetError(err)
			}

			return resp.Participants, resp.NextPageToken, nil
		}

		if c.All {
			all, err := collectAllPages(c.Page, fetch)
			if err != nil {
				return err
			}

			participants = all
		} else {
			var err error

			participants, nextPageToken, err = fetch(c.Page)
			if err != nil {
				return err
			}
		}
	}
	if participants == nil {
		participants = make([]*meet.Participant, 0)
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"meeting_code":  c.MeetingCode,
			"conference":    conferenceName,
			"participants":  participants,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}

		if len(participants) == 0 {
			return failEmptyExit(c.FailEmpty)
		}

		return nil
	}

	if len(participants) == 0 {
		u.Err().Linef("No participants found for %s", c.MeetingCode)

		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "DISPLAY_NAME\tJOINED\tLEFT")

	for _, p := range participants {
		if p == nil {
			continue
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(participantDisplayName(p)),
			sanitizeTab(p.EarliestStartTime),
			sanitizeTab(p.LatestEndTime),
		)
	}

	printNextPageHint(u, nextPageToken)

	return nil
}

// resolveConference determines the conference record name to use.
// If an explicit conference ID is provided, it is used directly.
// Otherwise, the most recent conference for the meeting is looked up.
func resolveConference(ctx context.Context, svc *meet.Service, meetingCode, conferenceOverride string) (string, error) {
	if conferenceOverride != "" {
		name := strings.TrimSpace(conferenceOverride)
		if !strings.HasPrefix(name, "conferenceRecords/") {
			name = "conferenceRecords/" + name
		}

		return name, nil
	}

	// Look up the most recent conference for this meeting.
	space, err := resolveMeetSpace(ctx, svc, meetingCode)
	if err != nil {
		return "", wrapMeetError(err)
	}

	filter := meetSpaceNameFilter(space.Name)

	resp, err := svc.ConferenceRecords.List().
		Filter(filter).
		PageSize(1).
		Context(ctx).
		Do()
	if err != nil {
		return "", wrapMeetError(err)
	}

	if len(resp.ConferenceRecords) == 0 {
		return "", nil
	}

	return resp.ConferenceRecords[0].Name, nil
}
