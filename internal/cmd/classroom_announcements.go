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
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	fetch := func(pageToken string) ([]*classroom.Announcement, string, error) {
		call := svc.Courses.Announcements.List(courseID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		if states := splitCSV(c.States); len(states) > 0 {
			upper := make([]string, 0, len(states))
			for _, state := range states {
				upper = append(upper, strings.ToUpper(state))
			}
			call.AnnouncementStates(upper...)
		}
		if v := strings.TrimSpace(c.OrderBy); v != "" {
			call.OrderBy(v)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Announcements, resp.NextPageToken, nil
	}

	var announcements []*classroom.Announcement
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		announcements = all
	} else {
		var err error
		announcements, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"announcements": announcements,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(announcements) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(announcements) == 0 {
		u.Err().Println("No announcements")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tSTATE\tTEXT\tSCHEDULED\tUPDATED")
	for _, ann := range announcements {
		if ann == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(ann.Id),
			sanitizeTab(ann.State),
			sanitizeTab(truncateClassroomText(ann.Text, 50)),
			sanitizeTab(ann.ScheduledTime),
			sanitizeTab(ann.UpdateTime),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ClassroomAnnouncementsGetCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
}

func (c *ClassroomAnnouncementsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	ann, err := svc.Courses.Announcements.Get(courseID, announcementID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"announcement": ann})
	}

	u.Out().Printf("id\t%s", ann.Id)
	u.Out().Printf("state\t%s", ann.State)
	if ann.Text != "" {
		u.Out().Printf("text\t%s", ann.Text)
	}
	if ann.ScheduledTime != "" {
		u.Out().Printf("scheduled\t%s", ann.ScheduledTime)
	}
	if ann.AlternateLink != "" {
		u.Out().Printf("link\t%s", ann.AlternateLink)
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
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if strings.TrimSpace(c.Text) == "" {
		return usage("empty text")
	}

	ann := &classroom.Announcement{Text: strings.TrimSpace(c.Text)}
	if v := strings.TrimSpace(c.State); v != "" {
		ann.State = strings.ToUpper(v)
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		ann.ScheduledTime = v
	}

	if err := dryRunExit(ctx, flags, "classroom.announcements.create", map[string]any{
		"course_id":    courseID,
		"announcement": ann,
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

	created, err := svc.Courses.Announcements.Create(courseID, ann).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"announcement": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("state\t%s", created.State)
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
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	ann := &classroom.Announcement{}
	fields := make([]string, 0, 4)
	if v := strings.TrimSpace(c.Text); v != "" {
		ann.Text = v
		fields = append(fields, "text")
	}
	if v := strings.TrimSpace(c.State); v != "" {
		ann.State = strings.ToUpper(v)
		fields = append(fields, "state")
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		ann.ScheduledTime = v
		fields = append(fields, "scheduledTime")
	}
	if len(fields) == 0 {
		return usage("no updates specified")
	}

	if err := dryRunExit(ctx, flags, "classroom.announcements.update", map[string]any{
		"course_id":       courseID,
		"announcement_id": announcementID,
		"update_mask":     updateMask(fields),
		"update_fields":   fields,
		"announcement":    ann,
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

	updated, err := svc.Courses.Announcements.Patch(courseID, announcementID, ann).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"announcement": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("state\t%s", updated.State)
	return nil
}

type ClassroomAnnouncementsDeleteCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string `arg:"" name:"announcementId" help:"Announcement ID"`
}

func (c *ClassroomAnnouncementsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete announcement %s from %s", announcementID, courseID)); err != nil {
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

	if _, err := svc.Courses.Announcements.Delete(courseID, announcementID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
		kv("announcementId", announcementID),
	)
}

type ClassroomAnnouncementsAssigneesCmd struct {
	CourseID       string   `arg:"" name:"courseId" help:"Course ID or alias"`
	AnnouncementID string   `arg:"" name:"announcementId" help:"Announcement ID"`
	Mode           string   `name:"mode" help:"Assignee mode: ALL_STUDENTS, INDIVIDUAL_STUDENTS"`
	AddStudents    []string `name:"add-student" help:"Student IDs to add" sep:","`
	RemoveStudents []string `name:"remove-student" help:"Student IDs to remove" sep:","`
}

func (c *ClassroomAnnouncementsAssigneesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	announcementID := strings.TrimSpace(c.AnnouncementID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if announcementID == "" {
		return usage("empty announcementId")
	}

	mode, opts, err := normalizeAssigneeMode(c.Mode, c.AddStudents, c.RemoveStudents)
	if err != nil {
		return usage(err.Error())
	}
	req := &classroom.ModifyAnnouncementAssigneesRequest{
		AssigneeMode:                    mode,
		ModifyIndividualStudentsOptions: opts,
	}
	if req.AssigneeMode == "" && req.ModifyIndividualStudentsOptions == nil {
		return usage("no assignee changes specified")
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.announcements.assignees", map[string]any{
		"course_id":       courseID,
		"announcement_id": announcementID,
		"request":         req,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Announcements.ModifyAssignees(courseID, announcementID, req).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"announcement": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("assignee_mode\t%s", updated.AssigneeMode)
	return nil
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
