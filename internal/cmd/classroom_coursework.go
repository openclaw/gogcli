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

type ClassroomCourseworkCmd struct {
	List      ClassroomCourseworkListCmd      `cmd:"" default:"withargs" aliases:"ls" help:"List coursework"`
	Get       ClassroomCourseworkGetCmd       `cmd:"" aliases:"info,show" help:"Get coursework"`
	Create    ClassroomCourseworkCreateCmd    `cmd:"" aliases:"add,new" help:"Create coursework"`
	Update    ClassroomCourseworkUpdateCmd    `cmd:"" aliases:"edit,set" help:"Update coursework"`
	Delete    ClassroomCourseworkDeleteCmd    `cmd:"" aliases:"rm,del,remove" help:"Delete coursework"`
	Assignees ClassroomCourseworkAssigneesCmd `cmd:"" name:"assignees" aliases:"assign" help:"Modify coursework assignees"`
}

type ClassroomCourseworkListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	States    string `name:"state" help:"Coursework states filter (comma-separated: DRAFT,PUBLISHED,DELETED)"`
	Topic     string `name:"topic" help:"Filter by topic ID"`
	OrderBy   string `name:"order-by" help:"Order by (e.g., updateTime desc, dueDate desc)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	ScanPages int    `name:"scan-pages" help:"Pages to scan when filtering by topic" default:"3"`
}

func (c *ClassroomCourseworkListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	makeCall := func(page string) (*classroom.ListCourseWorkResponse, error) {
		call := svc.Courses.CourseWork.List(courseID).PageSize(c.Max).PageToken(page).Context(ctx)
		if states := splitCSV(c.States); len(states) > 0 {
			upper := make([]string, 0, len(states))
			for _, state := range states {
				upper = append(upper, strings.ToUpper(state))
			}
			call.CourseWorkStates(upper...)
		}
		if v := strings.TrimSpace(c.OrderBy); v != "" {
			call.OrderBy(v)
		}
		return call.Do()
	}

	fetch := func(page string) ([]*classroom.CourseWork, string, error) {
		resp, callErr := makeCall(page)
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.CourseWork, resp.NextPageToken, nil
	}

	var coursework []*classroom.CourseWork
	var nextPageToken string
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return wrapClassroomError(err)
		}
		coursework = all
		if topic := strings.TrimSpace(c.Topic); topic != "" {
			filtered := coursework[:0]
			for _, work := range coursework {
				if work == nil {
					continue
				}
				if work.TopicId == topic {
					filtered = append(filtered, work)
				}
			}
			coursework = filtered
		}
	} else {
		var err error
		coursework, nextPageToken, err = scanClassroomTopicPages(
			c.Topic,
			c.Page,
			c.ScanPages,
			fetch,
			func(work *classroom.CourseWork) string {
				if work == nil {
					return ""
				}
				return work.TopicId
			},
		)
		if err != nil {
			return wrapClassroomError(err)
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"coursework":    coursework,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(coursework) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(coursework) == 0 {
		u.Err().Println("No coursework")
		printNextPageHint(u, nextPageToken)
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tSTATE\tDUE\tTYPE\tMAX_POINTS")
	for _, work := range coursework {
		if work == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(work.Id),
			sanitizeTab(work.Title),
			sanitizeTab(work.State),
			sanitizeTab(formatClassroomDue(work.DueDate, work.DueTime)),
			sanitizeTab(work.WorkType),
			formatFloatValue(work.MaxPoints),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ClassroomCourseworkGetCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
}

func (c *ClassroomCourseworkGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	work, err := svc.Courses.CourseWork.Get(courseID, courseworkID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"coursework": work})
	}

	u.Out().Printf("id\t%s", work.Id)
	u.Out().Printf("title\t%s", work.Title)
	if work.Description != "" {
		u.Out().Printf("description\t%s", work.Description)
	}
	u.Out().Printf("state\t%s", work.State)
	u.Out().Printf("type\t%s", work.WorkType)
	if due := formatClassroomDue(work.DueDate, work.DueTime); due != "" {
		u.Out().Printf("due\t%s", due)
	}
	if work.ScheduledTime != "" {
		u.Out().Printf("scheduled\t%s", work.ScheduledTime)
	}
	if work.TopicId != "" {
		u.Out().Printf("topic_id\t%s", work.TopicId)
	}
	if work.MaxPoints != 0 {
		u.Out().Printf("max_points\t%s", formatFloatValue(work.MaxPoints))
	}
	if work.AlternateLink != "" {
		u.Out().Printf("link\t%s", work.AlternateLink)
	}
	return nil
}

type ClassroomCourseworkCreateCmd struct {
	CourseID    string  `arg:"" name:"courseId" help:"Course ID or alias"`
	Title       string  `name:"title" help:"Title" required:""`
	Description string  `name:"description" help:"Description"`
	WorkType    string  `name:"type" help:"Work type: ASSIGNMENT, SHORT_ANSWER_QUESTION, MULTIPLE_CHOICE_QUESTION" default:"ASSIGNMENT"`
	State       string  `name:"state" help:"State: PUBLISHED, DRAFT"`
	MaxPoints   float64 `name:"max-points" help:"Max points"`
	Due         string  `name:"due" help:"Due date/time (RFC3339 or YYYY-MM-DD [HH:MM])"`
	DueDate     string  `name:"due-date" help:"Due date (YYYY-MM-DD)"`
	DueTime     string  `name:"due-time" help:"Due time (HH:MM or HH:MM:SS)"`
	Scheduled   string  `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
	TopicID     string  `name:"topic" help:"Topic ID"`
}

