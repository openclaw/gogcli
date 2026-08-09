package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ClassroomAnnouncementsCmd struct {
	List      ClassroomAnnouncementsListCmd      `cmd:"" default:"withargs" aliases:"ls" help:"List announcements"`
	Get       ClassroomAnnouncementsGetCmd       `cmd:"" aliases:"info,show" help:"Get an announcement"`
	Create    ClassroomAnnouncementsCreateCmd    `cmd:"" aliases:"add,new" help:"Create an announcement"`
	Update    ClassroomAnnouncementsUpdateCmd    `cmd:"" aliases:"edit,set" help:"Update an announcement"`
	Delete    ClassroomAnnouncementsDeleteCmd    `cmd:"" aliases:"rm,del,remove" help:"Delete an announcement"`
	Assignees ClassroomAnnouncementsAssigneesCmd `cmd:"" name:"assignees" aliases:"assign" help:"Modify announcement assignees"`
}

type ClassroomAnnouncementsListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	States    string `name:"state" help:"Announcement states filter (comma-separated: DRAFT,PUBLISHED,DELETED)"`
	OrderBy   string `name:"order-by" help:"Order by (e.g., updateTime desc)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ClassroomAnnouncementsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomPagedList(ctx, flags, classroomPagedListOptions[classroom.Announcement]{
		parentName: "courseId", parentID: c.CourseID, max: c.Max, page: c.Page, all: c.All,
		failEmpty: c.FailEmpty, jsonKey: "announcements", emptyMessage: "No announcements", columns: classroomAnnouncementColumns(),
		fetch: func(ctx context.Context, svc *classroom.Service, courseID string, pageSize int64, pageToken string) ([]*classroom.Announcement, string, error) {
			call := svc.Courses.Announcements.List(courseID).PageSize(pageSize).Context(ctx)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}
			if states := upperClassroomStates(c.States); len(states) > 0 {
				call.AnnouncementStates(states...)
			}
			if orderBy := strings.TrimSpace(c.OrderBy); orderBy != "" {
				call.OrderBy(orderBy)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", err
			}
			return resp.Announcements, resp.NextPageToken, nil
		},
	})
}

type ClassroomAnnouncementsGetCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
}

func (c *ClassroomAnnouncementsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	ann, err := svc.Courses.Announcements.Get(courseID, announcementID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": ann})
	}

	u.Out().Linef("id\t%s", ann.Id)
	u.Out().Linef("state\t%s", ann.State)
	if ann.Text != "" {
		u.Out().Linef("text\t%s", ann.Text)
	}
	if ann.ScheduledTime != "" {
		u.Out().Linef("scheduled\t%s", ann.ScheduledTime)
	}
	if ann.AlternateLink != "" {
		u.Out().Linef("link\t%s", ann.AlternateLink)
	}
	return nil
}

type ClassroomAnnouncementsCreateCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	Text      string `name:"text" help:"Announcement text" required:""`
	State     string `name:"state" help:"State: PUBLISHED, DRAFT"`
	Scheduled string `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
}

func (c *ClassroomAnnouncementsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomAnnouncementCreatePlan(classroomAnnouncementInput{
		CourseID:  c.CourseID,
		Text:      c.Text,
		State:     c.State,
		Scheduled: c.Scheduled,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.announcements.create", map[string]any{
		"course_id":    plan.CourseID,
		"announcement": plan.Announcement,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.Announcements.Create(plan.CourseID, plan.Announcement).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("state\t%s", created.State)
	return nil
}

type ClassroomAnnouncementsUpdateCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
	Text           string `name:"text" help:"Announcement text"`
	State          string `name:"state" help:"State: PUBLISHED, DRAFT"`
	Scheduled      string `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
}

func (c *ClassroomAnnouncementsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomAnnouncementUpdatePlan(classroomAnnouncementUpdateInput{
		classroomAnnouncementInput: classroomAnnouncementInput{
			CourseID:  c.CourseID,
			Text:      c.Text,
			State:     c.State,
			Scheduled: c.Scheduled,
		},
		AnnouncementID: c.AnnouncementID,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.announcements.update", map[string]any{
		"course_id":       plan.CourseID,
		"announcement_id": plan.AnnouncementID,
		"update_mask":     plan.UpdateMask,
		"update_fields":   plan.UpdateFields,
		"announcement":    plan.Announcement,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Announcements.Patch(plan.CourseID, plan.AnnouncementID, plan.Announcement).UpdateMask(plan.UpdateMask).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"announcement": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("state\t%s", updated.State)
	return nil
}

type ClassroomAnnouncementsDeleteCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
}

func (c *ClassroomAnnouncementsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomDelete(ctx, flags, c.CourseID, c.AnnouncementID, classroomDeleteOperation{
		op: "classroom.announcements.delete", parentName: "courseId", parentPayloadKey: "course_id", parentResultKey: "courseId",
		childName: "announcementId", childPayloadKey: "announcement_id", childResultKey: "announcementId", successResultKey: "deleted",
		action: func(courseID, announcementID string) string {
			return fmt.Sprintf("delete announcement %s from %s", announcementID, courseID)
		},
		delete: func(svc *classroom.Service, courseID, announcementID string) error {
			_, err := svc.Courses.Announcements.Delete(courseID, announcementID).Context(ctx).Do()
			return err
		},
	})
}

type ClassroomAnnouncementsAssigneesCmd struct {
	CourseID       string   `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string   `arg:"" name:"announcementId" help:"Announcement ID"`
	Mode           string   `name:"mode" help:"Assignee mode: ALL_STUDENTS, INDIVIDUAL_STUDENTS"`
	AddStudents    []string `name:"add-student" help:"Student IDs to add" sep:","`
	RemoveStudents []string `name:"remove-student" help:"Student IDs to remove" sep:","`
}

func (c *ClassroomAnnouncementsAssigneesCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomAssigneeMutation(ctx, flags, c.CourseID, c.AnnouncementID, c.Mode, c.AddStudents, c.RemoveStudents,
		classroomAnnouncementAssigneeOperation(ctx))
}

func classroomAnnouncementAssigneeOperation(ctx context.Context) classroomAssigneeOperation[*classroom.ModifyAnnouncementAssigneesRequest, classroom.Announcement] {
	return classroomAssigneeOperation[*classroom.ModifyAnnouncementAssigneesRequest, classroom.Announcement]{
		op: "classroom.announcements.assignees", itemName: "announcementId", itemPayloadKey: "announcement_id", jsonKey: "announcement",
		buildRequest: buildClassroomAnnouncementAssigneeRequest,
		mutate: func(svc *classroom.Service, courseID, announcementID string, request *classroom.ModifyAnnouncementAssigneesRequest) (*classroom.Announcement, error) {
			return svc.Courses.Announcements.ModifyAssignees(courseID, announcementID, request).Context(ctx).Do()
		},
		resultID:           func(announcement *classroom.Announcement) string { return announcement.Id },
		resultAssigneeMode: func(announcement *classroom.Announcement) string { return announcement.AssigneeMode },
	}
}

func buildClassroomAnnouncementAssigneeRequest(mode string, options *classroom.ModifyIndividualStudentsOptions) (*classroom.ModifyAnnouncementAssigneesRequest, error) {
	if err := validateClassroomAssigneeChanges(mode, options); err != nil {
		return nil, err
	}
	return &classroom.ModifyAnnouncementAssigneesRequest{AssigneeMode: mode, ModifyIndividualStudentsOptions: options}, nil
}

func truncateClassroomText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" || maxLen <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "..."
}
