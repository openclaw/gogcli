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

type ClassroomCoursesCmd struct {
	List      ClassroomCoursesListCmd      `cmd:"" default:"withargs" help:"List courses"`
	Get       ClassroomCoursesGetCmd       `cmd:"" help:"Get a course"`
	Create    ClassroomCoursesCreateCmd    `cmd:"" help:"Create a course"`
	Update    ClassroomCoursesUpdateCmd    `cmd:"" help:"Update a course"`
	Delete    ClassroomCoursesDeleteCmd    `cmd:"" help:"Delete a course" aliases:"rm"`
	Archive   ClassroomCoursesArchiveCmd   `cmd:"" help:"Archive a course"`
	Unarchive ClassroomCoursesUnarchiveCmd `cmd:"" help:"Unarchive a course"`
	Join      ClassroomCoursesJoinCmd      `cmd:"" help:"Join a course"`
	Leave     ClassroomCoursesLeaveCmd     `cmd:"" help:"Leave a course"`
	URL       ClassroomCoursesURLCmd       `cmd:"" name:"url" help:"Print Classroom web URLs for courses"`
}

type ClassroomCoursesListCmd struct {
	States    string `name:"state" help:"Course states filter (comma-separated: ACTIVE,ARCHIVED,PROVISIONED,DECLINED)"`
	TeacherID string `name:"teacher" help:"Filter by teacher user ID or email"`
	StudentID string `name:"student" help:"Filter by student user ID or email"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" help:"Page token"`
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

	call := svc.Courses.List().PageSize(c.Max).PageToken(c.Page).Context(ctx)
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
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"courses":       resp.Courses,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Courses) == 0 {
		u.Err().Println("No courses")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tSECTION\tSTATE\tOWNER")
	for _, course := range resp.Courses {
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
	printNextPageHint(u, resp.NextPageToken)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"course": course})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}
	owner := strings.TrimSpace(c.OwnerID)
	if owner == "" {
		return usage("empty owner")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
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

	created, err := svc.Courses.Create(course).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"course": created})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Patch(courseID, course).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"course": updated})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	err = confirmDestructive(ctx, flags, fmt.Sprintf("delete course %s", courseID))
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":  true,
			"courseId": courseID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	return nil
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID = strings.TrimSpace(courseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	course := &classroom.Course{CourseState: state}
	updated, err := svc.Courses.Patch(courseID, course).UpdateMask("courseState").Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"course": updated})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	role := strings.ToLower(strings.TrimSpace(c.Role))
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return usage("empty user")
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
			return outfmt.WriteJSON(os.Stdout, map[string]any{"student": created})
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
			return outfmt.WriteJSON(os.Stdout, map[string]any{"teacher": created})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	role := strings.ToLower(strings.TrimSpace(c.Role))
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return usage("empty user")
	}

	err = confirmDestructive(ctx, flags, fmt.Sprintf("remove %s %s from course %s", role, userID, courseID))
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"removed":  true,
			"courseId": courseID,
			"userId":   userID,
			"role":     role,
		})
	}
	u.Out().Printf("removed\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	u.Out().Printf("user_id\t%s", userID)
	u.Out().Printf("role\t%s", role)
	return nil
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"urls": urls})
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
