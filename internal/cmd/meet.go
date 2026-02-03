package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newMeetService = googleapi.NewMeet

type MeetCmd struct {
	Spaces MeetSpacesCmd `cmd:"" name:"spaces" help:"Manage Meet spaces"`
}

type MeetSpacesCmd struct {
	List   MeetSpacesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List meeting spaces"`
	Get    MeetSpacesGetCmd    `cmd:"" name:"get" help:"Get meeting space"`
	Create MeetSpacesCreateCmd `cmd:"" name:"create" help:"Create meeting space"`
	End    MeetSpacesEndCmd    `cmd:"" name:"end" help:"End active conference in space"`
}

type MeetSpacesListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *MeetSpacesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newMeetService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.ConferenceRecords.List()
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list conference records: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.ConferenceRecords) == 0 {
		u.Err().Println("No meeting spaces found")
		return nil
	}

	latest := make(map[string]*meet.ConferenceRecord)
	for _, record := range resp.ConferenceRecords {
		if record == nil || record.Space == "" {
			continue
		}
		if cur, ok := latest[record.Space]; !ok || cur.StartTime < record.StartTime {
			latest[record.Space] = record
		}
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SPACE\tLAST START\tLAST END")
	for space, record := range latest {
		endTime := ""
		if record != nil {
			endTime = record.EndTime
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", sanitizeTab(space), sanitizeTab(record.StartTime), sanitizeTab(endTime))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type MeetSpacesGetCmd struct {
	Space string `arg:"" name:"space" help:"Space name or meeting code"`
}

func (c *MeetSpacesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	space := strings.TrimSpace(c.Space)
	if space == "" {
		return usage("space is required")
	}
	if !strings.HasPrefix(space, "spaces/") {
		space = "spaces/" + space
	}

	svc, err := newMeetService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Get(space).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get space %s: %w", space, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	fmt.Fprintf(os.Stdout, "Name: %s\n", resp.Name)
	fmt.Fprintf(os.Stdout, "Meeting Code: %s\n", resp.MeetingCode)
	fmt.Fprintf(os.Stdout, "Meeting URI: %s\n", resp.MeetingUri)
	if resp.Config != nil {
		fmt.Fprintf(os.Stdout, "Access Type: %s\n", resp.Config.AccessType)
	}
	return nil
}

type MeetSpacesCreateCmd struct {
	AccessType string `name:"access-type" help:"Access type: OPEN|TRUSTED|RESTRICTED"`
}

func (c *MeetSpacesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	var cfg *meet.SpaceConfig
	if strings.TrimSpace(c.AccessType) != "" {
		accessType := strings.ToUpper(strings.TrimSpace(c.AccessType))
		switch accessType {
		case "OPEN", "TRUSTED", "RESTRICTED":
			cfg = &meet.SpaceConfig{AccessType: accessType}
		default:
			return usage("invalid --access-type (expected OPEN|TRUSTED|RESTRICTED)")
		}
	}

	svc, err := newMeetService(ctx, account)
	if err != nil {
		return err
	}

	space := &meet.Space{Config: cfg}
	created, err := svc.Spaces.Create(space).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create space: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created space: %s (%s)\n", created.Name, created.MeetingUri)
	return nil
}

type MeetSpacesEndCmd struct {
	Space string `arg:"" name:"space" help:"Space name or meeting code"`
}

func (c *MeetSpacesEndCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	space := strings.TrimSpace(c.Space)
	if space == "" {
		return usage("space is required")
	}
	if !strings.HasPrefix(space, "spaces/") {
		space = "spaces/" + space
	}

	svc, err := newMeetService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Spaces.EndActiveConference(space, &meet.EndActiveConferenceRequest{}).Context(ctx).Do(); err != nil {
		return fmt.Errorf("end active conference: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": space, "ended": true})
	}

	u.Out().Printf("Ended active conference in %s\n", space)
	return nil
}
