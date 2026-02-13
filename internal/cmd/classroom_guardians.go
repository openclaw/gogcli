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

type ClassroomGuardiansCmd struct {
	List   ClassroomGuardiansListCmd   `cmd:"" default:"withargs" help:"List guardians"`
	Get    ClassroomGuardiansGetCmd    `cmd:"" help:"Get a guardian"`
	Delete ClassroomGuardiansDeleteCmd `cmd:"" help:"Delete a guardian" aliases:"rm"`
}

type ClassroomGuardiansListCmd struct {
	StudentID string `arg:"" name:"studentId" help:"Student ID"`
	Email     string `name:"email" help:"Filter by invited email address"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" help:"Page token"`
}

func (c *ClassroomGuardiansListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	studentID := strings.TrimSpace(c.StudentID)
	if studentID == "" {
		return usage("empty studentId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	call := svc.UserProfiles.Guardians.List(studentID).PageSize(c.Max).PageToken(c.Page).Context(ctx)
	if v := strings.TrimSpace(c.Email); v != "" {
		call.InvitedEmailAddress(v)
	}

	resp, err := call.Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"guardians":     resp.Guardians,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Guardians) == 0 {
		u.Err().Println("No guardians")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "GUARDIAN_ID\tEMAIL\tNAME")
	for _, guardian := range resp.Guardians {
		if guardian == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(guardian.GuardianId),
			sanitizeTab(profileEmail(guardian.GuardianProfile)),
			sanitizeTab(profileName(guardian.GuardianProfile)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ClassroomGuardiansGetCmd struct {
	StudentID  string `arg:"" name:"studentId" help:"Student ID"`
	GuardianID string `arg:"" name:"guardianId" help:"Guardian ID"`
}

func (c *ClassroomGuardiansGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	studentID := strings.TrimSpace(c.StudentID)
	guardianID := strings.TrimSpace(c.GuardianID)
	if studentID == "" {
		return usage("empty studentId")
	}
	if guardianID == "" {
		return usage("empty guardianId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	guardian, err := svc.UserProfiles.Guardians.Get(studentID, guardianID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"guardian": guardian})
	}

	u.Out().Printf("id\t%s", guardian.GuardianId)
	u.Out().Printf("student_id\t%s", guardian.StudentId)
	u.Out().Printf("email\t%s", profileEmail(guardian.GuardianProfile))
	u.Out().Printf("name\t%s", profileName(guardian.GuardianProfile))
	return nil
}

type ClassroomGuardiansDeleteCmd struct {
	StudentID  string `arg:"" name:"studentId" help:"Student ID"`
	GuardianID string `arg:"" name:"guardianId" help:"Guardian ID"`
}

func (c *ClassroomGuardiansDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	studentID := strings.TrimSpace(c.StudentID)
	guardianID := strings.TrimSpace(c.GuardianID)
	if studentID == "" {
		return usage("empty studentId")
	}
	if guardianID == "" {
		return usage("empty guardianId")
	}

	err = confirmDestructive(ctx, flags, fmt.Sprintf("delete guardian %s for student %s", guardianID, studentID))
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.UserProfiles.Guardians.Delete(studentID, guardianID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":    true,
			"studentId":  studentID,
			"guardianId": guardianID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("student_id\t%s", studentID)
	u.Out().Printf("guardian_id\t%s", guardianID)
	return nil
}

type ClassroomGuardianInvitesCmd struct {
	List   ClassroomGuardianInvitesListCmd   `cmd:"" default:"withargs" help:"List guardian invitations"`
	Get    ClassroomGuardianInvitesGetCmd    `cmd:"" help:"Get a guardian invitation"`
	Create ClassroomGuardianInvitesCreateCmd `cmd:"" help:"Create a guardian invitation"`
}

type ClassroomGuardianInvitesListCmd struct {
	StudentID string `arg:"" name:"studentId" help:"Student ID"`
	Email     string `name:"email" help:"Filter by invited email address"`
	States    string `name:"state" help:"Invitation states filter (comma-separated: PENDING,COMPLETE)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" help:"Page token"`
}

func (c *ClassroomGuardianInvitesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	studentID := strings.TrimSpace(c.StudentID)
	if studentID == "" {
		return usage("empty studentId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	call := svc.UserProfiles.GuardianInvitations.List(studentID).PageSize(c.Max).PageToken(c.Page).Context(ctx)
	if v := strings.TrimSpace(c.Email); v != "" {
		call.InvitedEmailAddress(v)
	}
	if states := splitCSV(c.States); len(states) > 0 {
		upper := make([]string, 0, len(states))
		for _, state := range states {
			upper = append(upper, strings.ToUpper(state))
		}
		call.States(upper...)
	}

	resp, err := call.Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"invitations":   resp.GuardianInvitations,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.GuardianInvitations) == 0 {
		u.Err().Println("No guardian invitations")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "INVITATION_ID\tEMAIL\tSTATE\tCREATED")
	for _, inv := range resp.GuardianInvitations {
		if inv == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(inv.InvitationId),
			sanitizeTab(inv.InvitedEmailAddress),
			sanitizeTab(inv.State),
			sanitizeTab(inv.CreationTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ClassroomGuardianInvitesGetCmd struct {
	StudentID    string `arg:"" name:"studentId" help:"Student ID"`
	InvitationID string `arg:"" name:"invitationId" help:"Invitation ID"`
}

func (c *ClassroomGuardianInvitesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	studentID := strings.TrimSpace(c.StudentID)
	invitationID := strings.TrimSpace(c.InvitationID)
	if studentID == "" {
		return usage("empty studentId")
	}
	if invitationID == "" {
		return usage("empty invitationId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	inv, err := svc.UserProfiles.GuardianInvitations.Get(studentID, invitationID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"invitation": inv})
	}

	u.Out().Printf("id\t%s", inv.InvitationId)
	u.Out().Printf("student_id\t%s", inv.StudentId)
	u.Out().Printf("email\t%s", inv.InvitedEmailAddress)
	u.Out().Printf("state\t%s", inv.State)
	if inv.CreationTime != "" {
		u.Out().Printf("created\t%s", inv.CreationTime)
	}
	return nil
}

type ClassroomGuardianInvitesCreateCmd struct {
	StudentID string `arg:"" name:"studentId" help:"Student ID"`
	Email     string `name:"email" help:"Guardian email address" required:""`
}

func (c *ClassroomGuardianInvitesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	studentID := strings.TrimSpace(c.StudentID)
	if studentID == "" {
		return usage("empty studentId")
	}
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	invite := &classroom.GuardianInvitation{InvitedEmailAddress: email}
	created, err := svc.UserProfiles.GuardianInvitations.Create(studentID, invite).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"invitation": created})
	}
	u.Out().Printf("id\t%s", created.InvitationId)
	u.Out().Printf("student_id\t%s", created.StudentId)
	u.Out().Printf("email\t%s", created.InvitedEmailAddress)
	return nil
}
