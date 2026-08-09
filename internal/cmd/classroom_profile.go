package cmd

import (
	"context"
	"strings"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ClassroomProfileCmd struct {
	Get ClassroomProfileGetCmd `cmd:"" default:"withargs" help:"Get a user profile"`
}

type ClassroomProfileGetCmd struct {
	UserID string `arg:"" name:"userId" optional:"" help:"User ID or email (default: me)"`
}

func (c *ClassroomProfileGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		userID = "me"
	}

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	profile, err := svc.UserProfiles.Get(userID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"profile": profile})
	}

	u.Out().Linef("id\t%s", profile.Id)
	u.Out().Linef("email\t%s", profile.EmailAddress)
	u.Out().Linef("name\t%s", profileName(profile))
	u.Out().Linef("verified_teacher\t%t", profile.VerifiedTeacher)
	if profile.PhotoUrl != "" {
		u.Out().Linef("photo_url\t%s", profile.PhotoUrl)
	}
	return nil
}
