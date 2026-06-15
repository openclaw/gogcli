package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	classroomCourseStateActive      = "ACTIVE"
	classroomCourseStateArchived    = "ARCHIVED"
	classroomCourseStateProvisioned = "PROVISIONED"
	classroomCourseStateDeclined    = "DECLINED"
)

type ClassroomCoursesCmd struct {
	List      ClassroomCoursesListCmd      `cmd:"" default:"withargs" aliases:"ls" help:"List courses"`
	Get       ClassroomCoursesGetCmd       `cmd:"" aliases:"info,show" help:"Get a course"`
	Create    ClassroomCoursesCreateCmd    `cmd:"" aliases:"add,new" help:"Create a course"`
	Update    ClassroomCoursesUpdateCmd    `cmd:"" aliases:"edit,set" help:"Update a course"`
	Delete    ClassroomCoursesDeleteCmd    `cmd:"" aliases:"rm,del,remove" help:"Delete an archived course"`
	Archive   ClassroomCoursesArchiveCmd   `cmd:"" aliases:"arch" help:"Archive a course and wait until the state is visible"`
	Unarchive ClassroomCoursesUnarchiveCmd `cmd:"" aliases:"unarch,restore" help:"Unarchive a course and wait until the state is visible"`
	Join      ClassroomCoursesJoinCmd      `cmd:"" aliases:"enroll" help:"Join a course"`
	Leave     ClassroomCoursesLeaveCmd     `cmd:"" aliases:"unenroll" help:"Leave a course"`
	URL       ClassroomCoursesURLCmd       `cmd:"" name:"url" aliases:"link" help:"Print Classroom web URLs for courses"`
}

