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

type ClassroomSubmissionsCmd struct {
	List    ClassroomSubmissionsListCmd    `cmd:"" default:"withargs" help:"List student submissions"`
	Get     ClassroomSubmissionsGetCmd     `cmd:"" help:"Get a student submission"`
	TurnIn  ClassroomSubmissionsTurnInCmd  `cmd:"" name:"turn-in" help:"Turn in a submission"`
	Reclaim ClassroomSubmissionsReclaimCmd `cmd:"" help:"Reclaim a submission"`
	Return  ClassroomSubmissionsReturnCmd  `cmd:"" help:"Return a submission"`
	Grade   ClassroomSubmissionsGradeCmd   `cmd:"" help:"Set draft/assigned grades"`
}

type ClassroomSubmissionsListCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	States       string `name:"state" help:"Submission states filter (comma-separated: NEW,CREATED,TURNED_IN,RETURNED,RECLAIMED_BY_STUDENT)"`
	Late         string `name:"late" help:"Late filter: late|not-late"`
	UserID       string `name:"user" help:"Filter by user ID or email"`
	Max          int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page         string `name:"page" help:"Page token"`
}

func (c *ClassroomSubmissionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	call := svc.Courses.CourseWork.StudentSubmissions.List(courseID, courseworkID).PageSize(c.Max).PageToken(c.Page).Context(ctx)
	if states := splitCSV(c.States); len(states) > 0 {
		upper := make([]string, 0, len(states))
		for _, state := range states {
			upper = append(upper, strings.ToUpper(state))
		}
		call.States(upper...)
	}
	if v := strings.TrimSpace(c.UserID); v != "" {
		call.UserId(v)
	}
	if v := strings.ToLower(strings.TrimSpace(c.Late)); v != "" {
		switch v {
		case "late", "late_only", "late-only":
			call.Late("LATE_ONLY")
		case "not-late", "not_late", "not_late_only", "not-late-only", "not-late_only":
			call.Late("NOT_LATE_ONLY")
		default:
			call.Late(strings.ToUpper(v))
		}
	}

	resp, err := call.Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"submissions":   resp.StudentSubmissions,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.StudentSubmissions) == 0 {
		u.Err().Println("No submissions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tUSER_ID\tSTATE\tLATE\tDRAFT\tASSIGNED\tUPDATED")
	for _, sub := range resp.StudentSubmissions {
		if sub == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\t%s\t%s\n",
			sanitizeTab(sub.Id),
			sanitizeTab(sub.UserId),
			sanitizeTab(sub.State),
			sub.Late,
			formatFloatValue(sub.DraftGrade),
			formatFloatValue(sub.AssignedGrade),
			sanitizeTab(sub.UpdateTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ClassroomSubmissionsGetCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	SubmissionID string `arg:"" name:"submissionId" help:"Submission ID"`
}

func (c *ClassroomSubmissionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	submissionID := strings.TrimSpace(c.SubmissionID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}
	if submissionID == "" {
		return usage("empty submissionId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	sub, err := svc.Courses.CourseWork.StudentSubmissions.Get(courseID, courseworkID, submissionID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"submission": sub})
	}

	u.Out().Printf("id\t%s", sub.Id)
	u.Out().Printf("user_id\t%s", sub.UserId)
	u.Out().Printf("state\t%s", sub.State)
	u.Out().Printf("late\t%t", sub.Late)
	u.Out().Printf("draft_grade\t%s", formatFloatValue(sub.DraftGrade))
	u.Out().Printf("assigned_grade\t%s", formatFloatValue(sub.AssignedGrade))
	if sub.UpdateTime != "" {
		u.Out().Printf("updated\t%s", sub.UpdateTime)
	}
	if sub.AlternateLink != "" {
		u.Out().Printf("link\t%s", sub.AlternateLink)
	}
	return nil
}

type ClassroomSubmissionsTurnInCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	SubmissionID string `arg:"" name:"submissionId" help:"Submission ID"`
}

func (c *ClassroomSubmissionsTurnInCmd) Run(ctx context.Context, flags *RootFlags) error {
	return submissionAction(ctx, flags, c.CourseID, c.CourseworkID, c.SubmissionID, "turn-in")
}

type ClassroomSubmissionsReclaimCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	SubmissionID string `arg:"" name:"submissionId" help:"Submission ID"`
}

func (c *ClassroomSubmissionsReclaimCmd) Run(ctx context.Context, flags *RootFlags) error {
	return submissionAction(ctx, flags, c.CourseID, c.CourseworkID, c.SubmissionID, "reclaim")
}

type ClassroomSubmissionsReturnCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	SubmissionID string `arg:"" name:"submissionId" help:"Submission ID"`
}

