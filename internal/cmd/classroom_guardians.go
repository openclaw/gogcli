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

type ClassroomGuardiansCmd struct {
	List   ClassroomGuardiansListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List guardians"`
	Get    ClassroomGuardiansGetCmd    `cmd:"" aliases:"info,show" help:"Get a guardian"`
	Delete ClassroomGuardiansDeleteCmd `cmd:"" aliases:"rm,del,remove" help:"Delete a guardian"`
}

type ClassroomGuardiansListCmd struct {
	StudentID string `arg:"" name:"studentId" help:"Student ID"`
	Email     string `name:"email" help:"Filter by invited email address"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*classroom.Guardian, string, error) {
		call := svc.UserProfiles.Guardians.List(studentID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		if v := strings.TrimSpace(c.Email); v != "" {
			call.InvitedEmailAddress(v)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Guardians, resp.NextPageToken, nil
	}

	var guardians []*classroom.Guardian
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		guardians = all
	} else {
		var err error
		guardians, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"guardians":     guardians,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(guardians) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(guardians) == 0 {
		u.Err().Println("No guardians")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "GUARDIAN_ID\tEMAIL\tNAME")
	for _, guardian := range guardians {
		if guardian == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(guardian.GuardianId),
			sanitizeTab(profileEmail(guardian.GuardianProfile)),
			sanitizeTab(profileName(guardian.GuardianProfile)),
		)
	}
	printNextPageHint(u, nextPageToken)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"guardian": guardian})
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
	studentID := strings.TrimSpace(c.StudentID)
	guardianID := strings.TrimSpace(c.GuardianID)
	if studentID == "" {
		return usage("empty studentId")
	}
	if guardianID == "" {
		return usage("empty guardianId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete guardian %s for student %s", guardianID, studentID)); err != nil {
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

	if _, err := svc.UserProfiles.Guardians.Delete(studentID, guardianID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("studentId", studentID),
		kv("guardianId", guardianID),
	)
}

type ClassroomGuardianInvitesCmd struct {
	List   ClassroomGuardianInvitesListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List guardian invitations"`
	Get    ClassroomGuardianInvitesGetCmd    `cmd:"" aliases:"info,show" help:"Get a guardian invitation"`
	Create ClassroomGuardianInvitesCreateCmd `cmd:"" aliases:"add,new" help:"Create a guardian invitation"`
}

type ClassroomGuardianInvitesListCmd struct {
	StudentID string `arg:"" name:"studentId" help:"Student ID"`
	Email     string `name:"email" help:"Filter by invited email address"`
	States    string `name:"state" help:"Invitation states filter (comma-separated: PENDING,COMPLETE)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*classroom.GuardianInvitation, string, error) {
		call := svc.UserProfiles.GuardianInvitations.List(studentID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
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
			return nil, "", wrapClassroomError(err)
		}
		return resp.GuardianInvitations, resp.NextPageToken, nil
	}

	var invitations []*classroom.GuardianInvitation
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
		u.Err().Println("No guardian invitations")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "INVITATION_ID\tEMAIL\tSTATE\tCREATED")
	for _, inv := range invitations {
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
	printNextPageHint(u, nextPageToken)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"invitation": inv})
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
	studentID := strings.TrimSpace(c.StudentID)
	if studentID == "" {
		return usage("empty studentId")
	}
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	invite := &classroom.GuardianInvitation{InvitedEmailAddress: email}
	if err := dryRunExit(ctx, flags, "classroom.guardian_invitations.create", map[string]any{
		"student_id": studentID,
		"invitation": invite,
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

	created, err := svc.UserProfiles.GuardianInvitations.Create(studentID, invite).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"invitation": created})
	}
	u.Out().Printf("id\t%s", created.InvitationId)
	u.Out().Printf("student_id\t%s", created.StudentId)
	u.Out().Printf("email\t%s", created.InvitedEmailAddress)
	return nil
}
