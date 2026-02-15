package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ClassroomInvitationsCmd struct {
	List   ClassroomInvitationsListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List invitations"`
	Get    ClassroomInvitationsGetCmd    `cmd:"" aliases:"info,show" help:"Get an invitation"`
	Create ClassroomInvitationsCreateCmd `cmd:"" aliases:"add,new" help:"Create an invitation"`
	Accept ClassroomInvitationsAcceptCmd `cmd:"" aliases:"join" help:"Accept an invitation"`
	Delete ClassroomInvitationsDeleteCmd `cmd:"" aliases:"rm,del,remove" help:"Delete an invitation"`
}

type ClassroomInvitationsListCmd struct {
	CourseID  string `name:"course" help:"Filter by course ID"`
	UserID    string `name:"user" help:"Filter by user ID or email"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ClassroomInvitationsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	fetch := func(pageToken string) ([]*classroom.Invitation, string, error) {
		call := svc.Invitations.List().PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		if v := strings.TrimSpace(c.CourseID); v != "" {
			call.CourseId(v)
		}
		if v := strings.TrimSpace(c.UserID); v != "" {
			call.UserId(v)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Invitations, resp.NextPageToken, nil
	}

	var invitations []*classroom.Invitation
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		invitations = all
	} else {
		var err error
		invitations, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"invitations":   invitations,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(invitations) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(invitations) == 0 {
		u.Err().Println("No invitations")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tCOURSE_ID\tUSER_ID\tROLE")
	for _, inv := range invitations {
		if inv == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(inv.Id),
			sanitizeTab(inv.CourseId),
			sanitizeTab(inv.UserId),
			sanitizeTab(inv.Role),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ClassroomInvitationsGetCmd struct {
	InvitationID string `arg:"" name:"invitationId" help:"Invitation ID"`
}

func (c *ClassroomInvitationsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	invitationID := strings.TrimSpace(c.InvitationID)
	if invitationID == "" {
		return usage("empty invitationId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	inv, err := svc.Invitations.Get(invitationID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"invitation": inv})
	}

	u.Out().Printf("id\t%s", inv.Id)
	u.Out().Printf("course_id\t%s", inv.CourseId)
	u.Out().Printf("user_id\t%s", inv.UserId)
	u.Out().Printf("role\t%s", inv.Role)
	return nil
}

type ClassroomInvitationsCreateCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID   string `arg:"" name:"userId" help:"User ID or email"`
	Role     string `name:"role" help:"Role: STUDENT, TEACHER, OWNER" required:""`
}

func (c *ClassroomInvitationsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	role := strings.TrimSpace(c.Role)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}
	if role == "" {
		return usage("empty role")
	}

	inv := &classroom.Invitation{CourseId: courseID, UserId: userID, Role: strings.ToUpper(role)}
	if err := dryRunExit(ctx, flags, "classroom.invitations.create", map[string]any{
		"invitation": inv,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Invitations.Create(inv).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"invitation": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("course_id\t%s", created.CourseId)
	u.Out().Printf("user_id\t%s", created.UserId)
	u.Out().Printf("role\t%s", created.Role)
	return nil
}

type ClassroomInvitationsAcceptCmd struct {
	InvitationID string `arg:"" name:"invitationId" help:"Invitation ID"`
}

func (c *ClassroomInvitationsAcceptCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	invitationID := strings.TrimSpace(c.InvitationID)
	if invitationID == "" {
		return usage("empty invitationId")
	}

	if err := dryRunExit(ctx, flags, "classroom.invitations.accept", map[string]any{
		"invitation_id": invitationID,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Invitations.Accept(invitationID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"accepted":     true,
			"invitationId": invitationID,
		})
	}
	u.Out().Printf("accepted\ttrue")
	u.Out().Printf("invitation_id\t%s", invitationID)
	return nil
}

type ClassroomInvitationsDeleteCmd struct {
	InvitationID string `arg:"" name:"invitationId" help:"Invitation ID"`
}

func (c *ClassroomInvitationsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	invitationID := strings.TrimSpace(c.InvitationID)
	if invitationID == "" {
		return usage("empty invitationId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete invitation %s", invitationID)); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Invitations.Delete(invitationID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("invitationId", invitationID),
	)
}
