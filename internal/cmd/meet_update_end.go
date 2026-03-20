package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// MeetUpdateCmd updates the configuration of a meeting space.
type MeetUpdateCmd struct {
	MeetingCode string `arg:"" name:"meeting-code" help:"Meeting code (e.g. abc-defg-hij)"`
	Access      string `name:"access" aliases:"access-type" help:"Access type: open, trusted, or restricted"`
	EntryPoint  string `name:"entry-point" aliases:"entry-point-access" help:"Entry point access: all or creator-only" hidden:""`
}

func (c *MeetUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	if strings.TrimSpace(c.MeetingCode) == "" {
		return usage("empty meeting code")
	}

	var updateMask []string

	patch := &meet.Space{Config: &meet.SpaceConfig{}}

	if c.Access != "" {
		accessType, err := parseMeetAccessType(c.Access)
		if err != nil {
			return err
		}

		patch.Config.AccessType = accessType
		updateMask = append(updateMask, "config.accessType")
	}

	if c.EntryPoint != "" {
		entryPointAccess, err := parseMeetEntryPointAccess(c.EntryPoint)
		if err != nil {
			return err
		}

		patch.Config.EntryPointAccess = entryPointAccess
		updateMask = append(updateMask, "config.entryPointAccess")
	}

	if len(updateMask) == 0 {
		return usage("at least one of --access or --entry-point is required")
	}

	_, svc, err := requireMeetService(ctx, flags)
	if err != nil {
		return wrapMeetError(err)
	}

	space, err := resolveMeetSpace(ctx, svc, c.MeetingCode)
	if err != nil {
		return wrapMeetError(err)
	}

	if dryRunErr := dryRunExit(ctx, flags, "meet.spaces.patch", map[string]any{
		"meeting_code": c.MeetingCode,
		"update_mask":  strings.Join(updateMask, ","),
		"config":       patch.Config,
	}); dryRunErr != nil {
		return dryRunErr
	}

	updated, err := svc.Spaces.Patch(space.Name, patch).
		UpdateMask(strings.Join(updateMask, ",")).
		Context(ctx).
		Do()
	if err != nil {
		return wrapMeetError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"updated":      true,
			"name":         updated.Name,
			"meeting_uri":  updated.MeetingUri,
			"meeting_code": updated.MeetingCode,
			"config":       updated.Config,
		})
	}

	u := ui.FromContext(ctx)
	printMeetSpace(u, updated)

	return nil
}

// MeetEndCmd ends an active conference in a meeting space.
type MeetEndCmd struct {
	MeetingCode string `arg:"" name:"meeting-code" help:"Meeting code (e.g. abc-defg-hij)"`
}

func (c *MeetEndCmd) Run(ctx context.Context, flags *RootFlags) error {
	if strings.TrimSpace(c.MeetingCode) == "" {
		return usage("empty meeting code")
	}

	_, svc, err := requireMeetService(ctx, flags)
	if err != nil {
		return wrapMeetError(err)
	}

	space, err := resolveMeetSpace(ctx, svc, c.MeetingCode)
	if err != nil {
		return wrapMeetError(err)
	}

	if confirmErr := confirmDestructive(ctx, flags, "end active conference in "+c.MeetingCode); confirmErr != nil {
		return confirmErr
	}

	req := &meet.EndActiveConferenceRequest{}

	_, err = svc.Spaces.EndActiveConference(space.Name, req).Context(ctx).Do()
	if err != nil {
		return wrapMeetError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"ended":        true,
			"meeting_code": c.MeetingCode,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("ended\ttrue")
	u.Out().Printf("meeting_code\t%s", c.MeetingCode)

	return nil
}