func (c *ClassroomSubmissionsReturnCmd) Run(ctx context.Context, flags *RootFlags) error {
	return submissionAction(ctx, flags, c.CourseID, c.CourseworkID, c.SubmissionID, "return")
}

func submissionAction(ctx context.Context, flags *RootFlags, courseID, courseworkID, submissionID, action string) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID = strings.TrimSpace(courseID)
	courseworkID = strings.TrimSpace(courseworkID)
	submissionID = strings.TrimSpace(submissionID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}
	if submissionID == "" {
		return usage("empty submissionId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	switch action {
	case "turn-in":
		if _, err := svc.Courses.CourseWork.StudentSubmissions.TurnIn(courseID, courseworkID, submissionID, &classroom.TurnInStudentSubmissionRequest{}).Context(ctx).Do(); err != nil {
			return wrapClassroomError(err)
		}
	case "reclaim":
		if _, err := svc.Courses.CourseWork.StudentSubmissions.Reclaim(courseID, courseworkID, submissionID, &classroom.ReclaimStudentSubmissionRequest{}).Context(ctx).Do(); err != nil {
			return wrapClassroomError(err)
		}
	case "return":
		if _, err := svc.Courses.CourseWork.StudentSubmissions.Return(courseID, courseworkID, submissionID, &classroom.ReturnStudentSubmissionRequest{}).Context(ctx).Do(); err != nil {
			return wrapClassroomError(err)
		}
	default:
		return usagef("unknown action %q", action)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"ok":           true,
			"courseId":     courseID,
			"courseworkId": courseworkID,
			"submissionId": submissionID,
			"action":       action,
		})
	}
	u.Out().Printf("ok\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	u.Out().Printf("coursework_id\t%s", courseworkID)
	u.Out().Printf("submission_id\t%s", submissionID)
	u.Out().Printf("action\t%s", action)
	return nil
}

type ClassroomSubmissionsGradeCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	SubmissionID string `arg:"" name:"submissionId" help:"Submission ID"`
	Draft        string `name:"draft" help:"Draft grade"`
	Assigned     string `name:"assigned" help:"Assigned grade"`
}

func (c *ClassroomSubmissionsGradeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	submissionID := strings.TrimSpace(c.SubmissionID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}
	if submissionID == "" {
		return usage("empty submissionId")
	}

	fields := make([]string, 0, 2)
	sub := &classroom.StudentSubmission{}
	if strings.TrimSpace(c.Draft) != "" {
		grade, parseErr := parseFloat(c.Draft)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		sub.DraftGrade = grade
		fields = append(fields, "draftGrade")
	}
	if strings.TrimSpace(c.Assigned) != "" {
		grade, parseErr := parseFloat(c.Assigned)
		if parseErr != nil {
			return usage(parseErr.Error())
		}
		sub.AssignedGrade = grade
		fields = append(fields, "assignedGrade")
	}
	if len(fields) == 0 {
		return usage("no grades specified")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.CourseWork.StudentSubmissions.Patch(courseID, courseworkID, submissionID, sub).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"submission": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("draft_grade\t%s", formatFloatValue(updated.DraftGrade))
	u.Out().Printf("assigned_grade\t%s", formatFloatValue(updated.AssignedGrade))
	return nil
}
