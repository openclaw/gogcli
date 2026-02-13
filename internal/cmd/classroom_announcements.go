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

type ClassroomAnnouncementsCmd struct {
	List      ClassroomAnnouncementsListCmd      `cmd:"" default:"withargs" help:"List announcements"`
	Get       ClassroomAnnouncementsGetCmd       `cmd:"" help:"Get an announcement"`
	Create    ClassroomAnnouncementsCreateCmd    `cmd:"" help:"Create an announcement"`
	Update    ClassroomAnnouncementsUpdateCmd    `cmd:"" help:"Update an announcement"`
	Delete    ClassroomAnnouncementsDeleteCmd    `cmd:"" help:"Delete an announcement" aliases:"rm"`
	Assignees ClassroomAnnouncementsAssigneesCmd `cmd:"" name:"assignees" help:"Modify announcement assignees"`
}

type ClassroomAnnouncementsListCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	States   string `name:"state" help:"Announcement states filter (comma-separated: DRAFT,PUBLISHED,DELETED)"`
	OrderBy  string `name:"order-by" help:"Order by (e.g., updateTime desc)"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page     string `name:"page" help:"Page token"`
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

	call := svc.Courses.Announcements.List(courseID).PageSize(c.Max).PageToken(c.Page).Context(ctx)
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
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"announcements": resp.Announcements,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Announcements) == 0 {
		u.Err().Println("No announcements")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tSTATE\tTEXT\tSCHEDULED\tUPDATED")
	for _, ann := range resp.Announcements {
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
	printNextPageHint(u, resp.NextPageToken)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"announcement": ann})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if strings.TrimSpace(c.Text) == "" {
		return usage("empty text")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	ann := &classroom.Announcement{Text: strings.TrimSpace(c.Text)}
	if v := strings.TrimSpace(c.State); v != "" {
		ann.State = strings.ToUpper(v)
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		ann.ScheduledTime = v
	}

	created, err := svc.Courses.Announcements.Create(courseID, ann).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"announcement": created})
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Announcements.Patch(courseID, announcementID, ann).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"announcement": updated})
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

	err = confirmDestructive(ctx, flags, fmt.Sprintf("delete announcement %s from %s", announcementID, courseID))
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":        true,
			"courseId":       courseID,
			"announcementId": announcementID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	u.Out().Printf("announcement_id\t%s", announcementID)
	return nil
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Announcements.ModifyAssignees(courseID, announcementID, req).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"announcement": updated})
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
