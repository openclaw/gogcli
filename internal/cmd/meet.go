package cmd

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// openMeetBrowser opens the meeting URL in the default browser.
var openMeetBrowser = func(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // executable is fixed; arg is meeting URL
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // executable is fixed; arg is meeting URL
	default:
		cmd = exec.Command("xdg-open", url) //nolint:gosec // executable is fixed; arg is meeting URL
	}

	return cmd.Start()
}

var newMeetService = googleapi.NewMeet

type MeetCmd struct {
	Create       MeetCreateCmd       `cmd:"" name:"create" aliases:"new" help:"Create a meeting space"`
	Get          MeetGetCmd          `cmd:"" name:"get" aliases:"info,show" help:"Get a meeting space"`
	Update       MeetUpdateCmd       `cmd:"" name:"update" aliases:"edit,set" help:"Update space config"`
	End          MeetEndCmd          `cmd:"" name:"end" aliases:"stop" help:"End active conference"`
	History      MeetHistoryCmd      `cmd:"" name:"history" aliases:"calls,past" help:"List past calls in a meeting"`
	Participants MeetParticipantsCmd `cmd:"" name:"participants" aliases:"people,attendees,who" help:"List participants from the latest call"`
}

// MeetCreateCmd creates a new meeting space.
type MeetCreateCmd struct {
	Access     string `name:"access" aliases:"access-type" help:"Access type: open, trusted, or restricted" default:"trusted"`
	EntryPoint string `name:"entry-point" aliases:"entry-point-access" help:"Entry point access: all or creator-only" default:"all" hidden:""`
	Open       bool   `name:"open" aliases:"browser" help:"Open the meeting in a browser after creation"`
}

func (c *MeetCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	accessType, err := parseMeetAccessType(c.Access)
	if err != nil {
		return err
	}

	entryPointAccess, err := parseMeetEntryPointAccess(c.EntryPoint)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "meet.spaces.create", map[string]any{
		"access_type":        accessType,
		"entry_point_access": entryPointAccess,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireMeetService(ctx, flags)
	if err != nil {
		return wrapMeetError(err)
	}

	space := &meet.Space{
		Config: &meet.SpaceConfig{
			AccessType:       accessType,
			EntryPointAccess: entryPointAccess,
		},
	}

	created, err := svc.Spaces.Create(space).Context(ctx).Do()
	if err != nil {
		return wrapMeetError(err)
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"created":      true,
			"name":         created.Name,
			"meeting_uri":  created.MeetingUri,
			"meeting_code": created.MeetingCode,
			"config":       created.Config,
		}); err != nil {
			return err
		}

		return openMeetingIfRequested(c.Open, created.MeetingUri)
	}

	u := ui.FromContext(ctx)
	printMeetSpace(u, created)

	return openMeetingIfRequested(c.Open, created.MeetingUri)
}

// MeetGetCmd gets a meeting space by meeting code.
type MeetGetCmd struct {
	MeetingCode string `arg:"" name:"meeting-code" help:"Meeting code (e.g. abc-defg-hij)"`
}

func (c *MeetGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	spaceName := normalizeMeetSpaceName(c.MeetingCode)
	if spaceName == "" {
		return usage("empty meeting code")
	}

	_, svc, err := requireMeetService(ctx, flags)
	if err != nil {
		return wrapMeetError(err)
	}

	space, err := svc.Spaces.Get(spaceName).Context(ctx).Do()
	if err != nil {
		return wrapMeetError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"name":         space.Name,
			"meeting_uri":  space.MeetingUri,
			"meeting_code": space.MeetingCode,
			"config":       space.Config,
		})
	}

	u := ui.FromContext(ctx)
	printMeetSpace(u, space)

	return nil
}

func printMeetSpace(u *ui.UI, space *meet.Space) {
	if u == nil || space == nil {
		return
	}

	u.Out().Printf("meeting_code\t%s", space.MeetingCode)
	u.Out().Printf("meeting_uri\t%s", space.MeetingUri)

	if space.Config != nil {
		u.Out().Printf("access\t%s", strings.ToLower(space.Config.AccessType))
	}

	if space.ActiveConference != nil && space.ActiveConference.ConferenceRecord != "" {
		u.Out().Printf("active_conference\t%s", space.ActiveConference.ConferenceRecord)
	}
}

// normalizeMeetSpaceName accepts either a full resource name ("spaces/xxx")
// or a bare meeting code ("xxx-yyyy-zzz") and returns a value suitable for
// the spaces.get API, which accepts both formats.
func normalizeMeetSpaceName(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	if strings.HasPrefix(input, "spaces/") {
		return input
	}

	return "spaces/" + input
}

// resolveMeetSpace resolves a meeting code to the full Space resource.
// This is needed because some API methods (patch, endActiveConference)
// require the canonical resource name (e.g. "spaces/KP0uKCifZgYB"),
// which differs from the meeting code (e.g. "abc-defg-hij").
func resolveMeetSpace(ctx context.Context, svc *meet.Service, input string) (*meet.Space, error) {
	return svc.Spaces.Get(normalizeMeetSpaceName(input)).Context(ctx).Do()
}

func openMeetingIfRequested(shouldOpen bool, meetingURI string) error {
	if !shouldOpen || strings.TrimSpace(meetingURI) == "" {
		return nil
	}

	return openMeetBrowser(meetingURI)
}

func parseMeetAccessType(s string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "trusted", "":
		return "TRUSTED", nil
	case "open":
		return "OPEN", nil
	case "restricted":
		return "RESTRICTED", nil
	default:
		return "", usagef("invalid --access %q (expected open, trusted, or restricted)", s)
	}
}

func parseMeetEntryPointAccess(s string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case literalAll, "":
		return "ALL", nil
	case "creator-only", "creator_only", "creatoronly":
		return "CREATOR_APP_ONLY", nil
	default:
		return "", usagef("invalid --entry-point %q (expected all or creator-only)", s)
	}
}
