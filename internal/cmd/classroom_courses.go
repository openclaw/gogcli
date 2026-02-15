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

type ClassroomCoursesCmd struct {
	List      ClassroomCoursesListCmd      `cmd:"" default:"withargs" aliases:"ls" help:"List courses"`
	Get       ClassroomCoursesGetCmd       `cmd:"" aliases:"info,show" help:"Get a course"`
	Create    ClassroomCoursesCreateCmd    `cmd:"" aliases:"add,new" help:"Create a course"`
	Update    ClassroomCoursesUpdateCmd    `cmd:"" aliases:"edit,set" help:"Update a course"`
	Delete    ClassroomCoursesDeleteCmd    `cmd:"" aliases:"rm,del,remove" help:"Delete a course"`
	Archive   ClassroomCoursesArchiveCmd   `cmd:"" aliases:"arch" help:"Archive a course"`
	Unarchive ClassroomCoursesUnarchiveCmd `cmd:"" aliases:"unarch,restore" help:"Unarchive a course"`
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
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
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Courses, resp.NextPageToken, nil
	}

	var courses []*classroom.Course
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		courses = all
	} else {
		var err error
		courses, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
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

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tSECTION\tSTATE\tOWNER")
	for _, course := range courses {
		if course == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(course.Id),
			sanitizeTab(course.Name),
			sanitizeTab(course.Section),
			sanitizeTab(course.CourseState),
			sanitizeTab(course.OwnerId),
		)
	}
	printNextPageHint(u, nextPageToken)
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	course, err := svc.Courses.Get(courseID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"course": course})
	}

	u.Out().Printf("id\t%s", course.Id)
	u.Out().Printf("name\t%s", course.Name)
	if course.Section != "" {
		u.Out().Printf("section\t%s", course.Section)
	}
	if course.DescriptionHeading != "" {
		u.Out().Printf("description_heading\t%s", course.DescriptionHeading)
	}
	if course.Description != "" {
		u.Out().Printf("description\t%s", course.Description)
	}
	if course.Room != "" {
		u.Out().Printf("room\t%s", course.Room)
	}
	u.Out().Printf("state\t%s", course.CourseState)
	if course.OwnerId != "" {
		u.Out().Printf("owner\t%s", course.OwnerId)
	}
	if course.EnrollmentCode != "" {
		u.Out().Printf("enrollment_code\t%s", course.EnrollmentCode)
	}
	if course.AlternateLink != "" {
		u.Out().Printf("link\t%s", course.AlternateLink)
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
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}
	owner := strings.TrimSpace(c.OwnerID)
	if owner == "" {
		return usage("empty owner")
	}

	course := &classroom.Course{
		Name:    name,
		OwnerId: owner,
	}
	if v := strings.TrimSpace(c.Section); v != "" {
		course.Section = v
	}
	if v := strings.TrimSpace(c.DescriptionHeading); v != "" {
		course.DescriptionHeading = v
	}
	if v := strings.TrimSpace(c.Description); v != "" {
		course.Description = v
	}
	if v := strings.TrimSpace(c.Room); v != "" {
		course.Room = v
	}
	if v := strings.TrimSpace(c.State); v != "" {
		course.CourseState = strings.ToUpper(v)
	}

	if err := dryRunExit(ctx, flags, "classroom.courses.create", map[string]any{
		"course": course,
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

	created, err := svc.Courses.Create(course).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"course": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("state\t%s", created.CourseState)
	u.Out().Printf("owner\t%s", created.OwnerId)
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
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	course := &classroom.Course{}
	fields := make([]string, 0, 6)

	if v := strings.TrimSpace(c.Name); v != "" {
		course.Name = v
		fields = append(fields, "name")
	}
	if v := strings.TrimSpace(c.OwnerID); v != "" {
		course.OwnerId = v
		fields = append(fields, "ownerId")
	}
	if v := strings.TrimSpace(c.Section); v != "" {
		course.Section = v
		fields = append(fields, "section")
	}
	if v := strings.TrimSpace(c.DescriptionHeading); v != "" {
		course.DescriptionHeading = v
		fields = append(fields, "descriptionHeading")
	}
	if v := strings.TrimSpace(c.Description); v != "" {
		course.Description = v
		fields = append(fields, "description")
	}
	if v := strings.TrimSpace(c.Room); v != "" {
		course.Room = v
		fields = append(fields, "room")
	}
	if v := strings.TrimSpace(c.State); v != "" {
		course.CourseState = strings.ToUpper(v)
		fields = append(fields, "courseState")
	}

	if len(fields) == 0 {
		return usage("no updates specified")
	}

	if err := dryRunExit(ctx, flags, "classroom.courses.update", map[string]any{
		"course_id":     courseID,
		"update_mask":   updateMask(fields),
		"update_fields": fields,
		"course":        course,
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

	updated, err := svc.Courses.Patch(courseID, course).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"course": updated})
	}
	u := ui.FromContext(ctx)
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	u.Out().Printf("state\t%s", updated.CourseState)
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

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete course %s", courseID)); err != nil {
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

	if _, err := svc.Courses.Delete(courseID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
	)
}

type ClassroomCoursesArchiveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
}

func (c *ClassroomCoursesArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return updateCourseState(ctx, flags, c.CourseID, "ARCHIVED")
}

type ClassroomCoursesUnarchiveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
}

func (c *ClassroomCoursesUnarchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return updateCourseState(ctx, flags, c.CourseID, "ACTIVE")
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
	case "ARCHIVED":
		op = "classroom.courses.archive"
	case "ACTIVE":
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Patch(courseID, course).UpdateMask("courseState").Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"course": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("state\t%s", updated.CourseState)
	return nil
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

	svc, err := newClassroomService(ctx, account)
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
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"student": created})
		}
		u.Out().Printf("user_id\t%s", created.UserId)
		u.Out().Printf("email\t%s", profileEmail(created.Profile))
		u.Out().Printf("name\t%s", profileName(created.Profile))
		return nil
	case "teacher":
		teacher := &classroom.Teacher{UserId: userID}
		created, err := svc.Courses.Teachers.Create(courseID, teacher).Context(ctx).Do()
		if err != nil {
			return wrapClassroomError(err)
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"teacher": created})
		}
		u.Out().Printf("user_id\t%s", created.UserId)
		u.Out().Printf("email\t%s", profileEmail(created.Profile))
		u.Out().Printf("name\t%s", profileName(created.Profile))
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

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove %s %s from course %s", role, userID, courseID)); err != nil {
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

	svc, err := newClassroomService(ctx, account)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"urls": urls})
	}

	for _, id := range c.CourseIDs {
		link, err := classroomCourseLink(ctx, svc, id)
		if err != nil {
			return err
		}
		u.Out().Printf("%s\t%s", id, link)
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