type ClassroomCoursesListCmd struct {
	States    string `name:"state" help:"Course states filter (comma-separated: ACTIVE,ARCHIVED,PROVISIONED,DECLINED)"`
	TeacherID string `name:"teacher" help:"Filter by teacher user ID or email"`
	StudentID string `name:"student" help:"Filter by student user ID or email"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ClassroomCoursesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
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

	fetch := func(pageToken string) ([]*classroom.Course, string, error) {
		call := svc.Courses.List().PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		if states := splitCSV(c.States); len(states) > 0 {
			upper := make([]string, 0, len(states))
			for _, state := range states {
				upper = append(upper, strings.ToUpper(state))
			}
			call.CourseStates(upper...)
		}
		if v := strings.TrimSpace(c.TeacherID); v != "" {
			call.TeacherId(v)
		}
		if v := strings.TrimSpace(c.StudentID); v != "" {
			call.StudentId(v)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", wrapClassroomError(callErr)
		}
		return resp.Courses, resp.NextPageToken, nil
	}

	courses, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}
	courses = nonNilClassroomItems(courses)

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"courses":       courses,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(courses) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(courses) == 0 {
		u.Err().Println("No courses")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactClassroomRows(courses),
		classroomCourseColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type ClassroomCoursesGetCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
}

func (c *ClassroomCoursesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	course, err := svc.Courses.Get(courseID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"course": course})
	}

	u.Out().Linef("id\t%s", course.Id)
	u.Out().Linef("name\t%s", course.Name)
	if course.Section != "" {
		u.Out().Linef("section\t%s", course.Section)
	}
	if course.DescriptionHeading != "" {
		u.Out().Linef("description_heading\t%s", course.DescriptionHeading)
	}
	if course.Description != "" {
		u.Out().Linef("description\t%s", course.Description)
	}
	if course.Room != "" {
		u.Out().Linef("room\t%s", course.Room)
	}
	u.Out().Linef("state\t%s", course.CourseState)
	if course.OwnerId != "" {
		u.Out().Linef("owner\t%s", course.OwnerId)
	}
	if course.EnrollmentCode != "" {
		u.Out().Linef("enrollment_code\t%s", course.EnrollmentCode)
	}
	if course.AlternateLink != "" {
		u.Out().Linef("link\t%s", course.AlternateLink)
	}
	return nil
}

type ClassroomCoursesCreateCmd struct {
	Name               string `name:"name" help:"Course name" required:""`
	OwnerID            string `name:"owner" help:"Owner user ID or email" default:"me"`
	Section            string `name:"section" help:"Section"`
	DescriptionHeading string `name:"description-heading" help:"Description heading"`
	Description        string `name:"description" help:"Description"`
	Room               string `name:"room" help:"Room"`
	State              string `name:"state" help:"Course state (ACTIVE, ARCHIVED, PROVISIONED, DECLINED)"`
}

func (c *ClassroomCoursesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := buildClassroomCourseCreatePlan(classroomCourseInput{
		Name:               c.Name,
		OwnerID:            c.OwnerID,
		Section:            c.Section,
		DescriptionHeading: c.DescriptionHeading,
		Description:        c.Description,
		Room:               c.Room,
		State:              c.State,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.courses.create", map[string]any{
		"course": plan.Course,
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

	created, err := svc.Courses.Create(plan.Course).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"course": created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("name\t%s", created.Name)
	u.Out().Linef("state\t%s", created.CourseState)
	u.Out().Linef("owner\t%s", created.OwnerId)
	return nil
}

type ClassroomCoursesUpdateCmd struct {
	CourseID           string `arg:"" name:"courseId" help:"Course ID or alias"`
	Name               string `name:"name" help:"Course name"`
	OwnerID            string `name:"owner" help:"Owner user ID or email"`
	Section            string `name:"section" help:"Section"`
	DescriptionHeading string `name:"description-heading" help:"Description heading"`
	Description        string `name:"description" help:"Description"`
	Room               string `name:"room" help:"Room"`
	State              string `name:"state" help:"Course state (ACTIVE, ARCHIVED, PROVISIONED, DECLINED)"`
}

func (c *ClassroomCoursesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	plan, err := buildClassroomCourseUpdatePlan(classroomCourseUpdateInput{
		CourseID: c.CourseID,
		classroomCourseInput: classroomCourseInput{
			Name:               c.Name,
			OwnerID:            c.OwnerID,
			Section:            c.Section,
			DescriptionHeading: c.DescriptionHeading,
			Description:        c.Description,
			Room:               c.Room,
			State:              c.State,
		},
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.courses.update", map[string]any{
		"course_id":     plan.CourseID,
		"update_mask":   plan.UpdateMask,
		"update_fields": plan.UpdateFields,
		"course":        plan.Course,
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

	updated, err := svc.Courses.Patch(plan.CourseID, plan.Course).UpdateMask(plan.UpdateMask).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}
	if plan.Course.CourseState != "" {
		updated, err = waitForClassroomCourseState(ctx, svc, plan.CourseID, plan.Course.CourseState)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"course": updated})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("name\t%s", updated.Name)
	u.Out().Linef("state\t%s", updated.CourseState)
	return nil
}

type ClassroomCoursesDeleteCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
}

func (c *ClassroomCoursesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "classroom.courses.delete", map[string]any{
		"course_id": courseID,
	}, fmt.Sprintf("delete course %s", courseID)); err != nil {
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

	course, err := svc.Courses.Get(courseID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}
	if course.CourseState != classroomCourseStateArchived {
		return classroomCourseDeleteStateError(courseID, course.CourseState)
	}

	if _, err := svc.Courses.Delete(courseID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
	)
}

func classroomCourseDeleteStateError(courseID, state string) error {
	switch state {
	case classroomCourseStateActive:
		return fmt.Errorf(
			"course %s is ACTIVE; archive it before deleting: gog classroom courses archive %s",
			courseID,
			courseID,
		)
	case classroomCourseStateProvisioned:
		return fmt.Errorf(
			"course %s is PROVISIONED; accept its teaching invitation in Google Classroom, then archive it before deleting",
			courseID,
		)
	case classroomCourseStateDeclined:
		return fmt.Errorf("course %s is DECLINED; declined courses cannot be deleted or recovered", courseID)
	default:
		return fmt.Errorf(
			"course %s must be ARCHIVED before deletion (current state: %s)",
			courseID,
			state,
		)
	}
}

type ClassroomCoursesArchiveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
}

func (c *ClassroomCoursesArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return updateCourseState(ctx, flags, c.CourseID, classroomCourseStateArchived)
}

type ClassroomCoursesUnarchiveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
}

func (c *ClassroomCoursesUnarchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return updateCourseState(ctx, flags, c.CourseID, classroomCourseStateActive)
}

func updateCourseState(ctx context.Context, flags *RootFlags, courseID, state string) error {
	u := ui.FromContext(ctx)
	courseID = strings.TrimSpace(courseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	course := &classroom.Course{CourseState: state}
	op := "classroom.courses.update_state"
	switch state {
	case classroomCourseStateArchived:
		op = "classroom.courses.archive"
	case classroomCourseStateActive:
		op = "classroom.courses.unarchive"
	}
	if err := dryRunExit(ctx, flags, op, map[string]any{
		"course_id":   courseID,
		"courseState": state,
		"course":      course,
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

	if _, err = svc.Courses.Patch(courseID, course).UpdateMask("courseState").Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}
	updated, err := waitForClassroomCourseState(ctx, svc, courseID, state)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"course": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("state\t%s", updated.CourseState)
	return nil
}

func waitForClassroomCourseState(
	ctx context.Context,
	svc *classroom.Service,
	courseID string,
	wantState string,
) (*classroom.Course, error) {
	return pollClassroomCourseState(
		ctx,
		courseID,
		wantState,
		defaultClassroomCourseStateVisibilityDelays(),
		waitForPollInterval,
		func(ctx context.Context) (*classroom.Course, error) {
			return svc.Courses.Get(courseID).Context(ctx).Do()
		},
	)
}

func defaultClassroomCourseStateVisibilityDelays() []time.Duration {
	return []time.Duration{
		0,
		200 * time.Millisecond,
		500 * time.Millisecond,
		time.Second,
		2 * time.Second,
		3 * time.Second,
		4 * time.Second,
		5 * time.Second,
	}
}

func pollClassroomCourseState(
	ctx context.Context,
	courseID string,
	wantState string,
	delays []time.Duration,
	wait func(context.Context, time.Duration) error,
	fetch func(context.Context) (*classroom.Course, error),
) (*classroom.Course, error) {
	var lastState string
	for _, delay := range delays {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if delay > 0 {
			if err := wait(ctx, delay); err != nil {
				return nil, err
			}
		}

		course, err := fetch(ctx)
		if err != nil {
			return nil, wrapClassroomError(err)
		}
		if course != nil {
			lastState = course.CourseState
			if lastState == wantState {
				return course, nil
			}
		}
	}

	if lastState == "" {
		lastState = "(not returned)"
	}
	return nil, &ExitError{
		Code: exitCodeRetryable,
		Err: fmt.Errorf(
			"course %s update was accepted, but reads still show state %s instead of %s; retry shortly",
			courseID,
			lastState,
			wantState,
		),
	}
}

type ClassroomCoursesJoinCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	Role           string `name:"role" help:"Role to join as: student|teacher" default:"student"`
	UserID         string `name:"user" help:"User ID or email to join" default:"me"`
	EnrollmentCode string `name:"enrollment-code" help:"Enrollment code (student joins only)"`
}

func (c *ClassroomCoursesJoinCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	role := strings.ToLower(strings.TrimSpace(c.Role))
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return usage("empty user")
	}

	if err := dryRunExit(ctx, flags, "classroom.courses.join", map[string]any{
		"course_id":       courseID,
		"role":            role,
		"user_id":         userID,
		"enrollment_code": strings.TrimSpace(c.EnrollmentCode),
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

	switch role {
	case "student":
		student := &classroom.Student{UserId: userID}
		call := svc.Courses.Students.Create(courseID, student).Context(ctx)
		if code := strings.TrimSpace(c.EnrollmentCode); code != "" {
			call.EnrollmentCode(code)
		}
		created, err := call.Do()
		if err != nil {
			return wrapClassroomError(err)
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"student": created})
		}
		u.Out().Linef("user_id\t%s", created.UserId)
		u.Out().Linef("email\t%s", profileEmail(created.Profile))
		u.Out().Linef("name\t%s", profileName(created.Profile))
		return nil
	case "teacher":
		teacher := &classroom.Teacher{UserId: userID}
		created, err := svc.Courses.Teachers.Create(courseID, teacher).Context(ctx).Do()
		if err != nil {
			return wrapClassroomError(err)
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"teacher": created})
		}
		u.Out().Linef("user_id\t%s", created.UserId)
		u.Out().Linef("email\t%s", profileEmail(created.Profile))
		u.Out().Linef("name\t%s", profileName(created.Profile))
		return nil
	default:
		return usagef("invalid role %q (expected student or teacher)", role)
	}
}

type ClassroomCoursesLeaveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	Role     string `name:"role" help:"Role to remove: student|teacher" default:"student"`
	UserID   string `name:"user" help:"User ID or email to remove" default:"me"`
}

func (c *ClassroomCoursesLeaveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	role := strings.ToLower(strings.TrimSpace(c.Role))
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return usage("empty user")
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "classroom.courses.leave", map[string]any{
		"course_id": courseID,
		"role":      role,
		"user_id":   userID,
	}, fmt.Sprintf("remove %s %s from course %s", role, userID, courseID)); err != nil {
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

	switch role {
	case "student":
		if _, err := svc.Courses.Students.Delete(courseID, userID).Context(ctx).Do(); err != nil {
			return wrapClassroomError(err)
		}
	case "teacher":
		if _, err := svc.Courses.Teachers.Delete(courseID, userID).Context(ctx).Do(); err != nil {
			return wrapClassroomError(err)
		}
	default:
		return usagef("invalid role %q (expected student or teacher)", role)
	}

	return writeResult(ctx, u,
		kv("removed", true),
		kv("courseId", courseID),
		kv("userId", userID),
		kv("role", role),
	)
}

type ClassroomCoursesURLCmd struct {
	CourseIDs []string `arg:"" name:"courseId" help:"Course IDs or aliases"`
}

func (c *ClassroomCoursesURLCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if len(c.CourseIDs) == 0 {
		return usage("missing courseId")
	}

	svc, err := classroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		urls := make([]map[string]string, 0, len(c.CourseIDs))
		for _, id := range c.CourseIDs {
			link, err := classroomCourseLink(ctx, svc, id)
			if err != nil {
				return err
			}
			urls = append(urls, map[string]string{"id": id, "url": link})
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"urls": urls})
	}

	for _, id := range c.CourseIDs {
		link, err := classroomCourseLink(ctx, svc, id)
		if err != nil {
			return err
		}
		u.Out().Linef("%s\t%s", id, link)
	}
	return nil
}

func classroomCourseLink(ctx context.Context, svc *classroom.Service, courseID string) (string, error) {
	id := strings.TrimSpace(courseID)
	if id == "" {
		return "", usage("empty courseId")
	}
	course, err := svc.Courses.Get(id).Context(ctx).Do()
	if err != nil {
		return "", wrapClassroomError(err)
	}
	return course.AlternateLink, nil
}
