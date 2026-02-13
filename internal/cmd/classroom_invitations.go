package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

type ClassroomInvitationsCmd struct {
	List   ClassroomInvitationsListCmd   `cmd:"" default:"withargs" help:"List invitations"`
	Get    ClassroomInvitationsGetCmd    `cmd:"" help:"Get an invitation"`
	Create ClassroomInvitationsCreateCmd `cmd:"" help:"Create an invitation"`
	Accept ClassroomInvitationsAcceptCmd `cmd:"" help:"Accept an invitation"`
	Delete ClassroomInvitationsDeleteCmd `cmd:"" help:"Delete an invitation" aliases:"rm"`
}

type ClassroomInvitationsListCmd struct {
	CourseID string `name:"course" help:"Filter by course ID"`
	UserID   string `name:"user" help:"Filter by user ID or email"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page     string `name:"page" help:"Page token"`
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

	call := svc.Invitations.List().PageSize(c.Max).PageToken(c.Page).Context(ctx)
	if v := strings.TrimSpace(c.CourseID); v != "" {
		call.CourseId(v)
	}
	if v := strings.TrimSpace(c.UserID); v != "" {
		call.UserId(v)
	}

	resp, err := call.Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"invitations":   resp.Invitations,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Invitations) == 0 {
		u.Err().Println("No invitations")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tCOURSE_ID\tUSER_ID\tROLE")
	for _, inv := range resp.Invitations {
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
	printNextPageHint(u, resp.NextPageToken)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"invitation": inv})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	inv := &classroom.Invitation{CourseId: courseID, UserId: userID, Role: strings.ToUpper(role)}
	created, err := svc.Invitations.Create(inv).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"invitation": created})
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

	if _, err := svc.Invitations.Accept(invitationID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	invitationID := strings.TrimSpace(c.InvitationID)
	if invitationID == "" {
		return usage("empty invitationId")
	}

	err = confirmDestructive(ctx, flags, fmt.Sprintf("delete invitation %s", invitationID))
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":      true,
			"invitationId": invitationID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("invitation_id\t%s", invitationID)
	return nil
}