func (c *ClassroomCourseworkCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if strings.TrimSpace(c.Title) == "" {
		return usage("empty title")
	}

	work := &classroom.CourseWork{
		Title:       strings.TrimSpace(c.Title),
		Description: strings.TrimSpace(c.Description),
		WorkType:    strings.ToUpper(strings.TrimSpace(c.WorkType)),
	}
	if v := strings.TrimSpace(c.State); v != "" {
		work.State = strings.ToUpper(v)
	}
	if c.MaxPoints != 0 {
		work.MaxPoints = c.MaxPoints
	}
	if v := strings.TrimSpace(c.TopicID); v != "" {
		work.TopicId = v
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		work.ScheduledTime = v
	}

	var err error
	var dueDate *classroom.Date
	var dueTime *classroom.TimeOfDay
	if strings.TrimSpace(c.Due) != "" {
		dueDate, dueTime, err = parseClassroomDue(c.Due)
		if err != nil {
			return usage(err.Error())
		}
	} else {
		if strings.TrimSpace(c.DueDate) != "" {
			dueDate, err = parseClassroomDate(c.DueDate)
			if err != nil {
				return usage(err.Error())
			}
		}
		if strings.TrimSpace(c.DueTime) != "" {
			dueTime, err = parseClassroomTime(c.DueTime)
			if err != nil {
				return usage(err.Error())
			}
		}
	}
	if dueTime != nil && dueDate == nil {
		return usage("due time requires a due date")
	}
	if dueDate != nil {
		work.DueDate = dueDate
	}
	if dueTime != nil {
		work.DueTime = dueTime
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.coursework.create", map[string]any{
		"course_id":  courseID,
		"coursework": work,
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

	created, err := svc.Courses.CourseWork.Create(courseID, work).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"coursework": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("title\t%s", created.Title)
	u.Out().Printf("state\t%s", created.State)
	return nil
}

type ClassroomCourseworkUpdateCmd struct {
	CourseID     string  `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string  `arg:"" name:"courseworkId" help:"Coursework ID"`
	Title        string  `name:"title" help:"Title"`
	Description  string  `name:"description" help:"Description"`
	State        string  `name:"state" help:"State: PUBLISHED, DRAFT"`
	MaxPoints    float64 `name:"max-points" help:"Max points"`
	Due          string  `name:"due" help:"Due date/time (RFC3339 or YYYY-MM-DD [HH:MM])"`
	DueDate      string  `name:"due-date" help:"Due date (YYYY-MM-DD)"`
	DueTime      string  `name:"due-time" help:"Due time (HH:MM or HH:MM:SS)"`
	Scheduled    string  `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
	TopicID      string  `name:"topic" help:"Topic ID"`
}

func (c *ClassroomCourseworkUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	work := &classroom.CourseWork{}
	fields := make([]string, 0, 6)

	if v := strings.TrimSpace(c.Title); v != "" {
		work.Title = v
		fields = append(fields, "title")
	}
	if v := strings.TrimSpace(c.Description); v != "" {
		work.Description = v
		fields = append(fields, "description")
	}
	if v := strings.TrimSpace(c.State); v != "" {
		work.State = strings.ToUpper(v)
		fields = append(fields, "state")
	}
	if c.MaxPoints != 0 {
		work.MaxPoints = c.MaxPoints
		fields = append(fields, "maxPoints")
	}
	if v := strings.TrimSpace(c.TopicID); v != "" {
		work.TopicId = v
		fields = append(fields, "topicId")
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		work.ScheduledTime = v
		fields = append(fields, "scheduledTime")
	}

	var err error
	var dueDate *classroom.Date
	var dueTime *classroom.TimeOfDay
	if strings.TrimSpace(c.Due) != "" {
		dueDate, dueTime, err = parseClassroomDue(c.Due)
		if err != nil {
			return usage(err.Error())
		}
	} else {
		if strings.TrimSpace(c.DueDate) != "" {
			dueDate, err = parseClassroomDate(c.DueDate)
			if err != nil {
				return usage(err.Error())
			}
		}
		if strings.TrimSpace(c.DueTime) != "" {
			dueTime, err = parseClassroomTime(c.DueTime)
			if err != nil {
				return usage(err.Error())
			}
		}
	}
	if dueTime != nil && dueDate == nil {
		return usage("due time requires a due date")
	}
	if dueDate != nil {
		work.DueDate = dueDate
		fields = append(fields, "dueDate")
	}
	if dueTime != nil {
		work.DueTime = dueTime
		fields = append(fields, "dueTime")
	}

	if len(fields) == 0 {
		return usage("no updates specified")
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.coursework.update", map[string]any{
		"course_id":     courseID,
		"coursework_id": courseworkID,
		"update_mask":   updateMask(fields),
		"update_fields": fields,
		"coursework":    work,
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

	updated, err := svc.Courses.CourseWork.Patch(courseID, courseworkID, work).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"coursework": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("title\t%s", updated.Title)
	u.Out().Printf("state\t%s", updated.State)
	return nil
}

type ClassroomCourseworkDeleteCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
}

func (c *ClassroomCourseworkDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete coursework %s from %s", courseworkID, courseID)); err != nil {
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

	if _, err := svc.Courses.CourseWork.Delete(courseID, courseworkID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
		kv("courseworkId", courseworkID),
	)
}

type ClassroomCourseworkAssigneesCmd struct {
	CourseID       string   `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID   string   `arg:"" name:"courseworkId" help:"Coursework ID"`
	Mode           string   `name:"mode" help:"Assignee mode: ALL_STUDENTS, INDIVIDUAL_STUDENTS"`
	AddStudents    []string `name:"add-student" help:"Student IDs to add" sep:","`
	RemoveStudents []string `name:"remove-student" help:"Student IDs to remove" sep:","`
}

func (c *ClassroomCourseworkAssigneesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}

	mode, opts, err := normalizeAssigneeMode(c.Mode, c.AddStudents, c.RemoveStudents)
	if err != nil {
		return usage(err.Error())
	}
	req := &classroom.ModifyCourseWorkAssigneesRequest{
		AssigneeMode:                    mode,
		ModifyIndividualStudentsOptions: opts,
	}
	if req.AssigneeMode == "" && req.ModifyIndividualStudentsOptions == nil {
		return usage("no assignee changes specified")
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.coursework.assignees", map[string]any{
		"course_id":     courseID,
		"coursework_id": courseworkID,
		"request":       req,
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

	updated, err := svc.Courses.CourseWork.ModifyAssignees(courseID, courseworkID, req).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"coursework": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("assignee_mode\t%s", updated.AssigneeMode)
	return nil
}
