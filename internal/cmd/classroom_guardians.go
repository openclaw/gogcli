package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
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
	return runClassroomPagedList(ctx, flags, classroomPagedListOptions[classroom.Guardian]{
		parentName: "studentId", parentID: c.StudentID, max: c.Max, page: c.Page, all: c.All,
		failEmpty: c.FailEmpty, jsonKey: "guardians", emptyMessage: "No guardians", columns: classroomGuardianColumns(),
		fetch: func(ctx context.Context, svc *classroom.Service, studentID string, pageSize int64, pageToken string) ([]*classroom.Guardian, string, error) {
			call := svc.UserProfiles.Guardians.List(studentID).PageSize(pageSize).Context(ctx)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}
			if email := strings.TrimSpace(c.Email); email != "" {
				call.InvitedEmailAddress(email)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Guardians, resp.NextPageToken, nil
		},
	})
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

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	guardian, err := svc.UserProfiles.Guardians.Get(studentID, guardianID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"guardian": guardian})
	}

	u.Out().Linef("id\t%s", guardian.GuardianId)
	u.Out().Linef("student_id\t%s", guardian.StudentId)
	u.Out().Linef("email\t%s", profileEmail(guardian.GuardianProfile))
	u.Out().Linef("name\t%s", profileName(guardian.GuardianProfile))
	return nil
}

type ClassroomGuardiansDeleteCmd struct {
	StudentID  string `arg:"" name:"studentId" help:"Student ID"`
	GuardianID string `arg:"" name:"guardianId" help:"Guardian ID"`
}

func (c *ClassroomGuardiansDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomDelete(ctx, flags, c.StudentID, c.GuardianID, classroomDeleteOperation{
		op: "classroom.guardians.delete", parentName: "studentId", parentPayloadKey: "student_id", parentResultKey: "studentId",
		childName: "guardianId", childPayloadKey: "guardian_id", childResultKey: "guardianId", successResultKey: "deleted",
		action: func(studentID, guardianID string) string {
			return fmt.Sprintf("delete guardian %s for student %s", guardianID, studentID)
		},
		delete: func(svc *classroom.Service, studentID, guardianID string) error {
			_, err := svc.UserProfiles.Guardians.Delete(studentID, guardianID).Context(ctx).Do()
			return err
		},
	})
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
	return runClassroomPagedList(ctx, flags, classroomPagedListOptions[classroom.GuardianInvitation]{
		parentName: "studentId", parentID: c.StudentID, max: c.Max, page: c.Page, all: c.All,
		failEmpty: c.FailEmpty, jsonKey: "invitations", emptyMessage: "No guardian invitations", columns: classroomGuardianInvitationColumns(),
		fetch: func(ctx context.Context, svc *classroom.Service, studentID string, pageSize int64, pageToken string) ([]*classroom.GuardianInvitation, string, error) {
			call := svc.UserProfiles.GuardianInvitations.List(studentID).PageSize(pageSize).Context(ctx)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}
			if email := strings.TrimSpace(c.Email); email != "" {
				call.InvitedEmailAddress(email)
			}
			if states := upperClassroomStates(c.States); len(states) > 0 {
				call.States(states...)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", err
			}
			return resp.GuardianInvitations, resp.NextPageToken, nil
		},
	})
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

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	inv, err := svc.UserProfiles.GuardianInvitations.Get(studentID, invitationID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"invitation": inv})
	}

	u.Out().Linef("id\t%s", inv.InvitationId)
	u.Out().Linef("student_id\t%s", inv.StudentId)
	u.Out().Linef("email\t%s", inv.InvitedEmailAddress)
	u.Out().Linef("state\t%s", inv.State)
	if inv.CreationTime != "" {
		u.Out().Linef("created\t%s", inv.CreationTime)
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
	if err := dryRunExit(ctx, flags, "classroom.guardian-invitations.create", map[string]any{
		"student_id": studentID,
		"invitation": invite,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.UserProfiles.GuardianInvitations.Create(studentID, invite).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"invitation": created})
	}
	u.Out().Linef("id\t%s", created.InvitationId)
	u.Out().Linef("student_id\t%s", created.StudentId)
	u.Out().Linef("email\t%s", created.InvitedEmailAddress)
	return nil
}
