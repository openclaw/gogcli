package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ClassroomSubmissionsCmd struct {
	List    ClassroomSubmissionsListCmd    `cmd:"" default:"withargs" aliases:"ls" help:"List student submissions"`
	Get     ClassroomSubmissionsGetCmd     `cmd:"" aliases:"info,show" help:"Get a student submission"`
	TurnIn  ClassroomSubmissionsTurnInCmd  `cmd:"" name:"turn-in" aliases:"turnin" help:"Turn in a submission"`
	Reclaim ClassroomSubmissionsReclaimCmd `cmd:"" aliases:"undo" help:"Reclaim a submission"`
	Return  ClassroomSubmissionsReturnCmd  `cmd:"" aliases:"send" help:"Return a submission"`
	Grade   ClassroomSubmissionsGradeCmd   `cmd:"" aliases:"set,edit" help:"Set draft/assigned grades"`
}

type ClassroomSubmissionsListCmd struct {
	CourseID     string `arg:"" name:"courseId" help:"Course ID or alias"`
	CourseworkID string `arg:"" name:"courseworkId" help:"Coursework ID"`
	States       string `name:"state" help:"Submission states filter (comma-separated: NEW,CREATED,TURNED_IN,RETURNED,RECLAIMED_BY_STUDENT)"`
	Late         string `name:"late" help:"Late filter: late|not-late"`
	UserID       string `name:"user" help:"Filter by user ID or email"`
	Max          int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page         string `name:"page" aliases:"cursor" help:"Page token"`
	All          bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty    bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ClassroomSubmissionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	courseworkID := strings.TrimSpace(c.CourseworkID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if courseworkID == "" {
		return usage("empty courseworkId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	fetch := func(pageToken string) ([]*classroom.StudentSubmission, string, error) {
		call := svc.Courses.CourseWork.StudentSubmissions.List(courseID, courseworkID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
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

		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", wrapClassroomError(callErr)
		}
		return resp.StudentSubmissions, resp.NextPageToken, nil
	}

	submissions, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}
	submissions = nonNilClassroomItems(submissions)

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"submissions":   submissions,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(submissions) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(submissions) == 0 {
		u.Err().Println("No submissions")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactClassroomRows(submissions),
		classroomSubmissionColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
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

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	sub, err := svc.Courses.CourseWork.StudentSubmissions.Get(courseID, courseworkID, submissionID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"submission": sub})
	}

	u.Out().Linef("id\t%s", sub.Id)
	u.Out().Linef("user_id\t%s", sub.UserId)
	u.Out().Linef("state\t%s", sub.State)
	u.Out().Linef("late\t%t", sub.Late)
	u.Out().Linef("draft_grade\t%s", formatFloatValue(sub.DraftGrade))
	u.Out().Linef("assigned_grade\t%s", formatFloatValue(sub.AssignedGrade))
	if sub.UpdateTime != "" {
		u.Out().Linef("updated\t%s", sub.UpdateTime)
	}
	if sub.AlternateLink != "" {
		u.Out().Linef("link\t%s", sub.AlternateLink)
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

	if err := dryRunExit(ctx, flags, "classroom.submissions."+action, map[string]any{
		"course_id":     courseID,
		"coursework_id": courseworkID,
		"submission_id": submissionID,
		"action":        action,
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"ok":           true,
			"courseId":     courseID,
			"courseworkId": courseworkID,
			"submissionId": submissionID,
			"action":       action,
		})
	}
	u.Out().Linef("ok\ttrue")
	u.Out().Linef("course_id\t%s", courseID)
	u.Out().Linef("coursework_id\t%s", courseworkID)
	u.Out().Linef("submission_id\t%s", submissionID)
	u.Out().Linef("action\t%s", action)
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
	plan, err := buildClassroomSubmissionGradePlan(classroomSubmissionGradeInput{
		CourseID:     c.CourseID,
		CourseworkID: c.CourseworkID,
		SubmissionID: c.SubmissionID,
		Draft:        c.Draft,
		Assigned:     c.Assigned,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.submissions.grade", map[string]any{
		"course_id":     plan.CourseID,
		"coursework_id": plan.CourseworkID,
		"submission_id": plan.SubmissionID,
		"update_mask":   plan.UpdateMask,
		"update_fields": plan.UpdateFields,
		"submission":    plan.Submission,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.CourseWork.StudentSubmissions.Patch(plan.CourseID, plan.CourseworkID, plan.SubmissionID, plan.Submission).UpdateMask(plan.UpdateMask).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"submission": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("draft_grade\t%s", formatFloatValue(updated.DraftGrade))
	u.Out().Linef("assigned_grade\t%s", formatFloatValue(updated.AssignedGrade))
	return nil
}
